package report

import (
	"strings"
	"unicode/utf8"
)

// buildTable renders headers/rows as a box-drawing table (like DuckDB's
// terminal output) and returns it as a slice of PLAIN, uncolored lines:
// [topBorder, headerRow, middleBorder, dataRow..., bottomBorder].
//
// Returning plain lines (rather than writing colored output directly) is
// deliberate: column widths must be computed from the real, visible
// content BEFORE any ANSI color codes exist, or padding breaks (this bit
// us once already with tabwriter -- see the comment on Write in
// report.go). Callers that want color wrap specific returned lines
// afterward, once alignment is already finalized and immutable.
//
// The first 3 returned lines and the last 1 are always
// borders/header/separator; data rows are lines[3 : len(lines)-1].
func buildTable(headers []string, rows [][]string) []string {
	widths := columnWidths(headers, rows)

	top, mid, bottom := borderLines(widths)
	lines := make([]string, 0, len(rows)+4)
	lines = append(lines, top, formatRow(headers, widths), mid)
	for _, row := range rows {
		lines = append(lines, formatRow(row, widths))
	}
	lines = append(lines, bottom)
	return lines
}

// columnWidths returns, for each column, the width of its widest cell
// (header included). utf8.RuneCountInString is used instead of len() so
// column names with multi-byte characters still align correctly.
func columnWidths(headers []string, rows [][]string) []int {
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = utf8.RuneCountInString(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if n := utf8.RuneCountInString(cell); n > widths[i] {
				widths[i] = n
			}
		}
	}
	return widths
}

// borderLines builds the top, middle (below header), and bottom border
// rows for the given column widths, e.g. "┌────┬─────┐".
func borderLines(widths []int) (top, mid, bottom string) {
	var t, m, b strings.Builder
	t.WriteString("┌")
	m.WriteString("├")
	b.WriteString("└")
	for i, w := range widths {
		dash := strings.Repeat("─", w+2) // +2 for the one-space padding on each side
		t.WriteString(dash)
		m.WriteString(dash)
		b.WriteString(dash)
		if i < len(widths)-1 {
			t.WriteString("┬")
			m.WriteString("┼")
			b.WriteString("┴")
		}
	}
	t.WriteString("┐")
	m.WriteString("┤")
	b.WriteString("┘")
	return t.String(), m.String(), b.String()
}

// formatRow renders one row as "│ cell1 │ cell2 │ ... │", padding each
// cell to its column's width.
func formatRow(cells []string, widths []int) string {
	var sb strings.Builder
	sb.WriteString("│")
	for i, cell := range cells {
		pad := widths[i] - utf8.RuneCountInString(cell)
		sb.WriteString(" ")
		sb.WriteString(cell)
		sb.WriteString(strings.Repeat(" ", pad))
		sb.WriteString(" │")
	}
	return sb.String()
}
