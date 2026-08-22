// Package shopify reads a Shopify storefront's public /products.json feed.
//
// The feed enumerates the entire catalogue — title, handle, images, and every
// variant's price and availability — in a handful of requests, which is what
// makes a daily full-catalogue sync affordable for the seven Shopify vendors.
//
// What it does not carry is a quantity-discount ladder: Shopify has no native
// concept of one, and stores that offer tiered pricing do it through an app
// that renders into the product page only. None of the tracked Shopify vendors
// use such an app today, so EnrichListing has nothing to fetch; the persistence
// layer synthesises the single "1 and above" tier that BR-4 calls for.
package shopify

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/R1yAA/Bat-ti/internal/scraper"
	"github.com/R1yAA/Bat-ti/internal/scraper/httpclient"
	"github.com/shopspring/decimal"
)

// pageSize is the maximum Shopify serves per request.
const pageSize = 250

// maximumPageCount stops a runaway loop if a storefront ever returns a full
// page forever. At 250 per page this still allows a 50,000-product catalogue.
const maximumPageCount = 200

// defaultVariantTitle is what Shopify names the single implicit variant of a
// product that has no real options. Such a product is not "a listing with one
// variant" — it is a listing whose pricing lives at the listing level (BR-4).
const defaultVariantTitle = "Default Title"

type productsResponse struct {
	Products []product `json:"products"`
}

type product struct {
	ID          int64            `json:"id"`
	Title       string           `json:"title"`
	Handle      string           `json:"handle"`
	BodyHTML    string           `json:"body_html"`
	ProductType string           `json:"product_type"`
	Variants    []productVariant `json:"variants"`
	Images      []productImage   `json:"images"`
}

type productVariant struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	SKU       string `json:"sku"`
	Available bool   `json:"available"`
	Price     string `json:"price"`
}

type productImage struct {
	Source string `json:"src"`
}

// Scraper reads one Shopify storefront.
type Scraper struct {
	sourceBaseURL string
	client        *httpclient.Client
	logger        *slog.Logger
}

// New builds a scraper for the storefront at sourceBaseURL.
func New(sourceBaseURL string, client *httpclient.Client, logger *slog.Logger) *Scraper {
	return &Scraper{
		sourceBaseURL: strings.TrimRight(sourceBaseURL, "/"),
		client:        client,
		logger:        logger,
	}
}

// TierName implements scraper.VendorScraper.
func (shopifyScraper *Scraper) TierName() string { return "shopify_json" }

// FetchCatalog implements scraper.VendorScraper.
func (shopifyScraper *Scraper) FetchCatalog(ctx context.Context) ([]scraper.ScrapedListing, error) {
	var scrapedListings []scraper.ScrapedListing

	for pageNumber := 1; pageNumber <= maximumPageCount; pageNumber++ {
		requestURL := fmt.Sprintf("%s/products.json?limit=%d&page=%d",
			shopifyScraper.sourceBaseURL, pageSize, pageNumber)

		responseBody, err := shopifyScraper.client.GetBytes(ctx, requestURL)
		if err != nil {
			return nil, fmt.Errorf("fetching catalogue page %d: %w", pageNumber, err)
		}

		var parsedResponse productsResponse
		if err := json.Unmarshal(responseBody, &parsedResponse); err != nil {
			return nil, fmt.Errorf("parsing catalogue page %d: %w", pageNumber, err)
		}

		if len(parsedResponse.Products) == 0 {
			return scrapedListings, nil
		}

		for _, currentProduct := range parsedResponse.Products {
			scrapedListings = append(scrapedListings,
				shopifyScraper.convertProduct(currentProduct))
		}

		// A short page means this was the last one; skip the extra request.
		if len(parsedResponse.Products) < pageSize {
			return scrapedListings, nil
		}
	}

	return nil, fmt.Errorf("catalogue did not end after %d pages of %d; refusing to keep paging",
		maximumPageCount, pageSize)
}

// EnrichListing implements scraper.VendorScraper. See the package comment for
// why there is nothing to fetch.
func (shopifyScraper *Scraper) EnrichListing(_ context.Context, _ *scraper.ScrapedListing) error {
	return nil
}

func (shopifyScraper *Scraper) convertProduct(sourceProduct product) scraper.ScrapedListing {
	scrapedListing := scraper.ScrapedListing{
		ProductURL:         shopifyScraper.sourceBaseURL + "/products/" + sourceProduct.Handle,
		ExternalProductID:  strconv.FormatInt(sourceProduct.ID, 10),
		ListingName:        sourceProduct.Title,
		Description:        sourceProduct.BodyHTML,
		VendorSideCategory: sourceProduct.ProductType,
		PackSize:           scraper.ParsePackSize(sourceProduct.Title),
	}

	if len(sourceProduct.Images) > 0 {
		scrapedListing.PrimaryImageURL = sourceProduct.Images[0].Source
	}

	// A product with no real options carries one variant Shopify named
	// "Default Title"; its price belongs to the listing itself.
	if len(sourceProduct.Variants) == 1 && sourceProduct.Variants[0].Title == defaultVariantTitle {
		onlyVariant := sourceProduct.Variants[0]
		scrapedListing.IsInStock = onlyVariant.Available
		scrapedListing.VendorSideSKU = onlyVariant.SKU
		scrapedListing.BasePrice = parsePrice(onlyVariant.Price, shopifyScraper.logger, sourceProduct.Handle)
		return scrapedListing
	}

	for _, sourceVariant := range sourceProduct.Variants {
		if sourceVariant.Available {
			scrapedListing.IsInStock = true
		}

		// A variant label often carries the pack size ("5pcs", "50pcs"), which
		// is what makes the per-unit price meaningful for these vendors.
		variantPackSize := scraper.ParsePackSize(sourceVariant.Title)
		if variantPackSize == nil {
			variantPackSize = scrapedListing.PackSize
		}

		scrapedListing.Variants = append(scrapedListing.Variants, scraper.ScrapedVariant{
			VariantLabel:      sourceVariant.Title,
			ExternalVariantID: strconv.FormatInt(sourceVariant.ID, 10),
			VariantSKU:        sourceVariant.SKU,
			IsInStock:         sourceVariant.Available,
			PackSize:          variantPackSize,
			Price:             parsePrice(sourceVariant.Price, shopifyScraper.logger, sourceProduct.Handle),
		})
	}

	return scrapedListing
}

// parsePrice converts Shopify's decimal-string price. A price that will not
// parse is left absent rather than defaulted to zero, so a parsing regression
// shows up as a gap in the data instead of a fictional free product.
func parsePrice(rawPrice string, logger *slog.Logger, productHandle string) *decimal.Decimal {
	trimmedPrice := strings.TrimSpace(rawPrice)
	if trimmedPrice == "" {
		return nil
	}
	parsedPrice, err := decimal.NewFromString(trimmedPrice)
	if err != nil {
		logger.Warn("could not parse price", "handle", productHandle, "raw_price", rawPrice)
		return nil
	}
	return &parsedPrice
}
