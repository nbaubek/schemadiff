package schema

// TypeMismatch describes a column present in both schemas but with a
// different inferred/declared type.
type TypeMismatch struct {
	Name  string
	TypeA ColumnType
	TypeB ColumnType
}

// DiffResult is the full comparison between two schemas: A is treated as
// the "before"/left side, B as the "after"/right side.
type DiffResult struct {
	Added     []Column       // present in B, not in A
	Removed   []Column       // present in A, not in B
	Changed   []TypeMismatch // present in both, type differs
	Unchanged int            // present in both, identical type -- a count only (not a list), used to detect e.g. zero column overlap between schemas
}

// Equal reports whether the two schemas have no differences at all.
// Useful for the CLI's exit code: no differences -> exit 0, else exit 1.
func (d DiffResult) Equal() bool {
	return len(d.Added) == 0 && len(d.Removed) == 0 && len(d.Changed) == 0
}

// SharesNoColumns reports whether A and B have zero columns in common by
// name -- a signal worth surfacing distinctly, since it usually means two
// unrelated datasets were compared rather than two versions of the same
// schema (as opposed to a normal case with some adds/removes/changes
// alongside columns that stayed the same).
func (d DiffResult) SharesNoColumns() bool {
	overlap := d.Unchanged + len(d.Changed)
	return overlap == 0 && len(d.Added) > 0 && len(d.Removed) > 0
}

// Diff compares schema b against schema a and reports what changed.
// a is the "left"/baseline file, b is the "right"/comparison file, matching
// the order files are given on the command line: schemadiff a.csv b.csv.
func Diff(a, b Schema) DiffResult {
	var result DiffResult

	for _, colB := range b.Columns {
		colA, existsInA := a.ColumnByName(colB.Name)
		if !existsInA {
			result.Added = append(result.Added, colB)
			continue
		}
		if colA.Type != colB.Type {
			result.Changed = append(result.Changed, TypeMismatch{
				Name:  colB.Name,
				TypeA: colA.Type,
				TypeB: colB.Type,
			})
		} else {
			result.Unchanged++
		}
	}

	for _, colA := range a.Columns {
		if _, existsInB := b.ColumnByName(colA.Name); !existsInB {
			result.Removed = append(result.Removed, colA)
		}
	}

	return result
}
