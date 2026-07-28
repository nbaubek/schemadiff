// Package csvschema infers a schema.Schema from a CSV file by reading its
// header row and sampling data rows to guess each column's type.
package csvschema

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/nbaubek/schemadiff/internal/schema"
)

// maxSampleRows caps how many data rows we scan for type inference.
// Large files don't need a full scan to guess a type with reasonable
// confidence, and capping it keeps InferSchema's runtime predictable.
const maxSampleRows = 1000

// timestampLayouts are date+time formats we try when guessing
// schema.TypeTimestamp. Checked before dateLayouts since a value with a
// time component wouldn't match a date-only layout anyway, but checking
// the more specific format first keeps the intent clear.
var timestampLayouts = []string{
	time.RFC3339,
	"2006-01-02 15:04:05",
	"2006-01-02T15:04:05",
}

// dateLayouts are date-only formats we try when guessing schema.TypeDate.
// Not exhaustive by design (v1 scope) -- easy to extend later.
var dateLayouts = []string{
	"2006-01-02",
}

// InferSchema reads a CSV from r: the first row is treated as column
// headers, and up to maxSampleRows subsequent rows are sampled to guess
// each column's type.
//
// It takes an io.Reader rather than a file path so it can be unit tested
// against an in-memory string (strings.NewReader(...)) without touching
// disk, and so the caller decides how the file gets opened.
func InferSchema(r io.Reader) (schema.Schema, error) {
	reader := csv.NewReader(r)

	headers, err := reader.Read()
	if err != nil {
		return schema.Schema{}, fmt.Errorf("reading csv header: %w", err)
	}

	// Start every column at the most restrictive type; each column is
	// "widened" as soon as a sampled value doesn't fit it.
	types := make([]schema.ColumnType, len(headers))
	for i := range types {
		types[i] = schema.TypeInt
	}

	for rowNum := 0; rowNum < maxSampleRows; rowNum++ {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return schema.Schema{}, fmt.Errorf("reading csv row %d: %w", rowNum, err)
		}

		for i, value := range record {
			if i >= len(types) {
				continue // ragged row longer than header; ignore extra fields
			}
			if value == "" {
				continue // empty values don't rule out any type
			}
			types[i] = widen(types[i], value)
		}
	}

	columns := make([]schema.Column, len(headers))
	for i, name := range headers {
		columns[i] = schema.Column{Name: name, Type: types[i]}
	}

	return schema.Schema{Columns: columns}, nil
}

// widen returns the narrowest type that still fits both the column's
// current type and the new value, moving through int -> float -> bool ->
// date -> string. It never narrows a column back down, only widens it,
// since we're scanning left-to-right through sampled rows.
func widen(current schema.ColumnType, value string) schema.ColumnType {
	switch current {
	case schema.TypeInt:
		if _, err := strconv.Atoi(value); err == nil {
			return schema.TypeInt
		}
		return widen(schema.TypeFloat, value)

	case schema.TypeFloat:
		if _, err := strconv.ParseFloat(value, 64); err == nil {
			return schema.TypeFloat
		}
		return widen(schema.TypeBool, value)

	case schema.TypeBool:
		if _, err := strconv.ParseBool(value); err == nil {
			return schema.TypeBool
		}
		return widen(schema.TypeTimestamp, value)

	case schema.TypeTimestamp:
		if matchesAny(value, timestampLayouts) {
			return schema.TypeTimestamp
		}
		return widen(schema.TypeDate, value)

	case schema.TypeDate:
		if matchesAny(value, dateLayouts) {
			return schema.TypeDate
		}
		return schema.TypeString

	default: // already schema.TypeString, or anything unrecognized
		return schema.TypeString
	}
}

func matchesAny(value string, layouts []string) bool {
	for _, layout := range layouts {
		if _, err := time.Parse(layout, value); err == nil {
			return true
		}
	}
	return false
}
