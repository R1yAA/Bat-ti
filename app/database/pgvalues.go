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

// DateOrNull maps a nil pointer to SQL NULL. A date that is only meaningful in
// one state — a delivery date before the order is delivered — is absent rather
// than zero, and the zero date would sort and display as year 1.
func DateOrNull(value *time.Time) pgtype.Date {
	if value == nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: *value, Valid: true}
}

// DecimalOrNull maps a nil pointer to SQL NULL.
func DecimalOrNull(value *decimal.Decimal) decimal.NullDecimal {
	if value == nil {
		return decimal.NullDecimal{}
	}
	return decimal.NullDecimal{Decimal: *value, Valid: true}
}

// Readers, for turning stored values back into plain Go types at the API
// boundary. pgx's null-aware types do not serialise to sensible JSON, so
// handlers convert rather than returning database rows directly — which also
// keeps the API's shape independent of the schema's.

// TextValue returns nil for SQL NULL.
func TextValue(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

// TextOrEmpty flattens SQL NULL to the empty string, for fields where the API
// promises a string and absent means empty.
func TextOrEmpty(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

// TimeValue returns nil for SQL NULL.
func TimeValue(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	return &value.Time
}

// DateValue returns the zero time for SQL NULL.
func DateValue(value pgtype.Date) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time
}

// DatePointer returns nil for SQL NULL, for dates the API reports as absent
// rather than flattening to the zero date the way DateValue does.
func DatePointer(value pgtype.Date) *time.Time {
	if !value.Valid {
		return nil
	}
	return &value.Time
}

// IntValue returns nil for SQL NULL.
func IntValue(value pgtype.Int4) *int {
	if !value.Valid {
		return nil
	}
	converted := int(value.Int32)
	return &converted
}

// DecimalValue returns nil for SQL NULL.
func DecimalValue(value decimal.NullDecimal) *decimal.Decimal {
	if !value.Valid {
		return nil
	}
	return &value.Decimal
}

// UUIDValue returns nil for SQL NULL.
func UUIDValue(value uuid.NullUUID) *uuid.UUID {
	if !value.Valid {
		return nil
	}
	return &value.UUID
}
