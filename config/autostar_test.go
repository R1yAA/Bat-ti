package config

import "testing"

func TestShouldAutoStar(t *testing.T) {
	testCases := []struct {
		listingName     string
		expectedStarred bool
	}{
		// Titles as the tracked vendors actually write them.
		{"Scented Candle", true},
		{"CANDLES - Set of 4", true},
		{"Soy Wax candle jar, 100g", true},
		{"Candleholder Brass", true},
		{"Diwali Diya & Candle Combo", true},

		{"Brass Diya", false},
		{"Gift Box", false},
		{"", false},
	}

	for _, testCase := range testCases {
		starred := ShouldAutoStar(testCase.listingName)
		if starred != testCase.expectedStarred {
			t.Errorf("ShouldAutoStar(%q) = %v, want %v",
				testCase.listingName, starred, testCase.expectedStarred)
		}
	}
}
