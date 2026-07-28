// Package s3source lets schemadiff read CSV/Parquet schemas directly from
// S3 objects without downloading them to a temp file first.
//
// The design deliberately splits into two layers:
//
//  1. This file (s3source.go): pure logic -- URI parsing and the
//     io.ReaderAt implementation that turns arbitrary byte-range requests
//     into calls to a small ObjectGetter interface. NONE of this imports
//     the AWS SDK, which is what makes it fully unit-testable with an
//     in-memory fake, no network or AWS credentials required.
//  2. aws_adapter.go: a thin, ~2-method adapter satisfying ObjectGetter
//     using the real AWS SDK. That's the ONLY file in this package that
//     touches AWS types, and it's small on purpose -- if the SDK's exact
//     API differs from what's written here, the fix is localized to a
//     handful of lines, not a redesign.
//
// Why Parquet needs this at all (and CSV doesn't): Parquet's schema
// lives in a footer at the END of the file, so reading it means seeking
// backward -- impossible with a single streaming download. CSV just
// needs to read forward from the start, so it can use S3's GetObject
// body directly as an io.Reader with zero extra machinery (see
// OpenCSVReader below).
package s3source

import (
	"fmt"
	"io"
	"strings"
)

// ObjectGetter is the minimal set of S3-like operations schemadiff
// actually needs. Defining our own small interface (rather than
// depending on *s3.Client directly everywhere) is what lets ReaderAt's
// logic below be tested with a fake, in-memory implementation.
type ObjectGetter interface {
	// ObjectSize returns the total size of the object in bytes.
	ObjectSize(bucket, key string) (int64, error)

	// GetObjectRange returns the bytes in [start, end] (inclusive). If
	// end < 0, it means "to the end of the object" (used for the CSV
	// streaming case, which wants the whole thing).
	GetObjectRange(bucket, key string, start, end int64) (io.ReadCloser, error)
}

// ParseURI splits an "s3://bucket/key/with/slashes.csv" URI into its
// bucket and key. ok is false if uri doesn't have the s3:// scheme or is
// missing a key.
func ParseURI(uri string) (bucket, key string, ok bool) {
	const prefix = "s3://"
	if !strings.HasPrefix(uri, prefix) {
		return "", "", false
	}
	rest := strings.TrimPrefix(uri, prefix)
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// OpenCSVReader returns a streaming reader over the whole object, for
// CSV's forward-only reading (see the package doc for why this doesn't
// need ReaderAt/random access the way Parquet does).
func OpenCSVReader(getter ObjectGetter, bucket, key string) (io.ReadCloser, error) {
	rc, err := getter.GetObjectRange(bucket, key, 0, -1)
	if err != nil {
		return nil, fmt.Errorf("opening s3://%s/%s: %w", bucket, key, err)
	}
	return rc, nil
}

// OpenParquetReaderAt returns an io.ReaderAt over the S3 object plus its
// total size -- exactly what parquet-go's OpenFile needs to seek to the
// footer and parse the schema, without ever downloading the whole object.
func OpenParquetReaderAt(getter ObjectGetter, bucket, key string) (io.ReaderAt, int64, error) {
	size, err := getter.ObjectSize(bucket, key)
	if err != nil {
		return nil, 0, fmt.Errorf("getting size of s3://%s/%s: %w", bucket, key, err)
	}
	return &ReaderAt{getter: getter, bucket: bucket, key: key}, size, nil
}

// ReaderAt implements io.ReaderAt by translating each ReadAt call into a
// single ranged S3 GetObject. parquet-go will call this repeatedly with
// different offsets as it parses the footer -- each call only fetches the
// specific bytes requested, never the whole object.
type ReaderAt struct {
	getter      ObjectGetter
	bucket, key string
}

// ReadAt satisfies io.ReaderAt's contract: it must fill p completely
// unless it hits the end of the object, in which case it returns the
// partial read along with io.EOF -- exactly the same contract
// io.ReadFull relies on, which is why this is built on top of it.
func (r *ReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	end := off + int64(len(p)) - 1
	body, err := r.getter.GetObjectRange(r.bucket, r.key, off, end)
	if err != nil {
		return 0, fmt.Errorf("reading s3://%s/%s [%d-%d]: %w", r.bucket, r.key, off, end, err)
	}
	defer body.Close()

	n, err := io.ReadFull(body, p)
	if err == io.ErrUnexpectedEOF {
		// Fewer bytes were available than requested (we asked past the
		// end of the object) -- io.ReaderAt's contract wants a plain
		// io.EOF in that case, not ErrUnexpectedEOF.
		return n, io.EOF
	}
	return n, err
}
