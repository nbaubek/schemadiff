package s3source

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

// fakeGetter is an in-memory ObjectGetter for testing ReaderAt's byte-range
// logic without any real S3 access. It intentionally enforces the same
// "give me exactly this range" contract a real S3 Range GetObject would.
type fakeGetter struct {
	objects map[string][]byte // key: "bucket/key" -> object bytes
}

func (f *fakeGetter) ObjectSize(bucket, key string) (int64, error) {
	data, ok := f.objects[bucket+"/"+key]
	if !ok {
		return 0, errors.New("object not found")
	}
	return int64(len(data)), nil
}

func (f *fakeGetter) GetObjectRange(bucket, key string, start, end int64) (io.ReadCloser, error) {
	data, ok := f.objects[bucket+"/"+key]
	if !ok {
		return nil, errors.New("object not found")
	}
	if end < 0 || end >= int64(len(data)) {
		end = int64(len(data)) - 1
	}
	if start > end {
		return io.NopCloser(bytes.NewReader(nil)), nil
	}
	return io.NopCloser(bytes.NewReader(data[start : end+1])), nil
}

func TestParseURI(t *testing.T) {
	cases := []struct {
		uri, wantBucket, wantKey string
		wantOK                   bool
	}{
		{"s3://my-bucket/data.csv", "my-bucket", "data.csv", true},
		{"s3://my-bucket/nested/path/data.parquet", "my-bucket", "nested/path/data.parquet", true},
		{"data.csv", "", "", false},                  // no s3:// scheme -> local path
		{"s3://bucket-only", "", "", false},          // missing key
		{"s3://", "", "", false},                     // missing everything
		{"https://example.com/x.csv", "", "", false}, // wrong scheme
	}

	for _, c := range cases {
		bucket, key, ok := ParseURI(c.uri)
		if ok != c.wantOK || bucket != c.wantBucket || key != c.wantKey {
			t.Errorf("ParseURI(%q) = (%q, %q, %v), want (%q, %q, %v)",
				c.uri, bucket, key, ok, c.wantBucket, c.wantKey, c.wantOK)
		}
	}
}

func TestReaderAt_MatchesBytesReaderBehavior(t *testing.T) {
	// The real test of correctness: our S3-backed ReaderAt should behave
	// EXACTLY like stdlib's bytes.Reader (which also implements
	// io.ReaderAt) for the same underlying data, across a range of
	// offsets/lengths including the tricky edge cases (start, middle,
	// end, and reading past the end).
	data := []byte("id,name,active\n1,alice,true\n2,bob,false\n")

	getter := &fakeGetter{objects: map[string][]byte{"bucket/key.csv": data}}
	s3ra := &ReaderAt{getter: getter, bucket: "bucket", key: "key.csv"}
	stdra := bytes.NewReader(data)

	cases := []struct {
		name      string
		off, plen int64
	}{
		{"from start", 0, 5},
		{"from middle", 10, 8},
		{"exact end", int64(len(data)) - 4, 4},
		{"past end (partial read + EOF)", int64(len(data)) - 2, 10},
		{"zero length", 5, 0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			bufS3 := make([]byte, c.plen)
			nS3, errS3 := s3ra.ReadAt(bufS3, c.off)

			bufStd := make([]byte, c.plen)
			nStd, errStd := stdra.ReadAt(bufStd, c.off)

			if nS3 != nStd {
				t.Errorf("byte count mismatch: got %d, want %d (matching bytes.Reader)", nS3, nStd)
			}
			if !bytes.Equal(bufS3[:nS3], bufStd[:nStd]) {
				t.Errorf("data mismatch:\ngot:  %q\nwant: %q", bufS3[:nS3], bufStd[:nStd])
			}
			// Both should agree on whether EOF was hit.
			if (errS3 == io.EOF) != (errStd == io.EOF) {
				t.Errorf("EOF mismatch: got err=%v, want err=%v (matching bytes.Reader)", errS3, errStd)
			}
		})
	}
}

func TestOpenParquetReaderAt_UsableByAConsumerLikeParquetGo(t *testing.T) {
	// Simulates roughly how parquet-go would use this: get the size, then
	// issue a handful of ReadAt calls at various offsets to read a
	// "footer" from the end and a "header" from the start.
	data := bytes.Repeat([]byte("X"), 1000)
	copy(data[0:4], []byte("PAR1"))
	copy(data[len(data)-4:], []byte("PAR1"))

	getter := &fakeGetter{objects: map[string][]byte{"b/k.parquet": data}}
	ra, size, err := OpenParquetReaderAt(getter, "b", "k.parquet")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if size != int64(len(data)) {
		t.Fatalf("size mismatch: got %d, want %d", size, len(data))
	}

	header := make([]byte, 4)
	if _, err := ra.ReadAt(header, 0); err != nil {
		t.Fatalf("reading header: %v", err)
	}
	if string(header) != "PAR1" {
		t.Errorf("header magic mismatch: got %q", header)
	}

	footer := make([]byte, 4)
	if _, err := ra.ReadAt(footer, size-4); err != nil {
		t.Fatalf("reading footer: %v", err)
	}
	if string(footer) != "PAR1" {
		t.Errorf("footer magic mismatch: got %q", footer)
	}
}

func TestOpenCSVReader_StreamsWholeObject(t *testing.T) {
	data := []byte("id,name\n1,alice\n2,bob\n")
	getter := &fakeGetter{objects: map[string][]byte{"b/k.csv": data}}

	rc, err := OpenCSVReader(getter, "b", "k.csv")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer rc.Close()

	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("unexpected error reading: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Errorf("got %q, want %q", got, data)
	}
}

func TestOpenParquetReaderAt_MissingObject(t *testing.T) {
	getter := &fakeGetter{objects: map[string][]byte{}}
	_, _, err := OpenParquetReaderAt(getter, "b", "does-not-exist.parquet")
	if err == nil {
		t.Fatal("expected an error for a missing object, got nil")
	}
}
