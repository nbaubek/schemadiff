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
	"bytes"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

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

// newTabwriter returns a tabwriter configured the same way everywhere in
// this package, so every table in the tool lines up with the same
// padding/alignment rules.
func newTabwriter(w io.Writer) *tabwriter.Writer {
	// minwidth=0, tabwidth=0, padding=2 spaces between columns, padchar=' ',
	// no special flags -- the plainest column alignment tabwriter offers.
	return tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
}

// WriteSchema prints a single schema as an aligned COLUMN/TYPE table, for
// the `inspect` command. label is a short name for the source (e.g. a
// file path) shown in the header line.
func WriteSchema(w io.Writer, label string, s schema.Schema) {
	fmt.Fprintf(w, "Schema for %s:\n", label)

	tw := newTabwriter(w)
	fmt.Fprintln(tw, "COLUMN\tTYPE")
	for _, col := range s.Columns {
		fmt.Fprintf(tw, "%s\t%s\n", col.Name, col.Type)
	}
	tw.Flush()
}

// Write prints a human-readable rendering of diff to w, as an aligned
// STATUS/COLUMN/TYPE table with +/-/~ rows colored green/red/yellow when
// useColor is true.
//
// It takes an io.Writer (not necessarily os.Stdout) so this function is
// testable by writing to a bytes.Buffer and inspecting the output, and so
// main.go decides where the output actually goes.
//
// IMPORTANT: the table is formatted through tabwriter FIRST, entirely in
// plain text, and only AFTER that is each finished line wrapped in a
// color code. Coloring while tabwriter is still computing alignment
// doesn't work: tabwriter counts every byte between tabs as visible
// width, including invisible ANSI escape bytes, which throws off column
// padding. Doing the two steps in this order -- align first, colorize
// finished lines second -- sidesteps that entirely, since a fully-padded
// line wrapped in a color code afterward doesn't get re-measured by
// anything.
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

	// rowColors[i] is the color for data row i (post-header), in the same
	// order the rows are written below: all Added, then all Removed, then
	// all Changed.
	var rowColors []string
	for range diff.Added {
		rowColors = append(rowColors, colorGreen)
	}
	for range diff.Removed {
		rowColors = append(rowColors, colorRed)
	}
	for range diff.Changed {
		rowColors = append(rowColors, colorYellow)
	}

	var plain bytes.Buffer
	tw := newTabwriter(&plain)
	fmt.Fprintln(tw, "STATUS\tCOLUMN\tTYPE")
	for _, col := range diff.Added {
		fmt.Fprintf(tw, "+\t%s\t%s\n", col.Name, col.Type)
	}
	for _, col := range diff.Removed {
		fmt.Fprintf(tw, "-\t%s\t%s\n", col.Name, col.Type)
	}
	for _, c := range diff.Changed {
		fmt.Fprintf(tw, "~\t%s\t%s -> %s\n", c.Name, c.TypeA, c.TypeB)
	}
	tw.Flush()

	lines := strings.Split(strings.TrimRight(plain.String(), "\n"), "\n")
	for i, line := range lines {
		if i == 0 { // header row: never colored
			fmt.Fprintln(w, line)
			continue
		}
		fmt.Fprintln(w, colorize(useColor, rowColors[i-1], line))
	}
}
