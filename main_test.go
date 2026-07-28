package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTempFile is a small test helper: writes content to a temp file
// with the given name and returns its path. t.TempDir() auto-cleans
// after the test.
func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	return path
}

func TestColorEnabled_NonFileWriterIsAlwaysFalse(t *testing.T) {
	// bytes.Buffer isn't *os.File, so colorEnabled can't check "is this a
	// terminal" and should conservatively say no -- this is also why
	// none of the run() tests above ever see ANSI codes even without
	// passing --no-color: they all write to bytes.Buffer, not os.Stdout.
	var buf bytes.Buffer
	if colorEnabled(&buf, false) {
		t.Error("expected colorEnabled to be false for a non-*os.File writer")
	}
}

func TestColorEnabled_NoColorFlagAlwaysWins(t *testing.T) {
	// Even against a real terminal-like file, the explicit flag should
	// override everything else.
	if colorEnabled(os.Stdout, true) {
		t.Error("expected --no-color to disable color regardless of the writer")
	}
}

func TestColorEnabled_NOCOLOREnvVarDisablesColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	if colorEnabled(os.Stdout, false) {
		t.Error("expected NO_COLOR env var to disable color")
	}
}

func TestRun_InspectCSV(t *testing.T) {
	file := writeTempFile(t, "data.csv", "id,name\n1,alice\n2,bob\n")

	var stdout, stderr bytes.Buffer
	code := run([]string{"inspect", "--csv", file}, &stdout, &stderr)

	if code != 0 {
		t.Errorf("expected exit code 0, got %d (stderr: %s)", code, stderr.String())
	}
	for _, want := range []string{"id", "int", "name", "string"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("expected output to mention %q, got: %q", want, stdout.String())
		}
	}
}

func TestRun_InspectRequiresFormatFlag(t *testing.T) {
	file := writeTempFile(t, "data.csv", "id,name\n1,alice\n")

	var stdout, stderr bytes.Buffer
	code := run([]string{"inspect", file}, &stdout, &stderr)

	if code != 2 {
		t.Errorf("expected exit code 2 when neither --csv nor --parquet given, got %d", code)
	}
}

func TestRun_InspectRejectsBothFormatFlags(t *testing.T) {
	file := writeTempFile(t, "data.csv", "id,name\n1,alice\n")

	var stdout, stderr bytes.Buffer
	code := run([]string{"inspect", "--csv", "--parquet", file}, &stdout, &stderr)

	if code != 2 {
		t.Errorf("expected exit code 2 when both --csv and --parquet given, got %d", code)
	}
}

func TestRun_DiffNoDifferences(t *testing.T) {
	fileA := writeTempFile(t, "a.csv", "id,name\n1,alice\n")
	fileB := writeTempFile(t, "b.csv", "id,name\n2,bob\n")

	var stdout, stderr bytes.Buffer
	code := run([]string{"diff", fileA, fileB}, &stdout, &stderr)

	if code != 0 {
		t.Errorf("expected exit code 0, got %d (stderr: %s)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "No schema differences") {
		t.Errorf("expected no-diff message, got: %q", stdout.String())
	}
}

func TestRun_DiffFindsDifferences(t *testing.T) {
	fileA := writeTempFile(t, "a.csv", "id,name\n1,alice\n")
	fileB := writeTempFile(t, "b.csv", "id,name,email\n1,alice,a@example.com\n")

	var stdout, stderr bytes.Buffer
	code := run([]string{"diff", fileA, fileB}, &stdout, &stderr)

	if code != 1 {
		t.Errorf("expected exit code 1, got %d (stderr: %s)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "email") {
		t.Errorf("expected output to mention added column 'email', got: %q", stdout.String())
	}
}

func TestRun_DiffWrongArgCount(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"diff", "only_one_file.csv"}, &stdout, &stderr)

	if code != 2 {
		t.Errorf("expected exit code 2 for bad usage, got %d", code)
	}
}

func TestRun_UnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"bogus"}, &stdout, &stderr)

	if code != 2 {
		t.Errorf("expected exit code 2 for unknown subcommand, got %d", code)
	}
}
