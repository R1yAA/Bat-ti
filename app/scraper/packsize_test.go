package scraper

import "testing"

func TestParsePackSize(t *testing.T) {
	testCases := []struct {
		inputText        string
		expectedPackSize *int
	}{
		// Shapes seen live on the tracked vendors.
		{"5pcs", intPointer(5)},
		{"50pcs", intPointer(50)},
		{"50 pcs", intPointer(50)},
		{"12 pieces", intPointer(12)},
		{"1 pc", intPointer(1)},
		{"Quantity (Pack of 100)", intPointer(100)},
		{"Set of 12", intPointer(12)},
		{"Box of 50", intPointer(50)},
		{"Golden Oval Decorative Basket | Pack of 1", intPointer(1)},

		// Everything ambiguous must yield nothing rather than a guess: a wrong
		// pack size corrupts every per-unit price derived from it.
		{"Default Title", nil},
		{"5.5 Inch", nil},
		{"Metal Lotus Urli 6.5 inch", nil},
		{"10 x 6.375 x 3.75 Mailer Box", nil},
		{"Hamper Box (10.5x8.5x4 Inches)", nil},
		{"Gold Finish", nil},
		{"", nil},
		{"0 pcs", nil},
		{"999999999 pcs", nil},
	}

	for _, testCase := range testCases {
		actualPackSize := ParsePackSize(testCase.inputText)
		switch {
		case testCase.expectedPackSize == nil && actualPackSize != nil:
			t.Errorf("ParsePackSize(%q) = %d, want no pack size", testCase.inputText, *actualPackSize)
		case testCase.expectedPackSize != nil && actualPackSize == nil:
			t.Errorf("ParsePackSize(%q) = nil, want %d", testCase.inputText, *testCase.expectedPackSize)
		case testCase.expectedPackSize != nil && *actualPackSize != *testCase.expectedPackSize:
			t.Errorf("ParsePackSize(%q) = %d, want %d",
				testCase.inputText, *actualPackSize, *testCase.expectedPackSize)
		}
	}
}

func intPointer(value int) *int { return &value }
