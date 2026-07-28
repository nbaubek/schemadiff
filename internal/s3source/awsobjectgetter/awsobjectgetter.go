// Package awsobjectgetter implements s3source.ObjectGetter using the
// real AWS SDK. It's kept in its own subpackage -- separate from
// internal/s3source itself -- specifically so that package's fully
// unit-tested core logic (URI parsing, the ReaderAt byte-range math) can
// build and pass `go test` on its own, even if this package's AWS SDK
// dependency can't be fetched (as happened in the sandbox this was
// written in -- see the verification note below). One broken/unfetchable
// dependency in this package no longer breaks the other.
package awsobjectgetter

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// NOTE ON VERIFICATION: this file is the ONLY place in this project that
// imports the AWS SDK. It could not be fetched or compiled in the
// sandbox this was written in -- github.com/aws/aws-sdk-go-v2/config also
// requires Go 1.24+, same wall hit by parquet-go. Run
// `go get github.com/aws/aws-sdk-go-v2/config@latest &&
//  go get github.com/aws/aws-sdk-go-v2/service/s3@latest && go mod tidy`
// locally, then `go build ./...`.
//
// If field/method names here are slightly wrong (this happened once
// already with parquet-go's LogicalType field), the fix is localized to
// this ~40-line file -- everything in internal/s3source itself (ParseURI,
// ReaderAt's range-read logic) is independently unit-tested against a
// fake and does not need to change.

// requestTimeout bounds every individual S3 call. Without this, a hung
// network or misconfigured region/credentials can leave the CLI frozen
// indefinitely instead of failing with a clear error -- worth having
// even though it means every Getter call takes a background
// context internally rather than one threaded in from the caller (kept
// simple for v1; threading a caller-supplied context through
// ObjectGetter's methods would be the natural next refinement).
const requestTimeout = 10 * time.Second

// Getter implements ObjectGetter using the real AWS SDK.
type Getter struct {
	client *s3.Client
}

// New builds a client using the SDK's standard credential
// chain (env vars, ~/.aws/credentials, IAM role/SSO, in that order) via
// config.LoadDefaultConfig -- deliberately NOT accepting access keys as
// flags or arguments, so schemadiff never has a code path that could log
// or leak a credential.
func New(ctx context.Context) (*Getter, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("loading AWS config: %w", err)
	}
	return &Getter{client: s3.NewFromConfig(cfg)}, nil
}

func (g *Getter) ObjectSize(bucket, key string) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	out, err := g.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return 0, fmt.Errorf("HEAD s3://%s/%s: %w", bucket, key, err)
	}
	return aws.ToInt64(out.ContentLength), nil
}

func (g *Getter) GetObjectRange(bucket, key string, start, end int64) (io.ReadCloser, error) {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)

	rangeHeader := fmt.Sprintf("bytes=%d-", start)
	if end >= 0 {
		rangeHeader = fmt.Sprintf("bytes=%d-%d", start, end)
	}

	out, err := g.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Range:  aws.String(rangeHeader),
	})
	if err != nil {
		cancel()
		return nil, fmt.Errorf("GET s3://%s/%s [%s]: %w", bucket, key, rangeHeader, err)
	}

	// out.Body's reads are tied to ctx above -- calling cancel() right
	// away (e.g. via a plain `defer cancel()`) would cancel it before the
	// caller gets a chance to read anything, since this function is about
	// to return. Instead, cancel only once the caller is actually done
	// with the body, i.e. when they Close() it.
	return &cancelOnCloseBody{ReadCloser: out.Body, cancel: cancel}, nil
}

// cancelOnCloseBody ties a context's lifetime to an io.ReadCloser's
// lifetime: the context stays alive (so reads keep working) until the
// caller calls Close(), at which point we release the timeout context's
// resources. This also means the requestTimeout above bounds the whole
// read, not just the initial connection -- a caller that reads too
// slowly for too long will start getting "context deadline exceeded"
// errors, which is the intended tradeoff for a CLI tool (fail loudly
// rather than hang forever).
type cancelOnCloseBody struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (b *cancelOnCloseBody) Close() error {
	err := b.ReadCloser.Close()
	b.cancel()
	return err
}
