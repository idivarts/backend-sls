package mediastore

// Removing a superseded copy of something we wrote.
//
// ── Why this exists at all, in a package about keeping things ────────────
// Everything else here is about making sure bytes are never anonymous. This
// is the one operation that takes bytes away, and it is deliberately narrow:
// it removes a whole prefix, it refuses any prefix that is not a design
// revision, and it is used by exactly one caller — the retention cap that
// stops a design edited fifty times from keeping fifty copies of its
// project.
//
// It does NOT touch the media library. A project file is written with
// NoLibrary, so there is no row to remove and no reference count to get
// wrong. Anything that IS in the library must be deleted through the
// library, not through here.
//
// ── The guard is the point ───────────────────────────────────────────────
// A delete-by-prefix with a caller-supplied prefix is one bad argument away
// from emptying a bucket. So the prefix must look exactly like
// design-projects/{brand}/{design}/revisions/{n} and nothing else: no
// traversal, no trailing wildcard, no design root (which would take the
// current revision with it), no brand root.

import (
	"context"
	"fmt"
	"regexp"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/idivarts/backend-sls/pkg/mys3"
)

// designRevisionPrefix is the only shape DeletePrefix accepts.
var designRevisionPrefix = regexp.MustCompile(`^design-projects/[A-Za-z0-9_-]+/[A-Za-z0-9_-]+/revisions/[1-9][0-9]*$`)

// deleteBatchSize is S3's own limit for one DeleteObjects call.
const deleteBatchSize = 1000

// DeletePrefix removes every object under a design-revision prefix and
// returns how many it removed.
//
// Returns an error without deleting anything if the prefix is not a design
// revision.
func DeletePrefix(ctx context.Context, t Target, prefix string) (int, error) {
	if !designRevisionPrefix.MatchString(prefix) {
		return 0, fmt.Errorf("mediastore.DeletePrefix: %q is not a design revision prefix", prefix)
	}
	bucket, _, _, err := resolveTarget(t)
	if err != nil {
		return 0, err
	}

	removed := 0
	var continuation *string
	for {
		// The trailing slash matters: without it, "…/revisions/1" also
		// lists "…/revisions/10", and a cleanup of one revision would take
		// another one with it.
		out, err := mys3.Client.ListObjectsV2WithContext(ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(bucket),
			Prefix:            aws.String(prefix + "/"),
			ContinuationToken: continuation,
		})
		if err != nil {
			return removed, fmt.Errorf("mediastore.DeletePrefix: listing %s: %w", prefix, err)
		}
		ids := make([]*s3.ObjectIdentifier, 0, len(out.Contents))
		for _, o := range out.Contents {
			ids = append(ids, &s3.ObjectIdentifier{Key: o.Key})
		}
		for start := 0; start < len(ids); start += deleteBatchSize {
			end := start + deleteBatchSize
			if end > len(ids) {
				end = len(ids)
			}
			res, err := mys3.Client.DeleteObjectsWithContext(ctx, &s3.DeleteObjectsInput{
				Bucket: aws.String(bucket),
				Delete: &s3.Delete{Objects: ids[start:end], Quiet: aws.Bool(true)},
			})
			if err != nil {
				return removed, fmt.Errorf("mediastore.DeletePrefix: deleting under %s: %w", prefix, err)
			}
			removed += end - start - len(res.Errors)
			for _, e := range res.Errors {
				return removed, fmt.Errorf("mediastore.DeletePrefix: %s: %s", aws.StringValue(e.Key), aws.StringValue(e.Message))
			}
		}
		if out.IsTruncated == nil || !*out.IsTruncated {
			return removed, nil
		}
		continuation = out.NextContinuationToken
	}
}
