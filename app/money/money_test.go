package money

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestDirectionOf(t *testing.T) {
	price := func(text string) *decimal.Decimal {
		parsed := decimal.RequireFromString(text)
		return &parsed
	}

	testCases := []struct {
		name              string
		currentPrice      *decimal.Decimal
		previousPrice     *decimal.Decimal
		expectedDirection PriceDirection
	}{
		{"a rise shows the up arrow", price("40"), price("39"), PriceDirectionUp},
		{"a fall shows the down arrow", price("36"), price("39"), PriceDirectionDown},
		{"no change shows nothing", price("39"), price("39"), PriceDirectionNone},
		// BR-16 sets no minimum threshold.
		{"a one paisa rise still counts", price("39.01"), price("39.00"), PriceDirectionUp},
		// A first sighting has nothing to compare against.
		{"no previous price shows nothing", price("39"), nil, PriceDirectionNone},
		{"no current price shows nothing", nil, price("39"), PriceDirectionNone},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			actual := DirectionOf(testCase.currentPrice, testCase.previousPrice)
			if actual != testCase.expectedDirection {
				t.Errorf("got %q, want %q", actual, testCase.expectedDirection)
			}
		})
	}
}

func TestPerUnit(t *testing.T) {
	packPrice := decimal.RequireFromString("8064")
	packSize := 504

	perUnit := PerUnit(&packPrice, &packSize)
	if perUnit == nil {
		t.Fatal("got no per-unit price")
	}
	if !perUnit.Equal(decimal.RequireFromString("16")) {
		t.Errorf("per-unit = %s, want 16", perUnit)
	}

	// A single-unit listing has no meaningful per-unit price distinct from its
	// headline price, and claiming one would be misleading.
	singleUnit := 1
	if PerUnit(&packPrice, &singleUnit) != nil {
		t.Error("a pack of one should have no separate per-unit price")
	}
	if PerUnit(&packPrice, nil) != nil {
		t.Error("no pack size should mean no per-unit price")
	}
	if PerUnit(nil, &packSize) != nil {
		t.Error("no price should mean no per-unit price")
	}
}

// The ladder from the PRD's worked example.
func TestTierFor(t *testing.T) {
	upperBound := func(value int) *int { return &value }
	minimums := []int{1, 10, 100}
	maximums := []*int{upperBound(9), upperBound(99), nil}
	prices := []decimal.Decimal{
		decimal.RequireFromString("39"),
		decimal.RequireFromString("36"),
		decimal.RequireFromString("35"),
	}

	testCases := []struct {
		orderQuantity int
		expectedPrice string
	}{
		{1, "39"}, {9, "39"},
		{10, "36"}, {99, "36"},
		{100, "35"}, {5000, "35"},
	}
	for _, testCase := range testCases {
		actual := TierFor(testCase.orderQuantity, minimums, maximums, prices)
		if actual == nil {
			t.Errorf("quantity %d matched no tier", testCase.orderQuantity)
			continue
		}
		if !actual.Equal(decimal.RequireFromString(testCase.expectedPrice)) {
			t.Errorf("quantity %d priced at %s, want %s",
				testCase.orderQuantity, actual, testCase.expectedPrice)
		}
	}

	// A ladder that starts above 1 — Plutonious states a minimum order
	// quantity — leaves smaller quantities unpriced rather than inventing one.
	if TierFor(0, minimums, maximums, prices) != nil {
		t.Error("quantity 0 should match no tier")
	}
}
