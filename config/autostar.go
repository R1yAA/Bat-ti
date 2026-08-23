package config

import "strings"

// AutoStarTitleTerms lists the words that star a listing on sight. A listing
// whose title contains one of them is tracked from the moment a scrape first
// sees it, without anyone having to click the star, so its MOQ tiers and daily
// price history are recorded from day one rather than from whenever someone
// remembered to follow it.
//
// This is the file to edit to follow another product line the same way.
var AutoStarTitleTerms = []string{"candle"}

// ShouldAutoStar reports whether a listing's title matches one of
// AutoStarTitleTerms.
//
// Matching is case-insensitive and on substrings, because vendors title the
// same product a dozen ways: "Scented Candle", "CANDLES - Set of 4" and
// "Candleholder" all name something worth watching the price of.
func ShouldAutoStar(listingName string) bool {
	loweredName := strings.ToLower(listingName)
	for _, term := range AutoStarTitleTerms {
		if strings.Contains(loweredName, strings.ToLower(term)) {
			return true
		}
	}
	return false
}
