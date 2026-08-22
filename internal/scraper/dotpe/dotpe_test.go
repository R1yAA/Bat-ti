package dotpe

import (
	"os"
	"testing"

	"github.com/shopspring/decimal"
)

const fixtureProductURL = "https://www.plutoniousinnovations.com/product/33200665/Diffuser-combo-pack-of-8-pcs"

// The tech PRD recorded this vendor as needing Playwright. This test is the
// evidence that it does not: everything below is parsed from a saved copy of
// the server-rendered page, with no browser involved.
func TestParseProductPageReadsServerRenderedProduct(t *testing.T) {
	pageHTML, err := os.ReadFile("testdata/plutonious_diffuser_product.html")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}

	listing, err := ParseProductPage(pageHTML, fixtureProductURL)
	if err != nil {
		t.Fatalf("ParseProductPage: %v", err)
	}

	if listing.ExternalProductID != "33200665" {
		t.Errorf("external product id = %q, want %q", listing.ExternalProductID, "33200665")
	}
	if listing.ListingName != "Diffuser combo pack of 8 pcs" {
		t.Errorf("listing name = %q", listing.ListingName)
	}
	if !listing.IsInStock {
		t.Error("listing should be in stock")
	}
	if listing.VendorSideCategory != "DIFFUSER POT" {
		t.Errorf("category = %q, want %q", listing.VendorSideCategory, "DIFFUSER POT")
	}
	if listing.PrimaryImageURL == "" {
		t.Error("no primary image URL")
	}

	// The buyer pays the discounted price (360), not the list price (400).
	if listing.BasePrice == nil {
		t.Fatal("no base price")
	}
	if !listing.BasePrice.Equal(decimal.RequireFromString("360")) {
		t.Errorf("base price = %s, want 360 (the discounted price, not 400)", listing.BasePrice)
	}

	// "pack of 8 pcs" in the name is what makes ₹45/piece computable.
	if listing.PackSize == nil {
		t.Error("pack size not found, want 8")
	} else if *listing.PackSize != 8 {
		t.Errorf("pack size = %d, want 8", *listing.PackSize)
	}

	if len(listing.MoqTiers) != 1 {
		t.Fatalf("got %d tiers, want 1", len(listing.MoqTiers))
	}
	if listing.MoqTiers[0].QuantityRangeMinimum != 1 {
		t.Errorf("tier minimum = %d, want 1", listing.MoqTiers[0].QuantityRangeMinimum)
	}
	if listing.MoqTiers[0].QuantityRangeMaximum != nil {
		t.Error("the only tier should be open-ended")
	}
}

func TestParseProductPageRejectsAPageWithoutAProduct(t *testing.T) {
	if _, err := ParseProductPage([]byte("<html><body>nothing here</body></html>"), "x"); err == nil {
		t.Error("expected an error for a page carrying no __NEXT_DATA__")
	}
}
