// Package config holds the editable vendor registry. Adding, retiring or
// re-tiering a vendor is a change to this file and nothing else: the scraper,
// the hour-slot dispatcher and the database seed all read from TrackedVendors.
package config

import "fmt"

// ScraperTier selects the scraping strategy for a vendor. Adding a tier here
// also requires extending the scraper_tier check constraint in
// database/migrations.
type ScraperTier string

const (
	// ScraperTierShopifyJSON reads /products.json, which enumerates the whole
	// catalogue with per-variant price and availability in a few requests.
	ScraperTierShopifyJSON ScraperTier = "shopify_json"

	// ScraperTierWooCommerceJSON reads the public WooCommerce Store API at
	// /wp-json/wc/store/v1/products. MOQ tiers are not part of that API and
	// come from the product page HTML instead.
	ScraperTierWooCommerceJSON ScraperTier = "woocommerce_json"

	// ScraperTierDotpeJSON reads DotPe storefronts: the product sitemap
	// enumerates products, and each product page server-renders its full
	// product object into a __NEXT_DATA__ script tag.
	ScraperTierDotpeJSON ScraperTier = "dotpe_json"

	// ScraperTierStaticHTML reads server-rendered pages through CSS selectors.
	ScraperTierStaticHTML ScraperTier = "static_html"

	// ScraperTierManual is reserved for vendors whose catalogue is entered by
	// hand rather than scraped (the deferred PDF-catalogue vendor).
	ScraperTierManual ScraperTier = "manual"

	// ScraperTierPlaywright is reserved. No vendor needs a headless browser
	// today; every site in TrackedVendors is reachable over plain HTTP.
	ScraperTierPlaywright ScraperTier = "playwright"
)

// ProductPageAddsDetail reports whether a vendor's product page carries
// anything its catalogue feed does not.
//
// Shopify, DotPe and static-HTML vendors publish variants and prices in the
// feed itself, so their product pages hold nothing extra and fetching one
// would be wasted work. WooCommerce publishes only a price range: the sizes,
// their individual prices and the quantity-discount ladder live on the page
// alone, so those listings are worth reading one page at a time.
func (tier ScraperTier) ProductPageAddsDetail() bool {
	return tier == ScraperTierWooCommerceJSON
}

// CatalogDiscovery tells the catalogue-sync phase how to enumerate a vendor's
// products. Shopify and WooCommerce need none of this because their APIs
// enumerate themselves.
type CatalogDiscovery struct {
	SitemapURL          string
	CollectionPageURLs  []string
	ProductLinkSelector string
}

// ListingSelectors drives goquery extraction. Static-HTML vendors use all of
// it; WooCommerce vendors use only the MOQ selectors, since their catalogue
// data arrives as JSON but their quantity-discount ladder does not.
// An empty selector is skipped, so a vendor without MOQ tiers simply leaves
// those fields blank.
type ListingSelectors struct {
	ListingName                string
	Description                string
	VendorSideCategorySelector string
	PrimaryImageURL            string
	BasePrice                  string
	PackSizeLabel              string
	StockIndicator             string
	VariantContainer           string
	VariantLabel               string
	MOQTierRow                 string
	MOQQuantityCell            string
	MOQDiscountCell            string
	MOQPriceCell               string
}

// VendorConfig is one entry in the registry.
// VendorSideCategory returns the selector for the vendor's own category label.
func (selectors ListingSelectors) VendorSideCategory() string {
	return selectors.VendorSideCategorySelector
}

type VendorConfig struct {
	// VendorSlug is the stable key shared by this file, the database
	// (vendors.vendor_slug) and the --vendor CLI flag. Never change one
	// without the others.
	VendorSlug          string
	DisplayName         string
	SourceBaseURL       string
	ScraperTier         ScraperTier
	ScrapeHourUTC       int // staggered slot; 23 UTC is 04:30 IST
	RequestDelaySeconds int // politeness gap between product-page fetches
	CatalogDiscovery    CatalogDiscovery
	ListingSelectors    ListingSelectors
}

// TrackedVendors is the registry. Hour slots run 23 UTC through 08 UTC, one
// vendor per hour, processed sequentially so two vendors are never scraped at
// the same time.
var TrackedVendors = []VendorConfig{
	{
		VendorSlug:          "royalboxshop",
		DisplayName:         "Royal Box Shop",
		SourceBaseURL:       "https://www.royalboxshop.com",
		ScraperTier:         ScraperTierShopifyJSON,
		ScrapeHourUTC:       23,
		RequestDelaySeconds: 2,
	},
	{
		VendorSlug:          "bakeyy",
		DisplayName:         "Bakeyy",
		SourceBaseURL:       "https://bakeyy.com",
		ScraperTier:         ScraperTierShopifyJSON,
		ScrapeHourUTC:       0,
		RequestDelaySeconds: 2,
	},
	{
		VendorSlug:          "paclioeolio",
		DisplayName:         "Paclio Eolio",
		SourceBaseURL:       "https://paclioeolio.com",
		ScraperTier:         ScraperTierShopifyJSON,
		ScrapeHourUTC:       1,
		RequestDelaySeconds: 2,
	},
	{
		VendorSlug:          "thecakecase",
		DisplayName:         "The Cake Case",
		SourceBaseURL:       "https://thecakecase.in",
		ScraperTier:         ScraperTierShopifyJSON,
		ScrapeHourUTC:       2,
		RequestDelaySeconds: 2,
	},
	{
		VendorSlug:          "matinimpex",
		DisplayName:         "Matin Impex",
		SourceBaseURL:       "https://matinimpex.com",
		ScraperTier:         ScraperTierShopifyJSON,
		ScrapeHourUTC:       3,
		RequestDelaySeconds: 2,
	},
	{
		VendorSlug:          "candlemould",
		DisplayName:         "Candle Mould",
		SourceBaseURL:       "https://www.candlemould.in",
		ScraperTier:         ScraperTierShopifyJSON,
		ScrapeHourUTC:       4,
		RequestDelaySeconds: 2,
	},
	{
		VendorSlug:          "dispozable",
		DisplayName:         "Dispozable",
		SourceBaseURL:       "https://dispozable.in",
		ScraperTier:         ScraperTierShopifyJSON,
		ScrapeHourUTC:       5,
		RequestDelaySeconds: 2,
	},
	{
		VendorSlug:          "jindeal",
		DisplayName:         "Jindeal",
		SourceBaseURL:       "https://jindeal.com",
		ScraperTier:         ScraperTierWooCommerceJSON,
		ScrapeHourUTC:       6,
		RequestDelaySeconds: 2,
		// Catalogue comes from the Store API; only the tier-pricing-table
		// plugin's ladder is read out of the product page HTML.
		ListingSelectors: ListingSelectors{
			MOQTierRow:      "div.tiered-pricing-wrapper table tbody tr",
			MOQQuantityCell: "td:nth-child(1)",
			MOQDiscountCell: "td:nth-child(2)",
			MOQPriceCell:    "td:nth-child(3)",
		},
	},
	{
		VendorSlug:          "plutonious",
		DisplayName:         "Plutonious Innovations",
		SourceBaseURL:       "https://www.plutoniousinnovations.com",
		ScraperTier:         ScraperTierDotpeJSON,
		ScrapeHourUTC:       7,
		RequestDelaySeconds: 2,
		CatalogDiscovery: CatalogDiscovery{
			SitemapURL: "https://www.plutoniousinnovations.com/sitemap_products.xml",
		},
	},
	{
		VendorSlug:          "restokart",
		DisplayName:         "Restokart",
		SourceBaseURL:       "https://www.restokart.com",
		ScraperTier:         ScraperTierStaticHTML,
		ScrapeHourUTC:       8,
		RequestDelaySeconds: 2,
		CatalogDiscovery: CatalogDiscovery{
			SitemapURL: "https://www.restokart.com/sitemap.xml",
		},
		// Captured from the live product page; the fixture under
		// internal/scraper/statichtml/testdata guards them against drift.
		//
		// The tier table carries no class or id of its own, so the row
		// selector is deliberately broad: the parser keeps only rows whose
		// first cell reads as a quantity range, which filters out the header
		// row and every unrelated table on the page.
		ListingSelectors: ListingSelectors{
			ListingName:                "h3.product-title",
			Description:                ".product-details-article",
			PrimaryImageURL:            ".product-details-slider img",
			BasePrice:                  ".price-block .price-new",
			PackSizeLabel:              "h5.product-title",
			VendorSideCategorySelector: "",
			MOQTierRow:                 "table tr",
			MOQQuantityCell:            "td:nth-child(1)",
			MOQDiscountCell:            "td:nth-child(2)",
			MOQPriceCell:               "td:nth-child(3)",
		},
	},
}

// FindVendorBySlug returns the registry entry for a slug.
func FindVendorBySlug(vendorSlug string) (VendorConfig, error) {
	for _, vendorConfig := range TrackedVendors {
		if vendorConfig.VendorSlug == vendorSlug {
			return vendorConfig, nil
		}
	}
	return VendorConfig{}, fmt.Errorf("no vendor in the registry with slug %q", vendorSlug)
}
