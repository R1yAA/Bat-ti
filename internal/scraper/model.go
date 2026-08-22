// Package scraper defines the vendor-neutral shape that every scraping tier
// produces, so the persistence layer never needs to know whether a listing
// arrived from a Shopify JSON endpoint, a WooCommerce Store API, a DotPe
// storefront or parsed HTML.
package scraper

import (
	"context"

	"github.com/shopspring/decimal"
)

// ScrapedMoqTier is one row of a quantity-discount ladder (BR-4, BR-5).
type ScrapedMoqTier struct {
	QuantityRangeMinimum int
	// Nil means "and above", e.g. the "100+" row.
	QuantityRangeMaximum *int
	PricePerUnit         decimal.Decimal
	DiscountPercent      *decimal.Decimal
}

// ScrapedVariant is a sub-option of a listing, such as a size.
type ScrapedVariant struct {
	VariantLabel      string
	ExternalVariantID string
	VariantSKU        string
	IsInStock         bool
	// Nil unless the variant is sold in packs.
	PackSize *int
	Price    *decimal.Decimal
	MoqTiers []ScrapedMoqTier
}

// ScrapedListing is one product page on one vendor's site.
type ScrapedListing struct {
	ProductURL         string
	ExternalProductID  string
	ListingName        string
	Description        string
	PrimaryImageURL    string
	VendorSideCategory string
	VendorSideSKU      string
	IsInStock          bool
	PackSize           *int

	// BasePrice and MoqTiers apply only when the listing has no variants;
	// otherwise pricing lives on each variant (BR-4).
	BasePrice *decimal.Decimal
	MoqTiers  []ScrapedMoqTier

	Variants []ScrapedVariant
}

// HasVariants reports whether pricing is tracked per variant for this listing.
func (listing ScrapedListing) HasVariants() bool {
	return len(listing.Variants) > 0
}

// VendorScraper is implemented once per platform. Every vendor in the registry
// maps to exactly one implementation.
type VendorScraper interface {
	// TierName identifies the implementation in logs and scrape_runs rows.
	TierName() string

	// FetchCatalog enumerates every listing the vendor sells, with the fields
	// the catalogue view needs: name, image, stock and price. It is run for
	// every listing on every scrape, so it must stay cheap.
	FetchCatalog(ctx context.Context) ([]ScrapedListing, error)

	// EnrichListing fills in whatever only the product page can supply —
	// principally the MOQ tier ladder. It runs only for listings the user has
	// starred, because it costs one request per product.
	//
	// Implementations whose FetchCatalog already reads each product page have
	// nothing left to do and return nil.
	EnrichListing(ctx context.Context, listing *ScrapedListing) error
}
