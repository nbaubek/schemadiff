package report

import (
	"bytes"
	"strings"
	"testing"

	"github.com/nbaubek/schemadiff/internal/schema"
)

func TestWriteSchema(t *testing.T) {
	s := schema.Schema{Columns: []schema.Column{
		{Name: "id", Type: schema.TypeInt},
		{Name: "created_at", Type: schema.TypeTimestamp},
	}}

	var buf bytes.Buffer
	WriteSchema(&buf, "data.csv", s)
	got := buf.String()

	for _, want := range []string{"data.csv", "id", "int", "created_at", "timestamp"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected output to mention %q, got:\n%s", want, got)
		}
	}
}

func TestWrite_NoDifferences(t *testing.T) {
	var buf bytes.Buffer
	Write(&buf, schema.DiffResult{}, false)

	if got := buf.String(); !strings.Contains(got, "No schema differences") {
		t.Errorf("expected no-diff message, got: %q", got)
	}
}

func TestWrite_AllCategories(t *testing.T) {
	diff := schema.DiffResult{
		Added:     []schema.Column{{Name: "new_col", Type: schema.TypeString}},
		Removed:   []schema.Column{{Name: "old_col", Type: schema.TypeBool}},
		Changed:   []schema.TypeMismatch{{Name: "id", TypeA: schema.TypeInt, TypeB: schema.TypeString}},
		Unchanged: 3, // some overlap, so SharesNoColumns() is false -> no warning expected
	}

	var buf bytes.Buffer
	Write(&buf, diff, false)
	got := buf.String()

	for _, want := range []string{"new_col", "old_col", "id", "int", "string"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected output to mention %q, got:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Warning") {
		t.Errorf("did not expect a no-overlap warning when schemas share columns, got:\n%s", got)
	}
}

func TestWrite_NoColorMeansNoEscapeCodes(t *testing.T) {
	diff := schema.DiffResult{
		Added:     []schema.Column{{Name: "new_col", Type: schema.TypeString}},
		Unchanged: 1,
	}

	var buf bytes.Buffer
	Write(&buf, diff, false)

	if strings.Contains(buf.String(), "\033[") {
		t.Errorf("expected no ANSI escape codes when useColor=false, got:\n%q", buf.String())
	}
}

func TestWrite_ColorAddsEscapeCodesWithoutBreakingAlignment(t *testing.T) {
	diff := schema.DiffResult{
		Added:     []schema.Column{{Name: "new_col", Type: schema.TypeString}},
		Removed:   []schema.Column{{Name: "old_col", Type: schema.TypeBool}},
		Unchanged: 1,
	}

	var colored, plain bytes.Buffer
	Write(&colored, diff, true)
	Write(&plain, diff, false)

	if !strings.Contains(colored.String(), "\033[") {
		t.Error("expected ANSI escape codes when useColor=true")
	}

	// Stripping ANSI codes from the colored output should reproduce the
	// plain output exactly -- this is the real regression test for the
	// tabwriter/ANSI alignment bug: if color were applied BEFORE
	// tabwriter alignment (the wrong order), stripping escape codes
	// afterward would NOT recover the same padding as the plain version.
	stripped := stripANSI(colored.String())
	if stripped != plain.String() {
		t.Errorf("colored output, with ANSI codes stripped, should match plain output exactly.\nstripped: %q\nplain:    %q", stripped, plain.String())
	}
}

func TestWrite_SharesNoColumnsWarning(t *testing.T) {
	diff := schema.DiffResult{
		Added:   []schema.Column{{Name: "b1", Type: schema.TypeInt}},
		Removed: []schema.Column{{Name: "a1", Type: schema.TypeInt}},
		// Unchanged: 0, Changed: nil -> zero overlap
	}

	var buf bytes.Buffer
	Write(&buf, diff, false)

	if !strings.Contains(buf.String(), "Warning") {
		t.Errorf("expected a no-overlap warning, got:\n%s", buf.String())
	}
}

// stripANSI removes "\033[...m" escape sequences for test comparison
// purposes only -- not something the report package itself needs.
func stripANSI(s string) string {
	var b strings.Builder
	inEscape := false
	for _, r := range s {
		if r == '\033' {
			inEscape = true
			continue
		}
		if inEscape {
			if r == 'm' {
				inEscape = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
