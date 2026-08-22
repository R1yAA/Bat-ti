// Package statichtml reads a server-rendered storefront that publishes no
// product feed. Everything comes from the product page itself, located through
// the vendor's sitemap and extracted with the CSS selectors configured for that
// vendor in config/vendors.go.
//
// This is the most expensive tier by far: one HTTP request per product, where
// the JSON tiers read a whole catalogue in a handful. It is used only where no
// cheaper source exists.
package statichtml

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/R1yAA/Bat-ti/config"
	"github.com/R1yAA/Bat-ti/internal/scraper"
	"github.com/R1yAA/Bat-ti/internal/scraper/httpclient"
	"github.com/R1yAA/Bat-ti/internal/scraper/sitemap"
	"github.com/shopspring/decimal"
)

// productPathSegment identifies product URLs inside a sitemap that also lists
// category and content pages.
const productPathSegment = "/product/"

// ErrProductGone reports a soft 404: the vendor serves HTTP 200 with a "page
// not found" body for products it has removed but not yet dropped from its
// sitemap. Distinguishing this from a genuine parse failure matters — a
// "could not parse" warning should mean the vendor changed its markup and the
// selectors need attention, not that a stale sitemap entry was followed.
var ErrProductGone = errors.New("product page is a soft 404")

// Scraper reads one server-rendered storefront.
type Scraper struct {
	vendorConfig config.VendorConfig
	client       *httpclient.Client
	logger       *slog.Logger
	// maxListings caps how many product pages a run will fetch. Zero means no
	// cap. It exists so a full crawl of this tier — one request per product,
	// thousands of products — can be smoke-tested in seconds during
	// development without waiting for the real thing.
	maxListings int
}

// New builds a scraper driven by the vendor's configured selectors.
func New(
	vendorConfig config.VendorConfig,
	client *httpclient.Client,
	logger *slog.Logger,
	maxListings int,
) *Scraper {
	return &Scraper{
		vendorConfig: vendorConfig,
		client:       client,
		logger:       logger,
		maxListings:  maxListings,
	}
}

// TierName implements scraper.VendorScraper.
func (staticScraper *Scraper) TierName() string { return "static_html" }

// FetchCatalog implements scraper.VendorScraper.
func (staticScraper *Scraper) FetchCatalog(ctx context.Context) ([]scraper.ScrapedListing, error) {
	sitemapURL := staticScraper.vendorConfig.CatalogDiscovery.SitemapURL
	if sitemapURL == "" {
		return nil, fmt.Errorf("vendor %s has no sitemap configured", staticScraper.vendorConfig.VendorSlug)
	}

	sitemapEntries, err := sitemap.Fetch(ctx, staticScraper.client, sitemapURL)
	if err != nil {
		return nil, err
	}
	productEntries := sitemap.FilterByPathPrefix(sitemapEntries, productPathSegment)
	staticScraper.logger.Info("sitemap read",
		"total_urls", len(sitemapEntries), "product_urls", len(productEntries))

	if staticScraper.maxListings > 0 && len(productEntries) > staticScraper.maxListings {
		staticScraper.logger.Warn("capping this run; the catalogue is not complete",
			"cap", staticScraper.maxListings, "available", len(productEntries))
		productEntries = productEntries[:staticScraper.maxListings]
	}

	scrapedListings := make([]scraper.ScrapedListing, 0, len(productEntries))
	for _, productEntry := range productEntries {
		pageBytes, err := staticScraper.client.GetBytes(ctx, productEntry.Location)
		if err != nil {
			// One dead product page must not abandon the whole catalogue; the
			// listing simply is not seen this run, and the delisting sweep
			// decides what that means.
			staticScraper.logger.Warn("skipping product page",
				"url", productEntry.Location, "error", err)
			continue
		}

		scrapedListing, err := ParseProductPage(pageBytes, productEntry.Location, staticScraper.vendorConfig)
		if errors.Is(err, ErrProductGone) {
			// Not seen this run, so the delisting sweep will retire it.
			staticScraper.logger.Debug("product no longer exists",
				"url", productEntry.Location)
			continue
		}
		if err != nil {
			staticScraper.logger.Warn("could not parse product page",
				"url", productEntry.Location, "error", err)
			continue
		}
		scrapedListings = append(scrapedListings, scrapedListing)
	}
	return scrapedListings, nil
}

// EnrichListing implements scraper.VendorScraper. FetchCatalog already reads
// every product page in full, so there is nothing left to fetch.
func (staticScraper *Scraper) EnrichListing(_ context.Context, _ *scraper.ScrapedListing) error {
	return nil
}

// trailingIDPattern pulls the vendor's own product id out of a URL shaped like
// /product/{slug}/{id}, giving a stable key that survives a slug rename.
var trailingIDPattern = regexp.MustCompile(`/(\d+)/?$`)

// ParseProductPage extracts one listing from product page HTML. Exported so it
// can be tested against a saved fixture without touching the network.
func ParseProductPage(
	pageHTML []byte,
	productURL string,
	vendorConfig config.VendorConfig,
) (scraper.ScrapedListing, error) {
	document, err := goquery.NewDocumentFromReader(strings.NewReader(string(pageHTML)))
	if err != nil {
		return scraper.ScrapedListing{}, fmt.Errorf("parsing product page HTML: %w", err)
	}
	selectors := vendorConfig.ListingSelectors

	scrapedListing := scraper.ScrapedListing{
		ProductURL: productURL,
		// No stock indicator is rendered on this storefront, so a page that
		// exists means the product is offered. If that ever stops being true a
		// StockIndicator selector can be added to the vendor config.
		IsInStock: true,
	}

	if match := trailingIDPattern.FindStringSubmatch(productURL); match != nil {
		scrapedListing.ExternalProductID = match[1]
	}

	scrapedListing.ListingName = firstText(document, selectors.ListingName)
	if scrapedListing.ListingName == "" {
		if isSoftNotFoundPage(document) {
			return scraper.ScrapedListing{}, fmt.Errorf("%s: %w", productURL, ErrProductGone)
		}
		return scraper.ScrapedListing{}, fmt.Errorf("no listing name at %s", productURL)
	}

	scrapedListing.Description = firstText(document, selectors.Description)
	scrapedListing.VendorSideCategory = firstText(document, selectors.VendorSideCategory())
	scrapedListing.PrimaryImageURL = absoluteURL(
		firstAttribute(document, selectors.PrimaryImageURL, "src"),
		vendorConfig.SourceBaseURL,
	)

	// The pack size is rendered in its own node, e.g. "Quantity (Pack of 100)",
	// so it is read from there before falling back to the product name.
	scrapedListing.PackSize = scraper.ParsePackSize(firstText(document, selectors.PackSizeLabel))
	if scrapedListing.PackSize == nil {
		scrapedListing.PackSize = scraper.ParsePackSize(scrapedListing.ListingName)
	}

	scrapedListing.BasePrice = parseCurrency(firstText(document, selectors.BasePrice))
	scrapedListing.MoqTiers = extractTiers(document, selectors)

	// The ladder's opening row is the authoritative unit price when the two
	// disagree, because that is the number the vendor charges for a single
	// unit.
	if len(scrapedListing.MoqTiers) > 0 {
		openingPrice := scrapedListing.MoqTiers[0].PricePerUnit
		scrapedListing.BasePrice = &openingPrice
	}

	return scrapedListing, nil
}

// extractTiers reads the quantity-discount ladder. Rows whose quantity cell is
// not a quantity range — the header row, and any unrelated table on the page —
// are skipped, which is what lets a single generic selector work without the
// vendor giving its tier table a class or an id.
func extractTiers(document *goquery.Document, selectors config.ListingSelectors) []scraper.ScrapedMoqTier {
	if selectors.MOQTierRow == "" {
		return nil
	}

	var tiers []scraper.ScrapedMoqTier
	document.Find(selectors.MOQTierRow).Each(func(_ int, row *goquery.Selection) {
		quantityText := strings.TrimSpace(row.Find(selectors.MOQQuantityCell).First().Text())
		minimumQuantity, maximumQuantity, isQuantityRange := scraper.ParseQuantityRange(quantityText)
		if !isQuantityRange {
			return
		}

		tierPrice := parseCurrency(row.Find(selectors.MOQPriceCell).First().Text())
		if tierPrice == nil {
			return
		}

		tier := scraper.ScrapedMoqTier{
			QuantityRangeMinimum: minimumQuantity,
			QuantityRangeMaximum: maximumQuantity,
			PricePerUnit:         *tierPrice,
		}
		if selectors.MOQDiscountCell != "" {
			if discount := parseCurrency(row.Find(selectors.MOQDiscountCell).First().Text()); discount != nil &&
				discount.IsPositive() {
				tier.DiscountPercent = discount
			}
		}
		tiers = append(tiers, tier)
	})
	return tiers
}

// isSoftNotFoundPage recognises the "200 OK but actually missing" page this
// storefront serves for removed products.
func isSoftNotFoundPage(document *goquery.Document) bool {
	if strings.TrimSpace(document.Find("h1").First().Text()) == "404" {
		return true
	}
	return strings.Contains(
		strings.ToLower(document.Find("h2").First().Text()), "page not found")
}

// currencyNoisePattern strips everything that is not part of a decimal number:
// currency symbols, the rupee HTML entity once unescaped, spaces and thousands
// separators.
var currencyNoisePattern = regexp.MustCompile(`[^\d.]`)

func parseCurrency(text string) *decimal.Decimal {
	cleanedText := currencyNoisePattern.ReplaceAllString(strings.TrimSpace(text), "")
	if cleanedText == "" || cleanedText == "." {
		return nil
	}
	parsedPrice, err := decimal.NewFromString(cleanedText)
	if err != nil {
		return nil
	}
	return &parsedPrice
}

func firstText(document *goquery.Document, selector string) string {
	if selector == "" {
		return ""
	}
	return strings.Join(strings.Fields(document.Find(selector).First().Text()), " ")
}

func firstAttribute(document *goquery.Document, selector string, attributeName string) string {
	if selector == "" {
		return ""
	}
	attributeValue, _ := document.Find(selector).First().Attr(attributeName)
	return strings.TrimSpace(attributeValue)
}

func absoluteURL(rawURL string, sourceBaseURL string) string {
	if rawURL == "" || strings.HasPrefix(rawURL, "http") {
		return rawURL
	}
	base, err := url.Parse(sourceBaseURL)
	if err != nil {
		return rawURL
	}
	reference, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	return base.ResolveReference(reference).String()
}
