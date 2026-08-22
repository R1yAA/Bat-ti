// Package runner drives a scrape end to end: it builds the right scraper for a
// vendor's tier, syncs the whole catalogue cheaply, soft-deletes anything the
// vendor has pulled, then deep-scrapes only the listings the user has starred.
package runner

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	// Embedded zone data so the IST date used for price history is correct
	// even on a container image carrying no system tzdata.
	_ "time/tzdata"

	"github.com/R1yAA/Bat-ti/app/database"
	"github.com/R1yAA/Bat-ti/app/scraper"
	"github.com/R1yAA/Bat-ti/app/scraper/dotpe"
	"github.com/R1yAA/Bat-ti/app/scraper/httpclient"
	"github.com/R1yAA/Bat-ti/app/scraper/shopify"
	"github.com/R1yAA/Bat-ti/app/scraper/statichtml"
	"github.com/R1yAA/Bat-ti/app/scraper/woocommerce"
	"github.com/R1yAA/Bat-ti/config"
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

// scrapedListingFromRow rebuilds a stored listing in the shape the scrapers
// use, carrying every field forward.
//
// Enrichment adds variants and tier ladders; it does not re-read the name,
// image or price. Those are persisted through the same upsert, so anything
// left blank here would be written back as blank — which silently erased the
// price of every listing whose product page carries no price of its own.
// Copying the row in full keeps enrichment additive.
func scrapedListingFromRow(listingRow database.VendorListing) scraper.ScrapedListing {
	return scraper.ScrapedListing{
		ProductURL:         listingRow.ProductUrl,
		ExternalProductID:  database.TextOrEmpty(listingRow.ExternalProductID),
		ListingName:        listingRow.ListingName,
		Description:        database.TextOrEmpty(listingRow.Description),
		PrimaryImageURL:    database.TextOrEmpty(listingRow.PrimaryImageUrl),
		VendorSideCategory: database.TextOrEmpty(listingRow.VendorSideCategory),
		VendorSideSKU:      database.TextOrEmpty(listingRow.VendorSideSku),
		IsInStock:          listingRow.IsInStock,
		PackSize:           database.IntValue(listingRow.PackSize),
		BasePrice:          database.DecimalValue(listingRow.CurrentPrice),
	}
}

// EnrichOneListing fetches one product page and stores what only that page
// carries: the MOQ ladder, per-variant prices, and today's price-history row.
//
// It exists because starring a listing is otherwise invisible until the next
// nightly run. Deep detail is what the star turns on, so the star should
// deliver it straight away rather than a day later.
func (runner *Runner) EnrichOneListing(
	ctx context.Context,
	vendorListingID uuid.UUID,
	recordPriceHistory bool,
) error {
	listingRow, err := runner.queries.GetVendorListingByID(ctx, vendorListingID)
	if err != nil {
		return fmt.Errorf("loading the listing: %w", err)
	}
	vendorRow, err := runner.queries.GetVendorByID(ctx, listingRow.VendorID)
	if err != nil {
		return fmt.Errorf("loading its vendor: %w", err)
	}
	vendorConfig, err := config.FindVendorBySlug(vendorRow.VendorSlug)
	if err != nil {
		return err
	}

	client := httpclient.New(
		time.Duration(vendorConfig.RequestDelaySeconds)*time.Second, runner.logger)
	vendorScraper, err := buildVendorScraper(vendorConfig, client, runner.logger, 0)
	if err != nil {
		return err
	}

	scrapedListing := scrapedListingFromRow(listingRow)
	if err := vendorScraper.EnrichListing(ctx, &scrapedListing); err != nil {
		return fmt.Errorf("reading %s: %w", listingRow.ProductUrl, err)
	}

	storedRow, err := runner.persistListing(ctx, listingRow.VendorID, scrapedListing, time.Now())
	if err != nil {
		return err
	}
	if err := runner.persistDeepDetail(ctx, storedRow, scrapedListing,
		time.Now().In(runner.businessTimeLocation), recordPriceHistory); err != nil {
		return err
	}
	return runner.queries.MarkListingDetailFetched(ctx, vendorListingID)
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
	detailRefreshed      int
}

// detailRefreshBudgetPerRun caps how many product pages a single run reads for
// listings the user has not starred.
//
// Some vendors keep sizes, their prices and the discount ladder on the product
// page alone, so those listings show no options until the page has been read.
// Reading every one of them in a single run would take hours — Jindeal alone
// has over two thousand — and the job has a ninety-minute ceiling. A fixed
// budget per run fills the catalogue in over several days and then simply
// keeps it fresh, with the oldest read first so nothing is starved.
const detailRefreshBudgetPerRun = 400

// detailRefreshBudget returns the per-run budget, allowing DETAIL_REFRESH_BUDGET
// to override it. Vendor sites differ in how fast they answer, so this is worth
// tuning without a rebuild.
func detailRefreshBudget() int32 {
	if raw := os.Getenv("DETAIL_REFRESH_BUDGET"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed >= 0 {
			return int32(parsed)
		}
	}
	return detailRefreshBudgetPerRun
}

// detailStaleAfter is when an already-read product page is worth reading
// again. Longer than a day, because these listings are not the starred ones
// whose prices are being followed closely.
const detailStaleAfter = 7 * 24 * time.Hour

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
		"detail_refreshed", summary.detailRefreshed,
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
	for chunkStart := 0; chunkStart < len(scrapedListings); chunkStart += persistChunkSize {
		chunkEnd := min(chunkStart+persistChunkSize, len(scrapedListings))
		chunk := scrapedListings[chunkStart:chunkEnd]

		listingRows, err := runner.persistListings(ctx, vendorRow.VendorID, chunk, runStartedAt)
		if err != nil {
			return summary, err
		}
		for index, listingRow := range listingRows {
			if listingRow.PriceLastChangedAt.Valid &&
				!listingRow.PriceLastChangedAt.Time.Before(runStartedAt) {
				summary.listingsPriceChanged++
			}
			if listingRow.IsTracked {
				trackedListings = append(trackedListings, trackedListing{
					scrapedListing: chunk[index],
					listingRow:     listingRow,
				})
			}
		}

		// Without this a large vendor is half an hour of silence, which reads
		// exactly like a hung job.
		logger.Info("listings persisted",
			"done", chunkEnd, "of", len(scrapedListings))
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
		if err := runner.persistDeepDetail(ctx, enrichedRow, enrichedListing, scrapedOnDate, true); err != nil {
			return summary, err
		}
		summary.trackedEnriched++
	}

	// ── Phase C: options for everything else, a bounded slice per run ──────
	//
	// Starring governs price history, not whether a product's sizes are
	// visible: the sizes are what someone needs in order to judge whether a
	// product is worth starting to follow.
	if vendorConfig.ScraperTier.ProductPageAddsDetail() && runner.maxListings == 0 {
		refreshed, err := runner.refreshListingDetail(ctx, vendorRow, vendorScraper, logger)
		if err != nil {
			// The catalogue sync already succeeded, so this is reported rather
			// than failing the vendor.
			logger.Warn("could not refresh listing detail", "error", err)
		}
		summary.detailRefreshed = refreshed
	}

	return summary, nil
}

// refreshListingDetail reads product pages for listings whose options are
// missing or stale, up to this run's budget.
func (runner *Runner) refreshListingDetail(
	ctx context.Context,
	vendorRow database.Vendor,
	vendorScraper scraper.VendorScraper,
	logger *slog.Logger,
) (int, error) {
	pendingListings, err := runner.queries.ListListingsNeedingDetail(ctx,
		database.ListListingsNeedingDetailParams{
			VendorID:    vendorRow.VendorID,
			StaleBefore: database.Timestamptz(time.Now().Add(-detailStaleAfter)),
			ResultLimit: detailRefreshBudget(),
		})
	if err != nil {
		return 0, fmt.Errorf("listing products needing detail: %w", err)
	}
	if len(pendingListings) == 0 {
		return 0, nil
	}

	logger.Info("refreshing listing detail", "count", len(pendingListings))
	scrapedOnDate := time.Now().In(runner.businessTimeLocation)

	refreshedCount := 0
	for _, listingRow := range pendingListings {
		// Stop cleanly when the job is being shut down rather than leaving the
		// remaining pages half-read.
		if ctx.Err() != nil {
			break
		}

		enrichedListing := scrapedListingFromRow(listingRow)
		if err := vendorScraper.EnrichListing(ctx, &enrichedListing); err != nil {
			logger.Warn("could not read product page",
				"product_url", listingRow.ProductUrl, "error", err)
			continue
		}

		storedRow, err := runner.persistListing(ctx, vendorRow.VendorID, enrichedListing, time.Now())
		if err != nil {
			return refreshedCount, err
		}
		// Price history belongs to starred listings alone (D1), so it is
		// written here only when this listing happens to be one.
		if err := runner.persistDeepDetail(ctx, storedRow, enrichedListing,
			scrapedOnDate, storedRow.IsTracked); err != nil {
			return refreshedCount, err
		}
		if err := runner.queries.MarkListingDetailFetched(ctx, listingRow.VendorListingID); err != nil {
			return refreshedCount, err
		}
		refreshedCount++
	}
	return refreshedCount, nil
}

// persistChunkSize is how many listings are written per set of round trips.
//
// The database is remote, so latency dominates: writing a listing one query at
// a time cost roughly six round trips each, and a 2,800-product vendor took
// over half an hour. Batching turns each chunk into five round trips no matter
// how many listings it holds. The size is a balance — larger chunks amortise
// better, but every statement in a chunk is one implicit transaction, so a
// failure discards more work and the memory held for results grows.
const persistChunkSize = 100

// persistListings writes a chunk of listings and their variants, returning the
// stored rows in the order they were given.
//
// The query order is exactly what the one-at-a-time version did — listings,
// then variants, then the delisting sweep, then the price roll-up — because
// each step reads what the previous one wrote. What changes is only how many
// round trips that costs.
func (runner *Runner) persistListings(
	ctx context.Context,
	vendorID uuid.UUID,
	scrapedListings []scraper.ScrapedListing,
	runStartedAt time.Time,
) ([]database.VendorListing, error) {
	if len(scrapedListings) == 0 {
		return nil, nil
	}

	// ── 1. the listings themselves ────────────────────────────────────────
	listingParams := make([]database.UpsertVendorListingParams, 0, len(scrapedListings))
	for _, scrapedListing := range scrapedListings {
		listingParams = append(listingParams, database.UpsertVendorListingParams{
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
	}

	listingRows := make([]database.VendorListing, len(scrapedListings))
	var firstError error
	runner.queries.UpsertVendorListing(ctx, listingParams).QueryRow(
		func(index int, listingRow database.VendorListing, err error) {
			if err != nil {
				if firstError == nil {
					firstError = fmt.Errorf("upserting listing %s: %w",
						scrapedListings[index].ProductURL, err)
				}
				return
			}
			listingRows[index] = listingRow
		})
	if firstError != nil {
		return nil, firstError
	}

	// ── 2. variants, for the listings that have them ──────────────────────
	var variantParams []database.UpsertVariantParams
	var listingIDsWithVariants []uuid.UUID
	indexByListingID := map[uuid.UUID]int{}

	for index, scrapedListing := range scrapedListings {
		if !scrapedListing.HasVariants() {
			continue
		}
		listingID := listingRows[index].VendorListingID
		listingIDsWithVariants = append(listingIDsWithVariants, listingID)
		indexByListingID[listingID] = index

		for _, scrapedVariant := range scrapedListing.Variants {
			variantParams = append(variantParams, database.UpsertVariantParams{
				VendorListingID:   listingID,
				VariantLabel:      scrapedVariant.VariantLabel,
				ExternalVariantID: database.TextOrNull(scrapedVariant.ExternalVariantID),
				VariantSku:        database.TextOrNull(scrapedVariant.VariantSKU),
				IsInStock:         scrapedVariant.IsInStock,
				PackSize:          database.Int4OrNull(scrapedVariant.PackSize),
				CurrentPrice:      database.DecimalOrNull(scrapedVariant.Price),
			})
		}
	}

	if len(listingIDsWithVariants) == 0 {
		return listingRows, nil
	}

	if len(variantParams) > 0 {
		runner.queries.UpsertVariant(ctx, variantParams).Exec(func(index int, err error) {
			if err != nil && firstError == nil {
				firstError = fmt.Errorf("upserting variant %q: %w",
					variantParams[index].VariantLabel, err)
			}
		})
		if firstError != nil {
			return nil, firstError
		}
	}

	// ── 3. variants the vendor dropped, then the "from X" roll-up ─────────
	delistParams := make([]database.MarkUnseenVariantsDelistedParams, 0, len(listingIDsWithVariants))
	for _, listingID := range listingIDsWithVariants {
		delistParams = append(delistParams, database.MarkUnseenVariantsDelistedParams{
			VendorListingID: listingID,
			LastSeenAt:      database.Timestamptz(runStartedAt),
		})
	}
	runner.queries.MarkUnseenVariantsDelisted(ctx, delistParams).Exec(func(_ int, err error) {
		if err != nil && firstError == nil {
			firstError = fmt.Errorf("marking unseen variants delisted: %w", err)
		}
	})
	if firstError != nil {
		return nil, firstError
	}

	runner.queries.SetListingBasePriceFromVariants(ctx, listingIDsWithVariants).Exec(
		func(_ int, err error) {
			if err != nil && firstError == nil {
				firstError = fmt.Errorf("rolling variant prices up to the listing: %w", err)
			}
		})
	if firstError != nil {
		return nil, firstError
	}

	// ── 4. read back what the roll-up rewrote ─────────────────────────────
	refreshedRows, err := runner.queries.GetVendorListingsByIDs(ctx, listingIDsWithVariants)
	if err != nil {
		return nil, fmt.Errorf("re-reading listings after roll-up: %w", err)
	}
	for _, refreshedRow := range refreshedRows {
		if index, ok := indexByListingID[refreshedRow.VendorListingID]; ok {
			listingRows[index] = refreshedRow
		}
	}

	return listingRows, nil
}

// persistListing writes a single listing. Phase B enriches starred listings one
// product page at a time, so there is nothing to group there.
func (runner *Runner) persistListing(
	ctx context.Context,
	vendorID uuid.UUID,
	scrapedListing scraper.ScrapedListing,
	runStartedAt time.Time,
) (database.VendorListing, error) {
	listingRows, err := runner.persistListings(ctx, vendorID,
		[]scraper.ScrapedListing{scrapedListing}, runStartedAt)
	if err != nil {
		return database.VendorListing{}, err
	}
	return listingRows[0], nil
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
	recordPriceHistory bool,
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

			if recordPriceHistory && scrapedVariant.Price != nil {
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

		if recordPriceHistory && scrapedListing.BasePrice != nil {
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
