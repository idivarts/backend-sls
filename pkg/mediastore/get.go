package mediastore

// Reading an object back.
//
// Everything else here writes. An inspector edit has to read the film it
// is changing, and reading it from CloudFront would mean reading whatever
// the CDN happens to be holding — the project objects are stored
// `no-cache`, so a revalidation would usually get it right, and "usually"
// is not a basis for a read-modify-write.
//
// So: straight from the bucket, which is also the only way to be sure the
// bytes being edited are the bytes a render would use.

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/idivarts/backend-sls/pkg/mys3"
)

// maxReadBytes bounds what Get will pull into memory. A project document is
// tens of kilobytes; a font is not something this is for.
const maxReadBytes = 4 << 20

// Get reads one object by its exact key.
func Get(ctx context.Context, t Target, key string) ([]byte, error) {
	if key == "" {
		return nil, fmt.Errorf("mediastore.Get: no key")
	}
	bucket, _, _, err := resolveTarget(t)
	if err != nil {
		return nil, err
	}
	out, err := mys3.Client.GetObjectWithContext(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("mediastore.Get: %s: %w", key, err)
	}
	defer out.Body.Close()
	// LimitReader plus one, so an object AT the limit reads and one over it
	// is refused rather than silently truncated into a corrupt document.
	body, err := io.ReadAll(io.LimitReader(out.Body, maxReadBytes+1))
	if err != nil {
		return nil, fmt.Errorf("mediastore.Get: reading %s: %w", key, err)
	}
	if len(body) > maxReadBytes {
		return nil, fmt.Errorf("mediastore.Get: %s is larger than %d bytes", key, maxReadBytes)
	}
	return body, nil
}
