package scraper

import (
	"regexp"
	"strconv"
	"strings"
)

// Vendors write the quantity column of a tier table in several shapes:
//
//	"1 - 9"      "10 - 99"      Jindeal's rendered table
//	"1 - 500"    "12001+"       Restokart
//	"100+"       "10–99"        en dash, seen on some themes
//
// Anything not matching one of these is skipped rather than guessed at: a
// misread quantity range silently prices an order at the wrong tier.
var (
	quantityRangePattern  = regexp.MustCompile(`^(\d[\d,]*)\s*(?:-|–|—|to)\s*(\d[\d,]*)$`)
	quantityOpenPattern   = regexp.MustCompile(`^(\d[\d,]*)\s*(?:\+|and above|or more)$`)
	quantitySinglePattern = regexp.MustCompile(`^(\d[\d,]*)$`)
)

// ParseQuantityRange reads a tier's quantity column. The second return value is
// nil for an open-ended range such as "100+". ok is false when the text is not
// a quantity range at all, which is how header rows and stray table rows are
// filtered out.
func ParseQuantityRange(text string) (minimum int, maximum *int, ok bool) {
	trimmedText := strings.TrimSpace(text)

	if match := quantityRangePattern.FindStringSubmatch(trimmedText); match != nil {
		lowerBound, lowerErr := parseQuantityNumber(match[1])
		upperBound, upperErr := parseQuantityNumber(match[2])
		if lowerErr != nil || upperErr != nil || lowerBound < 1 || upperBound < lowerBound {
			return 0, nil, false
		}
		return lowerBound, &upperBound, true
	}

	if match := quantityOpenPattern.FindStringSubmatch(trimmedText); match != nil {
		lowerBound, err := parseQuantityNumber(match[1])
		if err != nil || lowerBound < 1 {
			return 0, nil, false
		}
		return lowerBound, nil, true
	}

	if match := quantitySinglePattern.FindStringSubmatch(trimmedText); match != nil {
		exactQuantity, err := parseQuantityNumber(match[1])
		if err != nil || exactQuantity < 1 {
			return 0, nil, false
		}
		return exactQuantity, &exactQuantity, true
	}

	return 0, nil, false
}

func parseQuantityNumber(text string) (int, error) {
	return strconv.Atoi(strings.ReplaceAll(text, ",", ""))
}
