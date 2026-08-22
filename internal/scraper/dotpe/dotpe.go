// Package dotpe reads a storefront built on the DotPe platform.
//
// The tech PRD assumed this vendor needed a headless browser. It does not: the
// storefront is a Next.js app whose product pages are server-rendered, and each
// one embeds the complete product object in its __NEXT_DATA__ script tag. A
// plain GET plus a JSON decode gets everything — name, price, stock, images,
// category — with no browser, no Node runtime and no Playwright.
//
// The catalogue is enumerated through the product sitemap, because collection
// pages load their items client-side and the platform exposes no public
// catalogue endpoint. That means one request per product, which is why this
// tier's request delay matters.
package dotpe

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"

	"github.com/R1yAA/Bat-ti/config"
	"github.com/R1yAA/Bat-ti/internal/scraper"
	"github.com/R1yAA/Bat-ti/internal/scraper/httpclient"
	"github.com/R1yAA/Bat-ti/internal/scraper/sitemap"
	"github.com/shopspring/decimal"
)

const productPathSegment = "/product/"

// nextDataPattern lifts the embedded JSON out of the server-rendered page.
// goquery would also work, but the payload is a single well-known script tag
// and this avoids building a full DOM for every one of ~1,700 products.
var nextDataPattern = regexp.MustCompile(
	`(?s)<script id="__NEXT_DATA__" type="application/json">(.*?)</script>`)

type nextData struct {
	Props struct {
		PageProps struct {
			Product product `json:"product"`
		} `json:"pageProps"`
	} `json:"props"`
}

type product struct {
	ID                int64          `json:"id"`
	Name              string         `json:"name"`
	Price             json.Number    `json:"price"`
	DiscountedPrice   json.Number    `json:"discounted_price"`
	Available         int            `json:"available"`
	ImageURL          string         `json:"image_url"`
	Images            []productImage `json:"images"`
	Description       string         `json:"description"`
	DescriptionDetail string         `json:"description_detail"`
	SKUID             string         `json:"sku_id"`
	Category          struct {
		Name string `json:"name"`
	} `json:"category"`
	B2BPricingInfo struct {
		MinimumOrderQuantity int `json:"minimum_order_quantity"`
	} `json:"b2b_pricing_info"`
}

type productImage struct {
	ImageURL string `json:"image_url"`
}

// Scraper reads one DotPe storefront.
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

// New builds a scraper for the storefront described by vendorConfig.
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
func (dotpeScraper *Scraper) TierName() string { return "dotpe_json" }

// FetchCatalog implements scraper.VendorScraper.
func (dotpeScraper *Scraper) FetchCatalog(ctx context.Context) ([]scraper.ScrapedListing, error) {
	sitemapURL := dotpeScraper.vendorConfig.CatalogDiscovery.SitemapURL
	if sitemapURL == "" {
		return nil, fmt.Errorf("vendor %s has no sitemap configured", dotpeScraper.vendorConfig.VendorSlug)
	}

	sitemapEntries, err := sitemap.Fetch(ctx, dotpeScraper.client, sitemapURL)
	if err != nil {
		return nil, err
	}
	productEntries := sitemap.FilterByPathPrefix(sitemapEntries, productPathSegment)
	dotpeScraper.logger.Info("sitemap read",
		"total_urls", len(sitemapEntries), "product_urls", len(productEntries))

	if dotpeScraper.maxListings > 0 && len(productEntries) > dotpeScraper.maxListings {
		dotpeScraper.logger.Warn("capping this run; the catalogue is not complete",
			"cap", dotpeScraper.maxListings, "available", len(productEntries))
		productEntries = productEntries[:dotpeScraper.maxListings]
	}

	scrapedListings := make([]scraper.ScrapedListing, 0, len(productEntries))
	for _, productEntry := range productEntries {
		pageBytes, err := dotpeScraper.client.GetBytes(ctx, productEntry.Location)
		if err != nil {
			dotpeScraper.logger.Warn("skipping product page",
				"url", productEntry.Location, "error", err)
			continue
		}

		scrapedListing, err := ParseProductPage(pageBytes, productEntry.Location)
		if err != nil {
			dotpeScraper.logger.Warn("could not parse product page",
				"url", productEntry.Location, "error", err)
			continue
		}
		scrapedListings = append(scrapedListings, scrapedListing)
	}
	return scrapedListings, nil
}

// EnrichListing implements scraper.VendorScraper. FetchCatalog already reads
// each product page in full, so there is nothing left to fetch.
func (dotpeScraper *Scraper) EnrichListing(_ context.Context, _ *scraper.ScrapedListing) error {
	return nil
}

// ParseProductPage extracts one listing from a server-rendered product page.
// Exported so it can be tested against a saved fixture without the network.
func ParseProductPage(pageHTML []byte, productURL string) (scraper.ScrapedListing, error) {
	match := nextDataPattern.FindSubmatch(pageHTML)
	if match == nil {
		return scraper.ScrapedListing{},
			fmt.Errorf("no __NEXT_DATA__ payload at %s", productURL)
	}

	var parsedData nextData
	if err := json.Unmarshal(match[1], &parsedData); err != nil {
		return scraper.ScrapedListing{},
			fmt.Errorf("parsing __NEXT_DATA__ at %s: %w", productURL, err)
	}

	sourceProduct := parsedData.Props.PageProps.Product
	if sourceProduct.ID == 0 || sourceProduct.Name == "" {
		return scraper.ScrapedListing{},
			fmt.Errorf("__NEXT_DATA__ at %s carries no product", productURL)
	}

	description := sourceProduct.DescriptionDetail
	if description == "" {
		description = sourceProduct.Description
	}

	scrapedListing := scraper.ScrapedListing{
		ProductURL:         productURL,
		ExternalProductID:  strconv.FormatInt(sourceProduct.ID, 10),
		ListingName:        sourceProduct.Name,
		Description:        description,
		PrimaryImageURL:    sourceProduct.ImageURL,
		VendorSideCategory: sourceProduct.Category.Name,
		VendorSideSKU:      sourceProduct.SKUID,
		IsInStock:          sourceProduct.Available == 1,
		BasePrice:          sellingPrice(sourceProduct),
		PackSize:           scraper.ParsePackSize(sourceProduct.Name),
	}
	if scrapedListing.PrimaryImageURL == "" && len(sourceProduct.Images) > 0 {
		scrapedListing.PrimaryImageURL = sourceProduct.Images[0].ImageURL
	}

	// This storefront states no quantity-discount ladder, only a minimum order
	// quantity, so the single tier starts where the vendor allows ordering to
	// start rather than always at 1.
	if scrapedListing.BasePrice != nil {
		minimumQuantity := sourceProduct.B2BPricingInfo.MinimumOrderQuantity
		if minimumQuantity < 1 {
			minimumQuantity = 1
		}
		scrapedListing.MoqTiers = []scraper.ScrapedMoqTier{{
			QuantityRangeMinimum: minimumQuantity,
			PricePerUnit:         *scrapedListing.BasePrice,
		}}
	}

	return scrapedListing, nil
}

// sellingPrice returns what a buyer actually pays. DotPe carries both a list
// price and a discounted price; the discounted one is what the storefront
// charges when it is set, and zero means "not set" rather than free.
func sellingPrice(sourceProduct product) *decimal.Decimal {
	if discounted := parsePositiveNumber(sourceProduct.DiscountedPrice); discounted != nil {
		return discounted
	}
	return parsePositiveNumber(sourceProduct.Price)
}

func parsePositiveNumber(number json.Number) *decimal.Decimal {
	if number.String() == "" {
		return nil
	}
	parsed, err := decimal.NewFromString(number.String())
	if err != nil || !parsed.IsPositive() {
		return nil
	}
	return &parsed
}
