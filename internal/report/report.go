// Package report formats a schema.DiffResult (or a single schema.Schema)
// for display to a human on the terminal. It knows nothing about how the
// schemas were produced (CSV, Parquet, or otherwise) -- it only ever
// looks at the schema/DiffResult types, and nothing about *why* colors
// should or shouldn't be used (terminals, NO_COLOR, flags) -- that
// decision is made by the caller (main.go) and passed in as a plain bool.
// Keeping that decision out of this package is what makes every function
// here deterministic and easy to test: a test just picks true or false
// and asserts on exact output, with no environment to fake.
package report

import (
	"fmt"
	"io"

	"github.com/nbaubek/schemadiff/internal/schema"
)

// ANSI color codes. Using raw escape sequences instead of a color library
// keeps this package dependency-free -- for three colors used a handful
// of times, a library would be more machinery than the problem needs.
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m" // removed columns
	colorGreen  = "\033[32m" // added columns
	colorYellow = "\033[33m" // changed columns / warnings
)

// colorize wraps s in the given color code if useColor is true, and
// returns s unmodified otherwise -- so every call site stays readable
// (no scattered if/else for the color-vs-no-color cases).
func colorize(useColor bool, code, s string) string {
	if !useColor {
		return s
	}
	return code + s + colorReset
}

// WriteSchema prints a single schema as a bordered COLUMN/TYPE table
// (DuckDB-style box-drawing), for the `inspect` command. label is a short
// name for the source (e.g. a file path) shown in the header line.
func WriteSchema(w io.Writer, label string, s schema.Schema) {
	fmt.Fprintf(w, "Schema for %s:\n", label)

	rows := make([][]string, len(s.Columns))
	for i, col := range s.Columns {
		rows[i] = []string{col.Name, string(col.Type)}
	}

	for _, line := range buildTable([]string{"COLUMN", "TYPE"}, rows) {
		fmt.Fprintln(w, line)
	}
}

// Write prints a human-readable rendering of diff to w, as a bordered
// STATUS/COLUMN/TYPE table (DuckDB-style box-drawing) with +/-/~ rows
// colored green/red/yellow when useColor is true.
//
// It takes an io.Writer (not necessarily os.Stdout) so this function is
// testable by writing to a bytes.Buffer and inspecting the output, and so
// main.go decides where the output actually goes.
//
// IMPORTANT: buildTable computes the entire table -- borders, padding,
// everything -- from PLAIN text first. Only once that's finalized does
// this function wrap specific already-aligned lines in a color code.
// Coloring before alignment is computed doesn't work: any table renderer
// (tabwriter or this hand-rolled one) counts every byte as visible width,
// including invisible ANSI escape bytes, which throws off padding. Align
// first, colorize finished lines second, always.
func Write(w io.Writer, diff schema.DiffResult, useColor bool) {
	if diff.Equal() {
		fmt.Fprintln(w, "No schema differences found.")
		return
	}

	if diff.SharesNoColumns() {
		fmt.Fprintln(w, colorize(useColor, colorYellow,
			"Warning: these files share no columns in common -- this looks like two"))
		fmt.Fprintln(w, colorize(useColor, colorYellow,
			"unrelated datasets rather than two versions of the same schema."))
		fmt.Fprintln(w)
	}

	var rows [][]string
	var rowColors []string
	for _, col := range diff.Added {
		rows = append(rows, []string{"+", col.Name, string(col.Type)})
		rowColors = append(rowColors, colorGreen)
	}
	for _, col := range diff.Removed {
		rows = append(rows, []string{"-", col.Name, string(col.Type)})
		rowColors = append(rowColors, colorRed)
	}
	for _, c := range diff.Changed {
		rows = append(rows, []string{"~", c.Name, fmt.Sprintf("%s -> %s", c.TypeA, c.TypeB)})
		rowColors = append(rowColors, colorYellow)
	}

	lines := buildTable([]string{"STATUS", "COLUMN", "TYPE"}, rows)
	// lines[0:3] are top border/header/middle border, lines[len-1] is the
	// bottom border -- only the data rows in between get colored, using
	// the same order rows were appended above.
	dataStart, dataEnd := 3, len(lines)-1
	for i, line := range lines {
		if i < dataStart || i >= dataEnd {
			fmt.Fprintln(w, line) // borders/header: never colored
			continue
		}
		fmt.Fprintln(w, colorize(useColor, rowColors[i-dataStart], line))
	}
}
