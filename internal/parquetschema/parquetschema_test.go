package parquetschema

import (
	"testing"

	"github.com/nbaubek/schemadiff/internal/schema"
)

// Fixtures in testdata/ were generated with pyarrow (Python), not this
// package, so these tests are also an independent check that InferSchema
// reads a real-world Parquet file correctly -- not just one this same
// library wrote.

func TestInferSchema_MissingFile(t *testing.T) {
	_, err := InferSchema("does_not_exist.parquet")
	if err == nil {
		t.Fatal("expected an error for a missing file, got nil")
	}
}

func TestInferSchema_OldParquet(t *testing.T) {
	got, err := InferSchema("testdata/old.parquet")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := schema.Schema{Columns: []schema.Column{
		{Name: "id", Type: schema.TypeInt},
		{Name: "name", Type: schema.TypeString},
		{Name: "active", Type: schema.TypeBool},
		{Name: "legacy_flag", Type: schema.TypeInt},
	}}
	assertSchemaEqual(t, got, want)
}

func TestInferSchema_NewParquetHasLogicalDate(t *testing.T) {
	got, err := InferSchema("testdata/new.parquet")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := schema.Schema{Columns: []schema.Column{
		{Name: "id", Type: schema.TypeInt},
		{Name: "name", Type: schema.TypeString},
		{Name: "active", Type: schema.TypeBool},
		// signup_date is physically int32 but logically annotated Date --
		// this specifically exercises the LogicalType() check in mapType.
		{Name: "signup_date", Type: schema.TypeDate},
	}}
	assertSchemaEqual(t, got, want)
}

func TestInferSchema_EventsParquetHasLogicalTimestamp(t *testing.T) {
	got, err := InferSchema("testdata/events.parquet")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := schema.Schema{Columns: []schema.Column{
		{Name: "event_id", Type: schema.TypeInt},
		// event_time is physically int64 but logically annotated Timestamp.
		{Name: "event_time", Type: schema.TypeTimestamp},
		{Name: "amount", Type: schema.TypeFloat},
	}}
	assertSchemaEqual(t, got, want)
}

// assertSchemaEqual mirrors the helper in csvschema_test.go: Schema
// contains a slice, so it isn't comparable with ==, but Column is, so we
// compare column-by-column for a readable failure message.
func assertSchemaEqual(t *testing.T, got, want schema.Schema) {
	t.Helper()
	if len(got.Columns) != len(want.Columns) {
		t.Fatalf("column count mismatch: got %d, want %d\ngot:  %+v\nwant: %+v",
			len(got.Columns), len(want.Columns), got.Columns, want.Columns)
	}
	for i := range want.Columns {
		if got.Columns[i] != want.Columns[i] {
			t.Errorf("column %d mismatch: got %+v, want %+v", i, got.Columns[i], want.Columns[i])
		}
	}
}
