package csvschema

import (
	"strings"
	"testing"

	"github.com/nbaubek/schemadiff/internal/schema"
)

func TestInferSchema_BasicTypes(t *testing.T) {
	csvData := "id,name,score,active,signup_date\n" +
		"1,alice,9.5,true,2024-01-15\n" +
		"2,bob,8.0,false,2024-02-20\n"

	got, err := InferSchema(strings.NewReader(csvData))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := schema.Schema{Columns: []schema.Column{
		{Name: "id", Type: schema.TypeInt},
		{Name: "name", Type: schema.TypeString},
		{Name: "score", Type: schema.TypeFloat},
		{Name: "active", Type: schema.TypeBool},
		{Name: "signup_date", Type: schema.TypeDate},
	}}

	assertSchemaEqual(t, got, want)
}

func TestInferSchema_TimestampVsDateOnly(t *testing.T) {
	csvData := "event_time,event_date\n" +
		"2024-01-15T10:30:00Z,2024-01-15\n" +
		"2024-02-20T08:00:00Z,2024-02-20\n"

	got, err := InferSchema(strings.NewReader(csvData))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := schema.Schema{Columns: []schema.Column{
		{Name: "event_time", Type: schema.TypeTimestamp},
		{Name: "event_date", Type: schema.TypeDate},
	}}

	assertSchemaEqual(t, got, want)
}

func TestInferSchema_MixedIntColumnWidensToFloat(t *testing.T) {
	// "id" starts as int-looking, then a later row breaks that -> float.
	csvData := "id\n1\n2\n2.5\n"

	got, err := InferSchema(strings.NewReader(csvData))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := schema.Schema{Columns: []schema.Column{
		{Name: "id", Type: schema.TypeFloat},
	}}

	assertSchemaEqual(t, got, want)
}

func TestInferSchema_EmptyValuesDontForceString(t *testing.T) {
	// Blank cells shouldn't make a column collapse to string.
	csvData := "id\n1\n\n3\n"

	got, err := InferSchema(strings.NewReader(csvData))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := schema.Schema{Columns: []schema.Column{
		{Name: "id", Type: schema.TypeInt},
	}}

	assertSchemaEqual(t, got, want)
}

func TestInferSchema_HeaderOnlyIsError(t *testing.T) {
	// No header at all (empty input) should error, not panic.
	_, err := InferSchema(strings.NewReader(""))
	if err == nil {
		t.Fatal("expected an error for empty input, got nil")
	}
}

// assertSchemaEqual compares two schemas column-by-column, since Schema
// contains a slice and so isn't comparable with == (see diff_test.go in
// the schema package for the same pattern applied to Column, which IS
// comparable).
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
