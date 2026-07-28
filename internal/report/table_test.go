package report

import "testing"

func TestBuildTable_BasicAlignment(t *testing.T) {
	lines := buildTable(
		[]string{"COLUMN", "TYPE"},
		[][]string{{"id", "int"}, {"legacy_flag", "int"}},
	)

	want := []string{
		"┌─────────────┬──────┐",
		"│ COLUMN      │ TYPE │",
		"├─────────────┼──────┤",
		"│ id          │ int  │",
		"│ legacy_flag │ int  │",
		"└─────────────┴──────┘",
	}

	if len(lines) != len(want) {
		t.Fatalf("expected %d lines, got %d:\n%v", len(want), len(lines), lines)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Errorf("line %d mismatch:\ngot:  %q\nwant: %q", i, lines[i], want[i])
		}
	}
}

func TestBuildTable_NoRows(t *testing.T) {
	// A schema with zero columns should still render a valid (if empty)
	// table, not panic or produce a malformed border. buildTable always
	// emits top, header, mid, (rows...), bottom -- with zero rows that's
	// exactly 4 lines.
	lines := buildTable([]string{"COLUMN", "TYPE"}, nil)

	if len(lines) != 4 {
		t.Fatalf("expected 4 lines (top, header, mid, bottom) for zero rows, got %d:\n%v", len(lines), lines)
	}
}
