// Package money holds the pricing rules that several handlers need and that
// must not be re-derived differently in each of them.
package money

import "github.com/shopspring/decimal"

// PriceDirection describes how a price moved since the previous scrape.
type PriceDirection string

const (
	// PriceDirectionUp drives the red up-arrow (FR-P1-2).
	PriceDirectionUp PriceDirection = "up"
	// PriceDirectionDown drives the green down-arrow.
	PriceDirectionDown PriceDirection = "down"
	// PriceDirectionNone means no icon: either nothing changed, or there is no
	// earlier price to compare against.
	PriceDirectionNone PriceDirection = "none"
)

// DirectionOf compares the current price with the previous one.
//
// BR-16 sets no minimum threshold: any change at all shows an arrow. A listing
// seen for the first time has no previous price and therefore no arrow, which
// is why the absence of a previous price is a distinct case rather than a zero.
func DirectionOf(currentPrice *decimal.Decimal, previousPrice *decimal.Decimal) PriceDirection {
	if currentPrice == nil || previousPrice == nil {
		return PriceDirectionNone
	}
	switch currentPrice.Cmp(*previousPrice) {
	case 1:
		return PriceDirectionUp
	case -1:
		return PriceDirectionDown
	default:
		return PriceDirectionNone
	}
}

// PerUnit divides a pack price by the number of units in the pack, which is the
// figure BR-5 shows in small text beneath the headline price.
//
// It returns nil when there is no pack size, rather than echoing the pack price
// back: showing "₹500" as both the pack price and the per-unit price would be a
// quietly wrong claim about what one unit costs. A caller that wants to fall
// back to the headline price should say so itself.
func PerUnit(packPrice *decimal.Decimal, packSize *int) *decimal.Decimal {
	if packPrice == nil || packSize == nil || *packSize <= 1 {
		return nil
	}
	perUnitPrice := packPrice.Div(decimal.NewFromInt(int64(*packSize))).Round(2)
	return &perUnitPrice
}

// TierFor returns the price that applies at a given order quantity, or nil when
// no tier covers it. Tiers are expected in ascending order of minimum quantity,
// which is how both the scrapers build them and the queries return them.
func TierFor(orderQuantity int, tierMinimums []int, tierMaximums []*int, tierPrices []decimal.Decimal) *decimal.Decimal {
	for index := range tierMinimums {
		if orderQuantity < tierMinimums[index] {
			continue
		}
		if tierMaximums[index] != nil && orderQuantity > *tierMaximums[index] {
			continue
		}
		return &tierPrices[index]
	}
	return nil
}
