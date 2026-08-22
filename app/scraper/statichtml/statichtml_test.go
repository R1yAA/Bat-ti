package statichtml

import (
	"errors"
	"os"
	"testing"

	"github.com/R1yAA/Bat-ti/config"
	"github.com/shopspring/decimal"
)

const fixtureProductURL = "https://www.restokart.com/product/1-kg-ghevar-box/1184"

func TestParseRestokartProductPage(t *testing.T) {
	pageHTML, err := os.ReadFile("testdata/restokart_ghevar_box_product.html")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	vendorConfig, err := config.FindVendorBySlug("restokart")
	if err != nil {
		t.Fatalf("loading vendor config: %v", err)
	}

	listing, err := ParseProductPage(pageHTML, fixtureProductURL, vendorConfig)
	if err != nil {
		t.Fatalf("ParseProductPage: %v", err)
	}

	if listing.ListingName != "1 kg Ghevar Box" {
		t.Errorf("listing name = %q, want %q", listing.ListingName, "1 kg Ghevar Box")
	}
	// The id comes from the URL's trailing segment, so a slug rename does not
	// create a duplicate listing.
	if listing.ExternalProductID != "1184" {
		t.Errorf("external product id = %q, want %q", listing.ExternalProductID, "1184")
	}
	// "Quantity (Pack of 100)" is what makes the per-unit price meaningful.
	if listing.PackSize == nil {
		t.Error("pack size not found, want 100")
	} else if *listing.PackSize != 100 {
		t.Errorf("pack size = %d, want 100", *listing.PackSize)
	}
	if listing.PrimaryImageURL == "" {
		t.Error("no primary image URL")
	}
	if !listing.IsInStock {
		t.Error("listing should be in stock")
	}
}

// Restokart publishes a real five-step ladder, unlike the Shopify vendors.
func TestParseRestokartTierLadder(t *testing.T) {
	pageHTML, err := os.ReadFile("testdata/restokart_ghevar_box_product.html")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	vendorConfig, _ := config.FindVendorBySlug("restokart")

	listing, err := ParseProductPage(pageHTML, fixtureProductURL, vendorConfig)
	if err != nil {
		t.Fatalf("ParseProductPage: %v", err)
	}

	expectedTiers := []struct {
		quantityMinimum int
		quantityMaximum *int
		pricePerUnit    string
		discountPercent string
	}{
		{1, intPointer(500), "35.17", ""},
		{501, intPointer(2500), "34.11", "3"},
		{2501, intPointer(7500), "33.41", "5"},
		{7501, intPointer(12000), "32.71", "7"},
		{12001, nil, "32.00", "9"},
	}

	if len(listing.MoqTiers) != len(expectedTiers) {
		t.Fatalf("got %d tiers, want %d", len(listing.MoqTiers), len(expectedTiers))
	}

	for index, expectedTier := range expectedTiers {
		actualTier := listing.MoqTiers[index]

		if actualTier.QuantityRangeMinimum != expectedTier.quantityMinimum {
			t.Errorf("tier %d minimum = %d, want %d",
				index, actualTier.QuantityRangeMinimum, expectedTier.quantityMinimum)
		}
		switch {
		case expectedTier.quantityMaximum == nil && actualTier.QuantityRangeMaximum != nil:
			t.Errorf("tier %d maximum = %d, want open-ended", index, *actualTier.QuantityRangeMaximum)
		case expectedTier.quantityMaximum != nil && actualTier.QuantityRangeMaximum == nil:
			t.Errorf("tier %d maximum open-ended, want %d", index, *expectedTier.quantityMaximum)
		case expectedTier.quantityMaximum != nil &&
			*actualTier.QuantityRangeMaximum != *expectedTier.quantityMaximum:
			t.Errorf("tier %d maximum = %d, want %d",
				index, *actualTier.QuantityRangeMaximum, *expectedTier.quantityMaximum)
		}
		if !actualTier.PricePerUnit.Equal(decimal.RequireFromString(expectedTier.pricePerUnit)) {
			t.Errorf("tier %d price = %s, want %s", index, actualTier.PricePerUnit, expectedTier.pricePerUnit)
		}

		// A stated 0% discount is no discount, and must not be stored as one.
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

	// The opening tier is the price a single unit costs.
	if listing.BasePrice == nil {
		t.Fatal("no base price")
	}
	if !listing.BasePrice.Equal(decimal.RequireFromString("35.17")) {
		t.Errorf("base price = %s, want 35.17", listing.BasePrice)
	}
}

func intPointer(value int) *int { return &value }

// Restokart answers HTTP 200 with a "PAGE NOT FOUND" body for products it has
// removed but still lists in its sitemap. That must read as "gone", not as a
// broken parser, so a real markup change stays visible in the logs.
func TestParseProductPageRecognisesASoft404(t *testing.T) {
	vendorConfig, _ := config.FindVendorBySlug("restokart")
	softNotFoundHTML := []byte(`<html><body><h1>404</h1><h2>PAGE NOT FOUND</h2></body></html>`)

	_, err := ParseProductPage(softNotFoundHTML, fixtureProductURL, vendorConfig)
	if !errors.Is(err, ErrProductGone) {
		t.Errorf("error = %v, want ErrProductGone", err)
	}

	// A page that is neither a product nor a 404 must still be a parse error.
	_, err = ParseProductPage([]byte(`<html><body><p>hello</p></body></html>`),
		fixtureProductURL, vendorConfig)
	if err == nil || errors.Is(err, ErrProductGone) {
		t.Errorf("error = %v, want a plain parse error", err)
	}
}
