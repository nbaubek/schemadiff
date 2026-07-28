// Package parquetschema reads a schema.Schema directly from a Parquet
// file's embedded metadata (no row scanning/inference needed, unlike CSV --
// Parquet already carries its schema in the file footer).
//
// Confirmed working against real pyarrow-generated fixtures (see
// testdata/ and parquetschema_test.go). InferSchemaFromReaderAt (added
// for S3 support) has NOT been separately re-verified since the split --
// it's the same logic InferSchema always used internally, just exposed
// for reuse, so it should behave identically, but re-run `go test ./...`
// after pulling this change to confirm.
package parquetschema

import (
	"fmt"
	"io"
	"os"

	"github.com/parquet-go/parquet-go"

	"github.com/nbaubek/schemadiff/internal/schema"
)

// InferSchema reads path's Parquet metadata and returns its schema, for
// LOCAL files.
//
// This is a thin wrapper around InferSchemaFromReaderAt: it just opens
// the local file (satisfying io.ReaderAt via *os.File) and stats it for
// the size, then hands off to the shared core. That split is what lets
// S3 objects use the exact same schema-reading logic below, via
// internal/s3source's ReaderAt instead of *os.File -- neither this
// function nor InferSchemaFromReaderAt need to know or care which one
// they were given.
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

	return InferSchemaFromReaderAt(f, stat.Size())
}

// InferSchemaFromReaderAt is the actual schema-reading logic, decoupled
// from where the bytes come from. Any io.ReaderAt works here -- a local
// *os.File (via InferSchema above) or an S3-backed reader (see
// internal/s3source.ReaderAt) that turns each ReadAt into a ranged S3
// GetObject, without ever downloading the whole object.
func InferSchemaFromReaderAt(r io.ReaderAt, size int64) (schema.Schema, error) {
	pf, err := parquet.OpenFile(r, size)
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
