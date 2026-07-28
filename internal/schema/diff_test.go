package schema

import "testing"

func TestDiff_AddedRemovedChanged(t *testing.T) {
	a := Schema{Columns: []Column{
		{Name: "id", Type: TypeInt},
		{Name: "name", Type: TypeString},
		{Name: "legacy_flag", Type: TypeBool}, // will be "removed" (not in b)
	}}

	b := Schema{Columns: []Column{
		{Name: "id", Type: TypeString}, // type changed: int -> string
		{Name: "name", Type: TypeString},
		{Name: "signup_date", Type: TypeDate}, // "added" (not in a)
	}}

	got := Diff(a, b)

	// --- Added ---
	if len(got.Added) != 1 {
		t.Fatalf("expected 1 added column, got %d: %+v", len(got.Added), got.Added)
	}
	if got.Added[0] != (Column{Name: "signup_date", Type: TypeDate}) {
		// Column is comparable (only string fields), so != works directly.
		t.Errorf("added column mismatch: got %+v", got.Added[0])
	}

	// --- Removed ---
	if len(got.Removed) != 1 {
		t.Fatalf("expected 1 removed column, got %d: %+v", len(got.Removed), got.Removed)
	}
	if got.Removed[0] != (Column{Name: "legacy_flag", Type: TypeBool}) {
		t.Errorf("removed column mismatch: got %+v", got.Removed[0])
	}

	// --- Changed ---
	if len(got.Changed) != 1 {
		t.Fatalf("expected 1 changed column, got %d: %+v", len(got.Changed), got.Changed)
	}
	wantMismatch := TypeMismatch{Name: "id", TypeA: TypeInt, TypeB: TypeString}
	if got.Changed[0] != wantMismatch {
		t.Errorf("type mismatch: got %+v, want %+v", got.Changed[0], wantMismatch)
	}

	if got.Equal() {
		t.Error("expected Equal() to be false when there are differences")
	}
}

func TestDiff_UnchangedCount(t *testing.T) {
	a := Schema{Columns: []Column{
		{Name: "id", Type: TypeInt},
		{Name: "name", Type: TypeString}, // unchanged
	}}
	b := Schema{Columns: []Column{
		{Name: "id", Type: TypeString},   // changed
		{Name: "name", Type: TypeString}, // unchanged
	}}

	got := Diff(a, b)
	if got.Unchanged != 1 {
		t.Errorf("expected Unchanged == 1, got %d", got.Unchanged)
	}
}

func TestDiff_SharesNoColumns(t *testing.T) {
	a := Schema{Columns: []Column{{Name: "a1", Type: TypeInt}, {Name: "a2", Type: TypeString}}}
	b := Schema{Columns: []Column{{Name: "b1", Type: TypeInt}, {Name: "b2", Type: TypeString}}}

	got := Diff(a, b)
	if !got.SharesNoColumns() {
		t.Error("expected SharesNoColumns() to be true for fully disjoint schemas")
	}

	// Sanity check: a normal diff with SOME overlap should report false.
	c := Schema{Columns: []Column{{Name: "a1", Type: TypeInt}, {Name: "new", Type: TypeString}}}
	gotOverlap := Diff(a, c)
	if gotOverlap.SharesNoColumns() {
		t.Error("expected SharesNoColumns() to be false when schemas share a column")
	}
}

func TestDiff_IdenticalSchemas(t *testing.T) {
	a := Schema{Columns: []Column{
		{Name: "id", Type: TypeInt},
		{Name: "name", Type: TypeString},
	}}
	b := a // same columns, same order

	got := Diff(a, b)

	if !got.Equal() {
		t.Errorf("expected identical schemas to produce no diff, got %+v", got)
	}
}
