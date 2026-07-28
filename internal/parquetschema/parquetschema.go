// Package parquetschema reads a schema.Schema directly from a Parquet
// file's embedded metadata (no row scanning/inference needed, unlike CSV --
// Parquet already carries its schema in the file footer).
//
// NOTE: this package depends on github.com/parquet-go/parquet-go, which
// could not be fetched or compiled in the sandbox this was written in
// (network egress restrictions blocked golang.org/x/sys, a transitive
// dependency). Run `go get github.com/parquet-go/parquet-go@latest &&
// go mod tidy` locally, then `go test ./...` to confirm this package
// builds and behaves as intended -- treat it as a first draft to verify,
// not confirmed-working code.
package parquetschema

import (
	"fmt"
	"os"

	"github.com/parquet-go/parquet-go"

	"github.com/nbaubek/schemadiff/internal/schema"
)

// InferSchema reads path's Parquet metadata and returns its schema.
//
// Unlike csvschema.InferSchema, this takes a file path rather than an
// io.Reader: the parquet-go library needs an io.ReaderAt plus the file
// size to seek to the footer, which *os.File provides but a generic
// io.Reader does not, so opening the file is left to this function.
func InferSchema(path string) (schema.Schema, error) {
	f, err := os.Open(path)
	if err != nil {
		return schema.Schema{}, fmt.Errorf("opening parquet file: %w", err)
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return schema.Schema{}, fmt.Errorf("stat parquet file: %w", err)
	}

	pf, err := parquet.OpenFile(f, stat.Size())
	if err != nil {
		return schema.Schema{}, fmt.Errorf("reading parquet metadata: %w", err)
	}

	fields := pf.Schema().Fields()
	columns := make([]schema.Column, len(fields))
	for i, field := range fields {
		columns[i] = schema.Column{
			Name: field.Name(),
			Type: mapType(field.Type()),
		}
	}

	return schema.Schema{Columns: columns}, nil
}

// mapType translates a Parquet field's type into our ColumnType.
//
// Parquet stores logical types (DATE, TIMESTAMP, STRING, etc.) as an
// annotation layered on top of a physical storage type -- e.g. a DATE
// column is physically an int32 (days since epoch) and a TIMESTAMP
// column is physically an int64 (verified against real files written by
// pyarrow: signup_date shows as "optional int32 ... (Date)" and
// event_time as "optional int64 ... (Timestamp...)"). So logical type is
// checked FIRST for Date/Timestamp; only if there's no such annotation do
// we fall back to the physical Kind.
//
// String columns are NOT checked via LogicalType here -- an earlier
// attempt referenced a non-existent `String` field on LogicalType (it
// actually resolved to a String() method, not a field, which is why
// `go vet` flagged "comparison of function != nil is always true").
// That check isn't needed anyway: byte-array-backed columns already map
// to TypeString via the physical Kind() switch below.
func mapType(t parquet.Type) schema.ColumnType {
	if lt := t.LogicalType(); lt != nil {
		switch {
		case lt.Date != nil:
			return schema.TypeDate
		case lt.Timestamp != nil:
			return schema.TypeTimestamp
		}
	}

	switch t.Kind() {
	case parquet.Boolean:
		return schema.TypeBool
	case parquet.Int32, parquet.Int64, parquet.Int96:
		return schema.TypeInt
	case parquet.Float, parquet.Double:
		return schema.TypeFloat
	case parquet.ByteArray, parquet.FixedLenByteArray:
		return schema.TypeString
	default:
		return schema.TypeUnknown
	}
}
