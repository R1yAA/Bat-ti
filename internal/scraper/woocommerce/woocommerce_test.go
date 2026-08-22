package woocommerce

import (
	"os"
	"testing"

	"github.com/R1yAA/Bat-ti/internal/scraper"
	"github.com/shopspring/decimal"
)

// The fixture is a real capture of Jindeal's "Vedini Decorative Lotus Urli
// Bowl" page — the listing both PRDs use to define BR-4 and BR-5. Parsing runs
// entirely offline, so a change to the vendor's markup shows up as a failing
// test rather than as silent nulls in the database.
func loadFixtureListing(t *testing.T) *scraper.ScrapedListing {
	t.Helper()
	pageHTML, err := os.ReadFile("testdata/jindeal_lotus_urli_product.html")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	listing := &scraper.ScrapedListing{ExternalProductID: "62321"}
	if err := ParseProductPage(pageHTML, listing); err != nil {
		t.Fatalf("ParseProductPage: %v", err)
	}
	return listing
}

func TestParseProductPageFindsEveryVariant(t *testing.T) {
	listing := loadFixtureListing(t)

	if !listing.HasVariants() {
		t.Fatal("expected the lotus urli listing to have variants")
	}
	if len(listing.Variants) != 4 {
		t.Fatalf("got %d variants, want 4", len(listing.Variants))
	}

	// Labels must be the human-readable option text ("5.5 Inch"), not the
	// attribute slug ("5-5-inch").
	expectedPriceByLabel := map[string]string{
		"3.5 Inch / 1 Pc": "39",
		"4.5 Inch / 1 Pc": "49",
		"5.5 Inch / 1 Pc": "55",
		"6.5 Inch / 1 Pc": "65",
	}
	for _, variant := range listing.Variants {
		expectedPrice, known := expectedPriceByLabel[variant.VariantLabel]
		if !known {
			t.Errorf("unexpected variant label %q", variant.VariantLabel)
			continue
		}
		if variant.Price == nil {
			t.Errorf("variant %q has no price", variant.VariantLabel)
			continue
		}
		if !variant.Price.Equal(decimal.RequireFromString(expectedPrice)) {
			t.Errorf("variant %q price = %s, want %s",
				variant.VariantLabel, variant.Price, expectedPrice)
		}
		if !variant.IsInStock {
			t.Errorf("variant %q should be in stock", variant.VariantLabel)
		}
		if variant.VariantSKU == "" {
			t.Errorf("variant %q has no SKU", variant.VariantLabel)
		}
	}
}

// The exact ladder both PRDs quote as the worked example of BR-4/BR-5.
//
// The PRDs attribute it to the 5.5 inch variant; on the live page that ladder
// belongs to the 3.5 inch variant, and 5.5 inch now sits at ₹55. The shape is
// what the rule is about, and the shape matches.
func TestParseProductPageBuildsThePrdTierLadder(t *testing.T) {
	listing := loadFixtureListing(t)

	var smallestVariant *scraper.ScrapedVariant
	for index := range listing.Variants {
		if listing.Variants[index].VariantLabel == "3.5 Inch / 1 Pc" {
			smallestVariant = &listing.Variants[index]
		}
	}
	if smallestVariant == nil {
		t.Fatal(`no variant labelled "3.5 Inch / 1 Pc"`)
	}

	expectedTiers := []struct {
		quantityMinimum int
		quantityMaximum *int
		pricePerUnit    string
		discountPercent string
	}{
		{1, intPointer(9), "39", ""},
		{10, intPointer(99), "36", "7.69"},
		{100, nil, "35", "10.26"},
	}

	if len(smallestVariant.MoqTiers) != len(expectedTiers) {
		t.Fatalf("got %d tiers, want %d", len(smallestVariant.MoqTiers), len(expectedTiers))
	}

	for index, expectedTier := range expectedTiers {
		actualTier := smallestVariant.MoqTiers[index]

		if actualTier.QuantityRangeMinimum != expectedTier.quantityMinimum {
			t.Errorf("tier %d minimum = %d, want %d",
				index, actualTier.QuantityRangeMinimum, expectedTier.quantityMinimum)
		}
		switch {
		case expectedTier.quantityMaximum == nil && actualTier.QuantityRangeMaximum != nil:
			t.Errorf("tier %d maximum = %d, want open-ended",
				index, *actualTier.QuantityRangeMaximum)
		case expectedTier.quantityMaximum != nil && actualTier.QuantityRangeMaximum == nil:
			t.Errorf("tier %d maximum is open-ended, want %d", index, *expectedTier.quantityMaximum)
		case expectedTier.quantityMaximum != nil &&
			*actualTier.QuantityRangeMaximum != *expectedTier.quantityMaximum:
			t.Errorf("tier %d maximum = %d, want %d",
				index, *actualTier.QuantityRangeMaximum, *expectedTier.quantityMaximum)
		}
		if !actualTier.PricePerUnit.Equal(decimal.RequireFromString(expectedTier.pricePerUnit)) {
			t.Errorf("tier %d price = %s, want %s",
				index, actualTier.PricePerUnit, expectedTier.pricePerUnit)
		}

		if expectedTier.discountPercent == "" {
			if actualTier.DiscountPercent != nil {
				t.Errorf("tier %d discount = %s, want none", index, actualTier.DiscountPercent)
			}
			continue
		}
		if actualTier.DiscountPercent == nil {
			t.Errorf("tier %d has no discount, want %s", index, expectedTier.discountPercent)
			continue
		}
		if !actualTier.DiscountPercent.Equal(decimal.RequireFromString(expectedTier.discountPercent)) {
			t.Errorf("tier %d discount = %s, want %s",
				index, actualTier.DiscountPercent, expectedTier.discountPercent)
		}
	}
}

// Every variant must get its own ladder, never a shared or default one (BR-4).
func TestParseProductPageGivesEachVariantItsOwnLadder(t *testing.T) {
	listing := loadFixtureListing(t)

	expectedFirstBreakPrice := map[string]string{
		"3.5 Inch / 1 Pc": "36",
		"4.5 Inch / 1 Pc": "45",
		"5.5 Inch / 1 Pc": "52",
		"6.5 Inch / 1 Pc": "62",
	}
	for _, variant := range listing.Variants {
		if len(variant.MoqTiers) < 2 {
			t.Errorf("variant %q has %d tiers, want a real ladder",
				variant.VariantLabel, len(variant.MoqTiers))
			continue
		}
		secondTier := variant.MoqTiers[1]
		if secondTier.QuantityRangeMinimum != 10 {
			t.Errorf("variant %q first break at %d, want 10",
				variant.VariantLabel, secondTier.QuantityRangeMinimum)
		}
		wantPrice, known := expectedFirstBreakPrice[variant.VariantLabel]
		if !known {
			t.Errorf("unexpected variant label %q", variant.VariantLabel)
			continue
		}
		if !secondTier.PricePerUnit.Equal(decimal.RequireFromString(wantPrice)) {
			t.Errorf("variant %q first-break price = %s, want %s",
				variant.VariantLabel, secondTier.PricePerUnit, wantPrice)
		}
	}
}

func TestParseMinorUnitPrice(t *testing.T) {
	// The Store API quotes minor units: "5500" is ₹55.00, not ₹5,500.
	if price := parseMinorUnitPrice(storePrices{Price: "5500", CurrencyMinorUnit: 2}); price == nil {
		t.Error("5500 minor units parsed as no price")
	} else if !price.Equal(decimal.RequireFromString("55")) {
		t.Errorf("5500 minor units = %s, want 55", price)
	}

	// A zero price on this storefront means "no listed price", not free.
	if price := parseMinorUnitPrice(storePrices{Price: "0", CurrencyMinorUnit: 2}); price != nil {
		t.Errorf("zero price parsed as %s, want no price", price)
	}
	if price := parseMinorUnitPrice(storePrices{Price: "", CurrencyMinorUnit: 2}); price != nil {
		t.Errorf("empty price parsed as %s, want no price", price)
	}
}

func intPointer(value int) *int { return &value }
