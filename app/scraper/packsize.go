package scraper

import (
	"regexp"
	"strconv"
)

// Pack sizes are written into product titles and variant labels rather than
// exposed as a field, in several different shapes across the tracked vendors:
//
//	"5pcs", "50 pcs"            — Dispozable variant labels
//	"Pack of 100", "Set of 12"  — Restokart, and Shopify titles
//
// The patterns below are deliberately narrow. A wrong pack size silently
// corrupts every per-unit price derived from it, so anything not matching one
// of these shapes yields no pack size at all and the value stays null until a
// human fills it in.
var packSizePatterns = []*regexp.Regexp{
	// "5pcs", "50 pieces", "12 pc"
	regexp.MustCompile(`(?i)\b(\d{1,6})\s*(?:pcs?|pieces?)\b`),
	// "Pack of 100", "Set of 12", "Box of 50", "Bundle of 6"
	regexp.MustCompile(`(?i)\b(?:pack|set|box|bundle)\s+of\s+(\d{1,6})\b`),
}

// maximumPlausiblePackSize rejects readings that are almost certainly a
// dimension, a year or a model number rather than a count.
const maximumPlausiblePackSize = 100000

// ParsePackSize extracts a pack size from free text such as a variant label or
// a product title. It returns nil when the text does not clearly state one.
func ParsePackSize(text string) *int {
	for _, pattern := range packSizePatterns {
		match := pattern.FindStringSubmatch(text)
		if match == nil {
			continue
		}
		packSize, err := strconv.Atoi(match[1])
		if err != nil || packSize <= 0 || packSize > maximumPlausiblePackSize {
			continue
		}
		return &packSize
	}
	return nil
}
