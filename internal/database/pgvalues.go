package database

// Hand-written helpers (sqlc never rewrites this file). pgx's null-aware types
// are correct but verbose at call sites; these keep the scraper and handler
// code readable.

import (
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
)

// TextOrNull maps the empty string to SQL NULL. Scrapers routinely find a
// field absent, and storing "" would make "no description" indistinguishable
// from "a description that is empty".
func TextOrNull(value string) pgtype.Text {
	if value == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: value, Valid: true}
}

// Int4OrNull maps a nil pointer to SQL NULL.
func Int4OrNull(value *int) pgtype.Int4 {
	if value == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: int32(*value), Valid: true}
}

// NullUUID wraps a present uuid.
func NullUUID(value uuid.UUID) uuid.NullUUID {
	return uuid.NullUUID{UUID: value, Valid: true}
}

// Timestamptz wraps a present timestamp.
func Timestamptz(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}

// Date wraps a present date.
func Date(value time.Time) pgtype.Date {
	return pgtype.Date{Time: value, Valid: true}
}

// DecimalOrNull maps a nil pointer to SQL NULL.
func DecimalOrNull(value *decimal.Decimal) decimal.NullDecimal {
	if value == nil {
		return decimal.NullDecimal{}
	}
	return decimal.NullDecimal{Decimal: *value, Valid: true}
}
