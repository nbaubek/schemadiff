# schemadiff

A CLI tool that inspects and compares the schemas of CSV and Parquet files.

## Installation

Requires Go 1.24+ (a Go 1.21+ toolchain will typically auto-fetch a newer
one if needed, since that's driven by this module's `go.mod`).

**If you just want to run it** (no cloning, no manual build):

```
go install github.com/nbaubek/schemadiff@latest
```

This fetches the module, resolves and builds its dependencies (Cobra,
parquet-go) using the checksums already pinned in `go.sum`, and installs
the `schemadiff` binary to `$(go env GOPATH)/bin` (commonly `~/go/bin`).
If that directory is on your `PATH`, you can then run `schemadiff`
directly from anywhere -- no local copy of this repo needed.

**If you're developing on the project itself**, clone it and build
locally instead:

```
git clone https://github.com/nbaubek/schemadiff.git
cd schemadiff
go mod tidy
go build -o schemadiff .
go test ./...
```

## Usage

If installed via `go install` (see above), the binary is just
`schemadiff` on your `PATH`. If you built it locally instead, run it as
`./schemadiff` from the project directory.

Inspect a single file's schema:

```
schemadiff inspect --csv data.csv
schemadiff inspect --parquet data.parquet
```

Compare two files (formats may differ -- CSV vs CSV, Parquet vs Parquet,
or CSV vs Parquet):

```
schemadiff diff old.csv new.csv
schemadiff diff old.csv new.parquet
```

Exit codes follow the `diff(1)` convention:
- `0` — no schema differences (or `inspect` ran fine)
- `1` — `diff` found schema differences (useful as a CI gate)
- `2` — usage or runtime error (bad args, missing file, unrecognized format)

## Example

```
$ schemadiff diff old.csv new.csv
Schema for old.csv:
COLUMN       TYPE
id           int
name         string
legacy_flag  int
Schema for new.csv:
COLUMN       TYPE
id           int
name         string
signup_date  date
STATUS  COLUMN       TYPE
+       signup_date  date
-       legacy_flag  int
```

`+`/`-`/`~` rows are colored green/red/yellow on a real terminal. Color is
automatically disabled when output isn't a terminal (e.g. piped to a file
or `less`), and can be forced off with `--no-color` or by setting the
`NO_COLOR` environment variable.

If the two files share zero columns by name, a warning is printed first --
that usually means two unrelated datasets were compared, not two versions
of the same schema.

## Project layout

- `internal/schema` — format-agnostic Schema/Column model, ColumnType
  constants (string, int, float, bool, date, timestamp), and Diff logic
- `internal/csvschema` — infers a Schema from a CSV file (header + row
  sampling, narrowest-type-first widening)
- `internal/parquetschema` — reads a Schema directly from Parquet file
  metadata, including logical type annotations (Date/Timestamp), confirmed
  passing against real pyarrow-generated fixtures
- `internal/report` — formats a single Schema or a DiffResult for
  terminal output
- `main.go` — Cobra command tree (`inspect`, `diff`); all real logic
  lives in plain functions (`run`, `loadSchema`, etc.) so it's testable
  without spawning the binary

## Status / known limitations

- Local files only (no S3 yet — possible v2 extension)
- Parquet logical types (Date/Timestamp-annotated columns) are checked
  and mapped correctly in `mapType`, confirmed against real fixtures
  (`internal/parquetschema/testdata/new.parquet` for Date,
  `events.parquet` for Timestamp)
- CSV type inference is intentionally not exhaustive (v1 scope)
- Distribution today requires the user to have Go installed
  (`go install ...`). Publishing prebuilt binaries via GitHub Releases
  (e.g. with GoReleaser) would let people run this with no Go toolchain
  at all -- a natural next step, not yet done
