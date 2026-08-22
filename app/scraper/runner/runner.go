// Package runner drives a scrape end to end: it builds the right scraper for a
// vendor's tier, syncs the whole catalogue cheaply, soft-deletes anything the
// vendor has pulled, then deep-scrapes only the listings the user has starred.
package runner

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	// Embedded zone data so the IST date used for price history is correct
	// even on a container image carrying no system tzdata.
	_ "time/tzdata"

	"github.com/R1yAA/Bat-ti/config"
	"github.com/R1yAA/Bat-ti/app/database"
	"github.com/R1yAA/Bat-ti/app/scraper"
	"github.com/R1yAA/Bat-ti/app/scraper/dotpe"
	"github.com/R1yAA/Bat-ti/app/scraper/httpclient"
	"github.com/R1yAA/Bat-ti/app/scraper/shopify"
	"github.com/R1yAA/Bat-ti/app/scraper/statichtml"
	"github.com/R1yAA/Bat-ti/app/scraper/woocommerce"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

// businessTimeZoneName is the zone whose calendar day a price-history row is
// filed under. A run at 23:30 UTC is 05:00 the next morning in the business's
// own time, and the history graph must agree with the calendar Riya reads.
const businessTimeZoneName = "Asia/Kolkata"

// Runner executes scrapes against the database.
type Runner struct {
	pool                 *pgxpool.Pool
	queries              *database.Queries
	logger               *slog.Logger
	businessTimeLocation *time.Location
	// maxListings caps the per-product tiers during development. Zero, the
	// production value, means no cap.
	maxListings int
}

// New builds a Runner.
func New(pool *pgxpool.Pool, logger *slog.Logger, maxListings int) (*Runner, error) {
	businessTimeLocation, err := time.LoadLocation(businessTimeZoneName)
	if err != nil {
		return nil, fmt.Errorf("loading time zone %s: %w", businessTimeZoneName, err)
	}
	return &Runner{
		pool:                 pool,
		queries:              database.New(pool),
		logger:               logger,
		businessTimeLocation: businessTimeLocation,
		maxListings:          maxListings,
	}, nil
}

// SyncVendorRegistry writes config/vendors.go into the vendors table so the two
// can never drift. Safe to run before every scrape.
func (runner *Runner) SyncVendorRegistry(ctx context.Context) error {
	for _, vendorConfig := range config.TrackedVendors {
		_, err := runner.queries.UpsertVendorFromConfig(ctx, database.UpsertVendorFromConfigParams{
			VendorSlug:    vendorConfig.VendorSlug,
			VendorName:    vendorConfig.DisplayName,
			SourceBaseUrl: vendorConfig.SourceBaseURL,
			ScraperTier:   string(vendorConfig.ScraperTier),
			ScrapeHourUtc: int16(vendorConfig.ScrapeHourUTC),
		})
		if err != nil {
			return fmt.Errorf("syncing vendor %s: %w", vendorConfig.VendorSlug, err)
		}
	}
	runner.logger.Info("vendor registry synced", "vendor_count", len(config.TrackedVendors))
	return nil
}

// RunVendorBySlug scrapes exactly one vendor.
func (runner *Runner) RunVendorBySlug(ctx context.Context, vendorSlug string) error {
	vendorConfig, err := config.FindVendorBySlug(vendorSlug)
	if err != nil {
		return err
	}
	vendorRow, err := runner.queries.GetVendorBySlug(ctx, vendorSlug)
	if err != nil {
		return fmt.Errorf("loading vendor %s from the database: %w", vendorSlug, err)
	}
	return runner.runOneVendor(ctx, vendorConfig, vendorRow)
}

// RunDueVendors scrapes every vendor whose hour slot has arrived, strictly one
// after another. Sequential execution is the point: it keeps concurrent load
// off the vendor sites and makes a failing run easy to place in the logs.
func (runner *Runner) RunDueVendors(ctx context.Context) error {
	dueVendors, err := runner.queries.ListVendorsDueForScrape(ctx)
	if err != nil {
		return fmt.Errorf("listing vendors due for scrape: %w", err)
	}
	if len(dueVendors) == 0 {
		runner.logger.Info("no vendors are due for a scrape this hour")
		return nil
	}
	runner.logger.Info("vendors due for scrape", "count", len(dueVendors))

	var runErrors []error
	for _, vendorRow := range dueVendors {
		vendorConfig, err := config.FindVendorBySlug(vendorRow.VendorSlug)
		if err != nil {
			runErrors = append(runErrors, err)
			continue
		}
		// One vendor failing must not stop the rest: each has its own slot and
		// its own error recorded against it.
		if err := runner.runOneVendor(ctx, vendorConfig, vendorRow); err != nil {
			runErrors = append(runErrors, fmt.Errorf("%s: %w", vendorRow.VendorSlug, err))
		}
	}
	return errors.Join(runErrors...)
}

type scrapeSummary struct {
	listingsSeen         int
	listingsPriceChanged int
	listingsDelisted     int
	trackedEnriched      int
}

func (runner *Runner) runOneVendor(
	ctx context.Context,
	vendorConfig config.VendorConfig,
	vendorRow database.Vendor,
) error {
	logger := runner.logger.With("vendor", vendorConfig.VendorSlug, "tier", string(vendorConfig.ScraperTier))
	runStartedAt := time.Now().UTC()

	scrapeRun, err := runner.queries.StartScrapeRun(ctx, vendorRow.VendorID)
	if err != nil {
		return fmt.Errorf("opening scrape run: %w", err)
	}
	if err := runner.queries.MarkVendorScrapeAttempt(ctx, vendorRow.VendorID); err != nil {
		return fmt.Errorf("recording scrape attempt: %w", err)
	}

	summary, err := runner.scrapeAndPersist(ctx, vendorConfig, vendorRow, runStartedAt, logger)
	if err != nil {
		runner.recordFailure(ctx, scrapeRun.ScrapeRunID, vendorRow.VendorID, err, logger)
		return err
	}

	if err := runner.queries.FinishScrapeRunSuccess(ctx, database.FinishScrapeRunSuccessParams{
		ScrapeRunID:      scrapeRun.ScrapeRunID,
		ListingsSeen:     int32(summary.listingsSeen),
		ListingsUpdated:  int32(summary.listingsPriceChanged),
		ListingsDelisted: int32(summary.listingsDelisted),
	}); err != nil {
		return fmt.Errorf("closing scrape run: %w", err)
	}
	if err := runner.queries.MarkVendorScrapeSuccess(ctx, vendorRow.VendorID); err != nil {
		return fmt.Errorf("recording scrape success: %w", err)
	}

	logger.Info("scrape complete",
		"listings_seen", summary.listingsSeen,
		"price_changed", summary.listingsPriceChanged,
		"delisted", summary.listingsDelisted,
		"tracked_enriched", summary.trackedEnriched,
		"duration", time.Since(runStartedAt).Round(time.Second).String())
	return nil
}

// recordFailure is best effort: the run has already failed, so a bookkeeping
// problem here must never mask the original error.
func (runner *Runner) recordFailure(
	ctx context.Context,
	scrapeRunID uuid.UUID,
	vendorID uuid.UUID,
	runError error,
	logger *slog.Logger,
) {
	logger.Error("scrape failed", "error", runError)

	if err := runner.queries.FinishScrapeRunFailure(ctx, database.FinishScrapeRunFailureParams{
		ScrapeRunID:  scrapeRunID,
		ErrorMessage: database.TextOrNull(runError.Error()),
	}); err != nil {
		logger.Warn("could not record scrape run failure", "error", err)
	}
	if err := runner.queries.MarkVendorScrapeFailure(ctx, database.MarkVendorScrapeFailureParams{
		VendorID:        vendorID,
		LastScrapeError: database.TextOrNull(runError.Error()),
	}); err != nil {
		logger.Warn("could not record vendor scrape error", "error", err)
	}
}

// trackedListing pairs what the scrape found with the row it was written to,
// so the deep phase does not have to re-query or re-fetch the catalogue.
type trackedListing struct {
	scrapedListing scraper.ScrapedListing
	listingRow     database.VendorListing
}

func (runner *Runner) scrapeAndPersist(
	ctx context.Context,
	vendorConfig config.VendorConfig,
	vendorRow database.Vendor,
	runStartedAt time.Time,
	logger *slog.Logger,
) (scrapeSummary, error) {
	var summary scrapeSummary

	client := httpclient.New(
		time.Duration(vendorConfig.RequestDelaySeconds)*time.Second,
		logger,
	)
	vendorScraper, err := buildVendorScraper(vendorConfig, client, logger, runner.maxListings)
	if err != nil {
		return summary, err
	}

	// ── Phase A: whole catalogue, cheap ────────────────────────────────────
	scrapedListings, err := vendorScraper.FetchCatalog(ctx)
	if err != nil {
		return summary, fmt.Errorf("fetching catalogue: %w", err)
	}
	summary.listingsSeen = len(scrapedListings)
	logger.Info("catalogue fetched", "listings", len(scrapedListings))

	var trackedListings []trackedListing
	for _, scrapedListing := range scrapedListings {
		listingRow, err := runner.persistListing(ctx, vendorRow.VendorID, scrapedListing, runStartedAt)
		if err != nil {
			return summary, err
		}
		if listingRow.PriceLastChangedAt.Valid &&
			!listingRow.PriceLastChangedAt.Time.Before(runStartedAt) {
			summary.listingsPriceChanged++
		}
		if listingRow.IsTracked {
			trackedListings = append(trackedListings, trackedListing{
				scrapedListing: scrapedListing,
				listingRow:     listingRow,
			})
		}
	}

	// The delisting sweep concludes "not seen this run means the vendor pulled
	// it", which is only sound when the run saw the whole catalogue. A capped
	// development run did not, so it must not sweep — otherwise it would mark
	// every listing it skipped as delisted.
	if runner.maxListings > 0 {
		logger.Warn("skipping the delisting sweep because this run was capped")
	} else {
		delistedCount, err := runner.queries.MarkUnseenListingsDelisted(ctx,
			database.MarkUnseenListingsDelistedParams{
				VendorID:   vendorRow.VendorID,
				LastSeenAt: database.Timestamptz(runStartedAt),
			})
		if err != nil {
			return summary, fmt.Errorf("marking unseen listings delisted: %w", err)
		}
		summary.listingsDelisted = int(delistedCount)
	}

	// ── Phase B: starred listings only, expensive ──────────────────────────
	scrapedOnDate := time.Now().In(runner.businessTimeLocation)
	for _, tracked := range trackedListings {
		enrichedListing := tracked.scrapedListing
		if err := vendorScraper.EnrichListing(ctx, &enrichedListing); err != nil {
			// A single product page failing is not a reason to fail the whole
			// vendor: the catalogue sync already succeeded.
			logger.Warn("could not enrich tracked listing",
				"product_url", tracked.scrapedListing.ProductURL, "error", err)
			continue
		}
		// Enrichment can discover variants the catalogue phase did not carry —
		// WooCommerce only reveals per-variation prices on the product page —
		// so the listing is persisted again before its tiers are written.
		enrichedRow, err := runner.persistListing(ctx, vendorRow.VendorID, enrichedListing, runStartedAt)
		if err != nil {
			return summary, err
		}
		if err := runner.persistDeepDetail(ctx, enrichedRow, enrichedListing, scrapedOnDate); err != nil {
			return summary, err
		}
		summary.trackedEnriched++
	}

	return summary, nil
}

func (runner *Runner) persistListing(
	ctx context.Context,
	vendorID uuid.UUID,
	scrapedListing scraper.ScrapedListing,
	runStartedAt time.Time,
) (database.VendorListing, error) {
	listingRow, err := runner.queries.UpsertVendorListing(ctx, database.UpsertVendorListingParams{
		VendorID:           vendorID,
		ProductUrl:         scrapedListing.ProductURL,
		ExternalProductID:  database.TextOrNull(scrapedListing.ExternalProductID),
		ListingName:        scrapedListing.ListingName,
		Description:        database.TextOrNull(scrapedListing.Description),
		PrimaryImageUrl:    database.TextOrNull(scrapedListing.PrimaryImageURL),
		VendorSideCategory: database.TextOrNull(scrapedListing.VendorSideCategory),
		VendorSideSku:      database.TextOrNull(scrapedListing.VendorSideSKU),
		IsInStock:          scrapedListing.IsInStock,
		HasVariants:        scrapedListing.HasVariants(),
		PackSize:           database.Int4OrNull(scrapedListing.PackSize),
		CurrentPrice:       database.DecimalOrNull(scrapedListing.BasePrice),
	})
	if err != nil {
		return database.VendorListing{}, fmt.Errorf("upserting listing %s: %w", scrapedListing.ProductURL, err)
	}

	if !scrapedListing.HasVariants() {
		return listingRow, nil
	}

	for _, scrapedVariant := range scrapedListing.Variants {
		if _, err := runner.queries.UpsertVariant(ctx, database.UpsertVariantParams{
			VendorListingID:   listingRow.VendorListingID,
			VariantLabel:      scrapedVariant.VariantLabel,
			ExternalVariantID: database.TextOrNull(scrapedVariant.ExternalVariantID),
			VariantSku:        database.TextOrNull(scrapedVariant.VariantSKU),
			IsInStock:         scrapedVariant.IsInStock,
			PackSize:          database.Int4OrNull(scrapedVariant.PackSize),
			CurrentPrice:      database.DecimalOrNull(scrapedVariant.Price),
		}); err != nil {
			return database.VendorListing{}, fmt.Errorf("upserting variant %q of %s: %w",
				scrapedVariant.VariantLabel, scrapedListing.ProductURL, err)
		}
	}

	if _, err := runner.queries.MarkUnseenVariantsDelisted(ctx, database.MarkUnseenVariantsDelistedParams{
		VendorListingID: listingRow.VendorListingID,
		LastSeenAt:      database.Timestamptz(runStartedAt),
	}); err != nil {
		return database.VendorListing{}, fmt.Errorf("marking unseen variants delisted: %w", err)
	}

	// The catalogue shows "from X" for a listing with variants, so the
	// listing's own price tracks the cheapest live variant.
	if err := runner.queries.SetListingBasePriceFromVariants(ctx, listingRow.VendorListingID); err != nil {
		return database.VendorListing{}, fmt.Errorf("rolling variant prices up to the listing: %w", err)
	}

	// Re-read so the caller sees the rolled-up price and its change stamp.
	refreshedRow, err := runner.queries.GetVendorListingByID(ctx, listingRow.VendorListingID)
	if err != nil {
		return database.VendorListing{}, fmt.Errorf("re-reading listing after roll-up: %w", err)
	}
	return refreshedRow, nil
}

// persistDeepDetail writes the MOQ ladder and today's price-history row for a
// starred listing. Tier rows are replaced wholesale inside one transaction:
// the ladders are three or four rows, so diffing would be more code and more
// failure modes for no gain.
func (runner *Runner) persistDeepDetail(
	ctx context.Context,
	listingRow database.VendorListing,
	scrapedListing scraper.ScrapedListing,
	scrapedOnDate time.Time,
) error {
	transaction, err := runner.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("opening transaction for %s: %w", listingRow.ProductUrl, err)
	}
	defer transaction.Rollback(ctx)

	transactionalQueries := runner.queries.WithTx(transaction)

	if scrapedListing.HasVariants() {
		variantRows, err := transactionalQueries.ListVariantsForListing(ctx, listingRow.VendorListingID)
		if err != nil {
			return fmt.Errorf("loading variants of %s: %w", listingRow.ProductUrl, err)
		}
		variantRowByLabel := make(map[string]database.Variant, len(variantRows))
		for _, variantRow := range variantRows {
			variantRowByLabel[variantRow.VariantLabel] = variantRow
		}

		for _, scrapedVariant := range scrapedListing.Variants {
			variantRow, found := variantRowByLabel[scrapedVariant.VariantLabel]
			if !found {
				continue
			}

			tiers := scrapedVariant.MoqTiers
			if len(tiers) == 0 {
				tiers = synthesiseSingleTier(scrapedVariant.Price)
			}
			if err := transactionalQueries.DeleteMoqTiersForVariant(ctx,
				database.NullUUID(variantRow.VariantID)); err != nil {
				return fmt.Errorf("clearing tiers for variant %s: %w", scrapedVariant.VariantLabel, err)
			}
			for _, tier := range tiers {
				if _, err := transactionalQueries.InsertVariantMoqTier(ctx, database.InsertVariantMoqTierParams{
					VariantID:            database.NullUUID(variantRow.VariantID),
					QuantityRangeMinimum: int32(tier.QuantityRangeMinimum),
					QuantityRangeMaximum: database.Int4OrNull(tier.QuantityRangeMaximum),
					PricePerUnit:         tier.PricePerUnit,
					DiscountPercent:      database.DecimalOrNull(tier.DiscountPercent),
				}); err != nil {
					return fmt.Errorf("writing tier for variant %s: %w", scrapedVariant.VariantLabel, err)
				}
			}

			if scrapedVariant.Price != nil {
				if err := transactionalQueries.UpsertVariantPriceHistory(ctx,
					database.UpsertVariantPriceHistoryParams{
						VariantID:     database.NullUUID(variantRow.VariantID),
						ScrapedAtDate: database.Date(scrapedOnDate),
						Price:         *scrapedVariant.Price,
					}); err != nil {
					return fmt.Errorf("writing price history for variant %s: %w", scrapedVariant.VariantLabel, err)
				}
			}
		}
	} else {
		tiers := scrapedListing.MoqTiers
		if len(tiers) == 0 {
			tiers = synthesiseSingleTier(scrapedListing.BasePrice)
		}
		if err := transactionalQueries.DeleteMoqTiersForListing(ctx,
			database.NullUUID(listingRow.VendorListingID)); err != nil {
			return fmt.Errorf("clearing tiers for %s: %w", listingRow.ProductUrl, err)
		}
		for _, tier := range tiers {
			if _, err := transactionalQueries.InsertListingMoqTier(ctx, database.InsertListingMoqTierParams{
				VendorListingID:      database.NullUUID(listingRow.VendorListingID),
				QuantityRangeMinimum: int32(tier.QuantityRangeMinimum),
				QuantityRangeMaximum: database.Int4OrNull(tier.QuantityRangeMaximum),
				PricePerUnit:         tier.PricePerUnit,
				DiscountPercent:      database.DecimalOrNull(tier.DiscountPercent),
			}); err != nil {
				return fmt.Errorf("writing tier for %s: %w", listingRow.ProductUrl, err)
			}
		}

		if scrapedListing.BasePrice != nil {
			if err := transactionalQueries.UpsertListingPriceHistory(ctx,
				database.UpsertListingPriceHistoryParams{
					VendorListingID: database.NullUUID(listingRow.VendorListingID),
					ScrapedAtDate:   database.Date(scrapedOnDate),
					Price:           *scrapedListing.BasePrice,
				}); err != nil {
				return fmt.Errorf("writing price history for %s: %w", listingRow.ProductUrl, err)
			}
		}
	}

	if err := transaction.Commit(ctx); err != nil {
		return fmt.Errorf("committing deep detail for %s: %w", listingRow.ProductUrl, err)
	}
	return nil
}

func buildVendorScraper(
	vendorConfig config.VendorConfig,
	client *httpclient.Client,
	logger *slog.Logger,
	maxListings int,
) (scraper.VendorScraper, error) {
	switch vendorConfig.ScraperTier {
	case config.ScraperTierShopifyJSON:
		return shopify.New(vendorConfig.SourceBaseURL, client, logger), nil
	case config.ScraperTierWooCommerceJSON:
		return woocommerce.New(vendorConfig.SourceBaseURL, client, logger), nil
	case config.ScraperTierDotpeJSON:
		return dotpe.New(vendorConfig, client, logger, maxListings), nil
	case config.ScraperTierStaticHTML:
		return statichtml.New(vendorConfig, client, logger, maxListings), nil
	case config.ScraperTierManual:
		return nil, fmt.Errorf("vendor %s is entered by hand and is not scraped", vendorConfig.VendorSlug)
	default:
		return nil, fmt.Errorf("no scraper is implemented for tier %q", vendorConfig.ScraperTier)
	}
}

// synthesiseSingleTier covers the common case of a vendor with no quantity
// discount. BR-4 wants a tier table everywhere, and such a vendor still has
// one: buy one or more, pay the listed price.
func synthesiseSingleTier(price *decimal.Decimal) []scraper.ScrapedMoqTier {
	if price == nil {
		return nil
	}
	return []scraper.ScrapedMoqTier{{
		QuantityRangeMinimum: 1,
		QuantityRangeMaximum: nil,
		PricePerUnit:         *price,
	}}
}
