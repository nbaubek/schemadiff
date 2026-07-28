// Command schemadiff inspects and compares the schemas of CSV and Parquet
// files.
//
// NOTE: this file depends on github.com/spf13/cobra, which could not be
// fetched in the sandbox this was written in (network egress restrictions
// blocked two of its transitive dependencies). Run
// `go get github.com/spf13/cobra@latest && go mod tidy` locally, then
// `go build ./...` and `go test ./...` to confirm this compiles and
// behaves as intended.
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nbaubek/schemadiff/internal/csvschema"
	"github.com/nbaubek/schemadiff/internal/parquetschema"
	"github.com/nbaubek/schemadiff/internal/report"
	"github.com/nbaubek/schemadiff/internal/schema"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run builds the command tree and executes it, returning a process exit
// code rather than calling os.Exit directly -- same reasoning as
// realMain in the previous version: os.Exit inside a test kills the test
// binary, so keeping it out of this function keeps everything testable.
//
// Exit codes still follow the diff(1) convention:
//
//	0 = success, no schema differences (or `inspect` ran fine)
//	1 = `diff` found schema differences
//	2 = usage or runtime error
//
// exitCode is set by the diff subcommand's RunE closure below (Cobra's
// Execute() only tells us success/failure, not this tool's specific
// 3-way convention, so we track it ourselves).
func run(args []string, stdout, stderr io.Writer) int {
	exitCode := 0
	var noColor bool

	rootCmd := &cobra.Command{
		Use:           "schemadiff",
		Short:         "Inspect and compare CSV/Parquet schemas",
		SilenceUsage:  true, // we print our own error messages below
		SilenceErrors: true,
	}
	rootCmd.SetOut(stdout)
	rootCmd.SetErr(stderr)
	rootCmd.SetArgs(args)
	rootCmd.PersistentFlags().BoolVar(&noColor, "no-color", false, "disable colored output")

	rootCmd.AddCommand(newInspectCmd(stdout))
	// useColor is resolved lazily (in a closure) rather than up front,
	// because the --no-color flag hasn't been parsed yet at this point --
	// Cobra parses flags during Execute(), below.
	rootCmd.AddCommand(newDiffCmd(stdout, &exitCode, func() bool {
		return colorEnabled(stdout, noColor)
	}))

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 2
	}
	return exitCode
}

// colorEnabled decides whether to colorize output. It's kept out of the
// report package entirely (report.Write just takes a plain bool) so that
// package stays environment-free and trivially testable; only main.go,
// which is genuinely about the outside world (terminals, env vars,
// flags), needs to know about any of this.
//
// Precedence: --no-color always wins if set; otherwise the NO_COLOR
// convention (https://no-color.org -- any value, including empty, means
// "disable") is respected; otherwise color is enabled only if out is
// actually a terminal (so piping to a file or `| less` doesn't fill the
// output with escape codes).
func colorEnabled(out io.Writer, noColorFlag bool) bool {
	if noColorFlag {
		return false
	}
	if _, set := os.LookupEnv("NO_COLOR"); set {
		return false
	}
	f, ok := out.(*os.File)
	if !ok {
		return false
	}
	stat, err := f.Stat()
	if err != nil {
		return false
	}
	return stat.Mode()&os.ModeCharDevice != 0
}

// newInspectCmd builds `schemadiff inspect --csv|--parquet <file>`.
func newInspectCmd(stdout io.Writer) *cobra.Command {
	var useCSV, useParquet bool

	cmd := &cobra.Command{
		Use:   "inspect [file]",
		Short: "Print the schema of a single CSV or Parquet file",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			format, err := resolveExplicitFormat(useCSV, useParquet)
			if err != nil {
				return err
			}

			s, err := loadSchema(args[0], format)
			if err != nil {
				return fmt.Errorf("reading %s: %w", args[0], err)
			}

			report.WriteSchema(stdout, args[0], s)
			return nil
		},
	}

	cmd.Flags().BoolVar(&useCSV, "csv", false, "treat the file as CSV")
	cmd.Flags().BoolVar(&useParquet, "parquet", false, "treat the file as Parquet")
	return cmd
}

// newDiffCmd builds `schemadiff diff <file1> <file2>`. Format is
// auto-detected per file from its extension (not via --csv/--parquet),
// which is what allows file1 and file2 to be different formats -- e.g.
// `schemadiff diff old.csv new.parquet` just works, since each file's
// reader is chosen independently of the other.
//
// resolveUseColor is a func (not a plain bool) because it must run AFTER
// Cobra has parsed --no-color, which only happens once RunE is actually
// invoked -- so it's called from inside RunE, not passed in as an
// already-decided value.
func newDiffCmd(stdout io.Writer, exitCode *int, resolveUseColor func() bool) *cobra.Command {
	return &cobra.Command{
		Use:   "diff [file1] [file2]",
		Short: "Compare the schemas of two files and report differences",
		Args:  cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			schemaA, err := loadSchema(args[0], "")
			if err != nil {
				return fmt.Errorf("reading %s: %w", args[0], err)
			}
			schemaB, err := loadSchema(args[1], "")
			if err != nil {
				return fmt.Errorf("reading %s: %w", args[1], err)
			}

			report.WriteSchema(stdout, args[0], schemaA)
			report.WriteSchema(stdout, args[1], schemaB)

			diff := schema.Diff(schemaA, schemaB)
			report.Write(stdout, diff, resolveUseColor())

			if !diff.Equal() {
				*exitCode = 1
			}
			return nil
		},
	}
}

// resolveExplicitFormat validates the --csv/--parquet flags for
// `inspect`, where the format must be stated explicitly rather than
// inferred (that's the whole point of the flags: an explicit override,
// not a guess).
func resolveExplicitFormat(useCSV, useParquet bool) (string, error) {
	switch {
	case useCSV && useParquet:
		return "", fmt.Errorf("specify only one of --csv or --parquet")
	case useCSV:
		return "csv", nil
	case useParquet:
		return "parquet", nil
	default:
		return "", fmt.Errorf("must specify --csv or --parquet")
	}
}

// loadSchema reads path's schema. If formatOverride is "csv" or
// "parquet", that reader is used directly (the `inspect` case). If
// formatOverride is "", the format is detected from path's extension
// (the `diff` case, so each of the two files can be a different format).
func loadSchema(path, formatOverride string) (schema.Schema, error) {
	format := formatOverride
	if format == "" {
		format = detectFormat(path)
	}

	switch format {
	case "csv":
		f, err := os.Open(path)
		if err != nil {
			return schema.Schema{}, fmt.Errorf("opening file: %w", err)
		}
		defer f.Close()
		return csvschema.InferSchema(f)
	case "parquet":
		return parquetschema.InferSchema(path)
	default:
		return schema.Schema{}, fmt.Errorf(
			"unrecognized format for %q (use --csv/--parquet, or a .csv/.parquet extension)", path)
	}
}

func detectFormat(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".csv":
		return "csv"
	case ".parquet":
		return "parquet"
	default:
		return ""
	}
}
