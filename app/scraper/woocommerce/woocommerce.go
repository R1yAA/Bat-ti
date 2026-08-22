// Package woocommerce reads a WooCommerce storefront through its public Store
// API, plus the product page for quantity-discount ladders.
//
// Two data sources, because they carry different things:
//
//	/wp-json/wc/store/v1/products   the whole catalogue as JSON — name, images,
//	                                categories, SKU, stock, price. Cheap, and
//	                                what the daily catalogue sync uses.
//	the product page HTML           per-variation prices and the tier ladder
//	                                from the tier-pricing-table plugin. One
//	                                request per starred listing.
//
// A caution that has already caused one bug: the Store API quotes prices in
// minor units ("5500" is ₹55.00) while the product page quotes them in whole
// rupees (55). Each parser below converts from its own source's convention.
package woocommerce

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/R1yAA/Bat-ti/app/scraper"
	"github.com/R1yAA/Bat-ti/app/scraper/httpclient"
	"github.com/shopspring/decimal"
)

// storeAPIPageSize is the maximum the Store API accepts.
const storeAPIPageSize = 100

// maximumPageCount bounds the catalogue loop. At 100 per page this allows a
// 10,000-product catalogue; Jindeal currently lists around 2,300.
const maximumPageCount = 100

type storeProduct struct {
	ID               int64            `json:"id"`
	Name             string           `json:"name"`
	Slug             string           `json:"slug"`
	Permalink        string           `json:"permalink"`
	Type             string           `json:"type"`
	SKU              string           `json:"sku"`
	ShortDescription string           `json:"short_description"`
	Description      string           `json:"description"`
	IsInStock        bool             `json:"is_in_stock"`
	Prices           storePrices      `json:"prices"`
	Images           []storeImage     `json:"images"`
	Categories       []storeCategory  `json:"categories"`
	Variations       []storeVariation `json:"variations"`
}

type storePrices struct {
	Price             string `json:"price"`
	CurrencyMinorUnit int    `json:"currency_minor_unit"`
}

type storeImage struct {
	Source string `json:"src"`
}

type storeCategory struct {
	Name string `json:"name"`
}

type storeVariation struct {
	ID int64 `json:"id"`
}

// pageVariation is one entry of the JSON WooCommerce embeds in the product
// page's variations form. Prices here are whole rupees, not minor units.
type pageVariation struct {
	VariationID  int64             `json:"variation_id"`
	Attributes   map[string]string `json:"attributes"`
	DisplayPrice json.Number       `json:"display_price"`
	SKU          string            `json:"sku"`
	IsInStock    bool              `json:"is_in_stock"`
}

// Scraper reads one WooCommerce storefront.
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
func (wooScraper *Scraper) TierName() string { return "woocommerce_json" }

// FetchCatalog implements scraper.VendorScraper.
//
// Variants are deliberately not emitted here. Their prices are only available
// from the product page, one request per product, which would mean thousands
// of requests a day for a catalogue the user mostly does not track. The
// listing carries the vendor's own "from" price so the catalogue view is
// complete; starring a listing is what fills in its variants (see
// EnrichListing).
func (wooScraper *Scraper) FetchCatalog(ctx context.Context) ([]scraper.ScrapedListing, error) {
	var scrapedListings []scraper.ScrapedListing

	for pageNumber := 1; pageNumber <= maximumPageCount; pageNumber++ {
		requestURL := fmt.Sprintf("%s/wp-json/wc/store/v1/products?per_page=%d&page=%d",
			wooScraper.sourceBaseURL, storeAPIPageSize, pageNumber)

		responseBody, err := wooScraper.client.GetBytes(ctx, requestURL)
		if err != nil {
			return nil, fmt.Errorf("fetching catalogue page %d: %w", pageNumber, err)
		}

		var pageProducts []storeProduct
		if err := json.Unmarshal(responseBody, &pageProducts); err != nil {
			return nil, fmt.Errorf("parsing catalogue page %d: %w", pageNumber, err)
		}
		if len(pageProducts) == 0 {
			return scrapedListings, nil
		}

		for _, currentProduct := range pageProducts {
			scrapedListings = append(scrapedListings, wooScraper.convertProduct(currentProduct))
		}
		if len(pageProducts) < storeAPIPageSize {
			return scrapedListings, nil
		}
	}

	return nil, fmt.Errorf("catalogue did not end after %d pages of %d; refusing to keep paging",
		maximumPageCount, storeAPIPageSize)
}

func (wooScraper *Scraper) convertProduct(sourceProduct storeProduct) scraper.ScrapedListing {
	productURL := sourceProduct.Permalink
	if productURL == "" {
		productURL = wooScraper.sourceBaseURL + "/product/" + sourceProduct.Slug + "/"
	}

	description := sourceProduct.Description
	if description == "" {
		description = sourceProduct.ShortDescription
	}

	scrapedListing := scraper.ScrapedListing{
		ProductURL:        productURL,
		ExternalProductID: strconv.FormatInt(sourceProduct.ID, 10),
		ListingName:       sourceProduct.Name,
		Description:       description,
		VendorSideSKU:     sourceProduct.SKU,
		IsInStock:         sourceProduct.IsInStock,
		BasePrice:         parseMinorUnitPrice(sourceProduct.Prices),
		PackSize:          scraper.ParsePackSize(sourceProduct.Name),
	}
	if len(sourceProduct.Images) > 0 {
		scrapedListing.PrimaryImageURL = sourceProduct.Images[0].Source
	}
	if len(sourceProduct.Categories) > 0 {
		scrapedListing.VendorSideCategory = sourceProduct.Categories[0].Name
	}
	return scrapedListing
}

// EnrichListing implements scraper.VendorScraper: one fetch of the product
// page yields per-variation prices, human-readable variant labels and the
// quantity-discount ladder for each.
func (wooScraper *Scraper) EnrichListing(ctx context.Context, listing *scraper.ScrapedListing) error {
	pageBytes, err := wooScraper.client.GetBytes(ctx, listing.ProductURL)
	if err != nil {
		return fmt.Errorf("fetching product page: %w", err)
	}
	return ParseProductPage(pageBytes, listing)
}

// ParseProductPage fills variants and MOQ tiers into listing from product page
// HTML. Exported so the parser can be tested against a saved fixture without
// touching the network.
func ParseProductPage(pageHTML []byte, listing *scraper.ScrapedListing) error {
	document, err := goquery.NewDocumentFromReader(strings.NewReader(string(pageHTML)))
	if err != nil {
		return fmt.Errorf("parsing product page HTML: %w", err)
	}

	priceRulesByProductID := extractPriceRules(document)
	attributeLabels, attributeOrder := extractAttributeLabels(document)
	pageVariations := extractPageVariations(document)

	// A simple product: its ladder, if any, is filed under the product's own id.
	if len(pageVariations) == 0 {
		if listing.BasePrice != nil {
			listing.MoqTiers = buildTiers(*listing.BasePrice, priceRulesByProductID[listing.ExternalProductID])
		}
		return nil
	}

	listing.Variants = nil
	for _, variation := range pageVariations {
		variationID := strconv.FormatInt(variation.VariationID, 10)

		variantPrice, err := decimal.NewFromString(variation.DisplayPrice.String())
		if err != nil {
			continue
		}

		scrapedVariant := scraper.ScrapedVariant{
			VariantLabel:      composeVariantLabel(variation.Attributes, attributeLabels, attributeOrder),
			ExternalVariantID: variationID,
			VariantSKU:        variation.SKU,
			IsInStock:         variation.IsInStock,
			Price:             &variantPrice,
			MoqTiers:          buildTiers(variantPrice, priceRulesByProductID[variationID]),
		}
		scrapedVariant.PackSize = scraper.ParsePackSize(scrapedVariant.VariantLabel)
		if scrapedVariant.PackSize == nil {
			scrapedVariant.PackSize = listing.PackSize
		}
		listing.Variants = append(listing.Variants, scrapedVariant)
	}
	return nil
}

// extractPriceRules reads the tier-pricing-table plugin's own data attribute,
// which maps a minimum quantity to its unit price:
//
//	data-price-rules="{&quot;10&quot;:36,&quot;100&quot;:35}"
//
// This is preferred over scraping the rendered table: it needs no assumption
// about column order, carries no currency symbol or thousands separator to
// strip, and is keyed by variation id so each variant's ladder is unambiguous.
func extractPriceRules(document *goquery.Document) map[string]map[int]decimal.Decimal {
	priceRulesByProductID := make(map[string]map[int]decimal.Decimal)

	document.Find("table[data-tiered-pricing-table]").Each(func(_ int, table *goquery.Selection) {
		productID, hasProductID := table.Attr("data-product-id")
		rawRules, hasRules := table.Attr("data-price-rules")
		if !hasProductID || !hasRules {
			return
		}
		// The same ladder is rendered more than once on some themes.
		if _, alreadySeen := priceRulesByProductID[productID]; alreadySeen {
			return
		}

		var rawRuleMap map[string]json.Number
		if err := json.Unmarshal([]byte(rawRules), &rawRuleMap); err != nil {
			return
		}

		parsedRules := make(map[int]decimal.Decimal, len(rawRuleMap))
		for quantityText, priceNumber := range rawRuleMap {
			minimumQuantity, err := strconv.Atoi(quantityText)
			if err != nil || minimumQuantity < 1 {
				continue
			}
			tierPrice, err := decimal.NewFromString(priceNumber.String())
			if err != nil {
				continue
			}
			parsedRules[minimumQuantity] = tierPrice
		}
		if len(parsedRules) > 0 {
			priceRulesByProductID[productID] = parsedRules
		}
	})

	return priceRulesByProductID
}

// extractAttributeLabels maps an attribute's slug to the label a shopper sees,
// e.g. "5-5-inch" to "5.5 Inch", read from the variation selects. It also
// returns the attribute names in the order the page presents them.
//
// That order matters: the vendor puts size before quantity, so labels read
// "5.5 Inch / 1 Pc". Sorting the attribute keys instead would produce
// "1 Pc / 5.5 Inch", because "quantity" sorts before "size".
func extractAttributeLabels(document *goquery.Document) (map[string]string, []string) {
	attributeLabels := make(map[string]string)
	var attributeOrder []string

	document.Find("select[name^='attribute_']").Each(func(_ int, selectElement *goquery.Selection) {
		attributeName, hasName := selectElement.Attr("name")
		if hasName {
			attributeOrder = append(attributeOrder, attributeName)
		}
		selectElement.Find("option").Each(func(_ int, option *goquery.Selection) {
			optionValue, hasValue := option.Attr("value")
			if !hasValue || optionValue == "" {
				return
			}
			attributeLabels[optionValue] = strings.TrimSpace(option.Text())
		})
	})
	return attributeLabels, attributeOrder
}

func extractPageVariations(document *goquery.Document) []pageVariation {
	rawVariations, hasVariations := document.Find("form.variations_form").First().
		Attr("data-product_variations")
	if !hasVariations || rawVariations == "" || rawVariations == "false" {
		return nil
	}
	var variations []pageVariation
	if err := json.Unmarshal([]byte(rawVariations), &variations); err != nil {
		return nil
	}
	return variations
}

// composeVariantLabel turns {"attribute_pa_size": "5-5-inch"} into "5.5 Inch",
// falling back to the slug when no select carried a label for it. Parts follow
// the page's own attribute order; anything the page did not present is
// appended alphabetically so the label stays stable between scrapes.
func composeVariantLabel(
	attributes map[string]string,
	attributeLabels map[string]string,
	attributeOrder []string,
) string {
	attributeNames := make([]string, 0, len(attributes))
	alreadyOrdered := make(map[string]bool, len(attributeOrder))
	for _, attributeName := range attributeOrder {
		if _, present := attributes[attributeName]; present {
			attributeNames = append(attributeNames, attributeName)
			alreadyOrdered[attributeName] = true
		}
	}
	remainingNames := make([]string, 0, len(attributes))
	for attributeName := range attributes {
		if !alreadyOrdered[attributeName] {
			remainingNames = append(remainingNames, attributeName)
		}
	}
	sort.Strings(remainingNames)
	attributeNames = append(attributeNames, remainingNames...)

	labelParts := make([]string, 0, len(attributeNames))
	for _, attributeName := range attributeNames {
		attributeValue := attributes[attributeName]
		if attributeValue == "" {
			continue
		}
		if humanLabel, found := attributeLabels[attributeValue]; found && humanLabel != "" {
			labelParts = append(labelParts, humanLabel)
			continue
		}
		labelParts = append(labelParts, attributeValue)
	}
	if len(labelParts) == 0 {
		return "Default"
	}
	return strings.Join(labelParts, " / ")
}

// buildTiers turns a base price and a quantity-to-price rule map into the
// contiguous ladder the product expects. The plugin states only the discounted
// steps, so the opening "1 to just below the first break" tier is implied by
// the base price and is added here.
func buildTiers(basePrice decimal.Decimal, priceRules map[int]decimal.Decimal) []scraper.ScrapedMoqTier {
	if len(priceRules) == 0 {
		return []scraper.ScrapedMoqTier{{
			QuantityRangeMinimum: 1,
			PricePerUnit:         basePrice,
		}}
	}

	minimumQuantities := make([]int, 0, len(priceRules))
	for minimumQuantity := range priceRules {
		minimumQuantities = append(minimumQuantities, minimumQuantity)
	}
	sort.Ints(minimumQuantities)

	tiers := make([]scraper.ScrapedMoqTier, 0, len(minimumQuantities)+1)

	if firstBreak := minimumQuantities[0]; firstBreak > 1 {
		openingMaximum := firstBreak - 1
		tiers = append(tiers, scraper.ScrapedMoqTier{
			QuantityRangeMinimum: 1,
			QuantityRangeMaximum: &openingMaximum,
			PricePerUnit:         basePrice,
		})
	}

	for index, minimumQuantity := range minimumQuantities {
		tier := scraper.ScrapedMoqTier{
			QuantityRangeMinimum: minimumQuantity,
			PricePerUnit:         priceRules[minimumQuantity],
		}
		if index+1 < len(minimumQuantities) {
			tierMaximum := minimumQuantities[index+1] - 1
			tier.QuantityRangeMaximum = &tierMaximum
		}
		tier.DiscountPercent = discountFromBase(basePrice, tier.PricePerUnit)
		tiers = append(tiers, tier)
	}
	return tiers
}

// discountFromBase computes the saving a tier represents. The plugin renders
// this percentage itself, but deriving it avoids parsing a formatted cell and
// produces the same number: 39 to 36 is 7.69%, 39 to 35 is 10.26%.
func discountFromBase(basePrice decimal.Decimal, tierPrice decimal.Decimal) *decimal.Decimal {
	if !basePrice.IsPositive() || tierPrice.GreaterThanOrEqual(basePrice) {
		return nil
	}
	discount := basePrice.Sub(tierPrice).
		Div(basePrice).
		Mul(decimal.NewFromInt(100)).
		Round(2)
	return &discount
}

// parseMinorUnitPrice converts the Store API's minor-unit price string. A zero
// price on this storefront means "not for sale at a listed price" rather than
// free, so it is reported as absent instead of ₹0.
func parseMinorUnitPrice(prices storePrices) *decimal.Decimal {
	trimmedPrice := strings.TrimSpace(prices.Price)
	if trimmedPrice == "" {
		return nil
	}
	parsedPrice, err := decimal.NewFromString(trimmedPrice)
	if err != nil {
		return nil
	}
	if !parsedPrice.IsPositive() {
		return nil
	}
	if prices.CurrencyMinorUnit > 0 {
		parsedPrice = parsedPrice.Shift(-int32(prices.CurrencyMinorUnit))
	}
	return &parsedPrice
}
