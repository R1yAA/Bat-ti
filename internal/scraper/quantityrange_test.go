package scraper

import "testing"

func TestParseQuantityRange(t *testing.T) {
	testCases := []struct {
		inputText       string
		expectedMinimum int
		expectedMaximum *int
		expectedOK      bool
	}{
		// Jindeal's rendered ladder.
		{"1 - 9", 1, intPointer(9), true},
		{"10 - 99", 10, intPointer(99), true},
		{"100+", 100, nil, true},
		// Restokart's ladder.
		{"1 - 500", 1, intPointer(500), true},
		{"501 - 2500", 501, intPointer(2500), true},
		{"12001+", 12001, nil, true},
		// Shapes other themes use.
		{"10–99", 10, intPointer(99), true},
		{"1,000 - 4,999", 1000, intPointer(4999), true},
		{"50 and above", 50, nil, true},
		{"5", 5, intPointer(5), true},

		// Header rows and noise must be rejected so they never become tiers.
		{"Pieces Count", 0, nil, false},
		{"Quantity", 0, nil, false},
		{"", 0, nil, false},
		{"—", 0, nil, false},
		{"₹35.17", 0, nil, false},
		{"0 - 10", 0, nil, false},
		{"99 - 10", 0, nil, false},
	}

	for _, testCase := range testCases {
		actualMinimum, actualMaximum, actualOK := ParseQuantityRange(testCase.inputText)

		if actualOK != testCase.expectedOK {
			t.Errorf("ParseQuantityRange(%q) ok = %v, want %v",
				testCase.inputText, actualOK, testCase.expectedOK)
			continue
		}
		if !testCase.expectedOK {
			continue
		}
		if actualMinimum != testCase.expectedMinimum {
			t.Errorf("ParseQuantityRange(%q) minimum = %d, want %d",
				testCase.inputText, actualMinimum, testCase.expectedMinimum)
		}
		switch {
		case testCase.expectedMaximum == nil && actualMaximum != nil:
			t.Errorf("ParseQuantityRange(%q) maximum = %d, want open-ended",
				testCase.inputText, *actualMaximum)
		case testCase.expectedMaximum != nil && actualMaximum == nil:
			t.Errorf("ParseQuantityRange(%q) maximum is open-ended, want %d",
				testCase.inputText, *testCase.expectedMaximum)
		case testCase.expectedMaximum != nil && *actualMaximum != *testCase.expectedMaximum:
			t.Errorf("ParseQuantityRange(%q) maximum = %d, want %d",
				testCase.inputText, *actualMaximum, *testCase.expectedMaximum)
		}
	}
}
