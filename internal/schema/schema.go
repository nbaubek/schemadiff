// Package schema defines the format-agnostic representation of a data
// source's schema (Schema, Column) and the logic to diff two schemas.
//
// Nothing in this package knows about CSV or Parquet. Readers for each
// format (see internal/csvschema, internal/parquetschema) are responsible
// for producing a Schema; this package only compares Schemas once they exist.
package schema

// ColumnType is a small, closed set of types we infer/read for a column.
// Using a named string type instead of a bare string catches typos at
// compile time when we construct one from a constant (schema.TypeInt vs
// the unchecked "int").
type ColumnType string

const (
	TypeString    ColumnType = "string"
	TypeInt       ColumnType = "int"
	TypeFloat     ColumnType = "float"
	TypeBool      ColumnType = "bool"
	TypeDate      ColumnType = "date"      // date only, e.g. 2024-01-15
	TypeTimestamp ColumnType = "timestamp" // date + time, e.g. 2024-01-15T10:30:00Z
	TypeUnknown   ColumnType = "unknown"
)

// Column is a single named field and its type.
type Column struct {
	Name string
	Type ColumnType
}

// Schema is an ordered list of columns, in the order they appear in the
// source file. Order is preserved (not a map) because diff output reads
// more naturally column-by-column in file order, and because CSV/Parquet
// column order is itself meaningful data.
type Schema struct {
	Columns []Column
}

// ColumnByName returns the column with the given name and true if found.
// Used by Diff to look up whether a column from one schema exists in the
// other, by name rather than by position (columns can be reordered,
// added, or removed independently).
func (s Schema) ColumnByName(name string) (Column, bool) {
	for _, c := range s.Columns {
		if c.Name == name {
			return c, true
		}
	}
	return Column{}, false
}
