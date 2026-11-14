package mediastore

// Copying a design's project from one revision to the next.
//
// ── Why this is server-side, and why it exists at all ────────────────────
// A revision is a WHOLE project: index.html plus the brand's fonts, its
// logo, any resolved photograph or soundtrack. An inspector edit changes
// one attribute of one element — but the revision it produces still has to
// be complete, or the film it restores to would be a document with no
// assets.
//
// Reading several megabytes into a Lambda and writing them straight back
// would be the obvious way to do that, and it would make every colour
// change cost the whole project twice over the wire. S3 copies objects
// server-side; the bytes never leave the bucket.
//
// Like DeletePrefix, this is deliberately narrow: both ends must be design
// project prefixes, so no caller can use it to move a brand's media
// library somewhere.

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/idivarts/backend-sls/pkg/mys3"
)

// designProjectPrefix matches a design's root or any revision under it.
var designProjectPrefix = regexp.MustCompile(`^design-projects/[A-Za-z0-9_-]+/[A-Za-z0-9_-]+(/revisions/[1-9][0-9]*)?$`)

// CopyPrefix copies every object under src to dst, keeping the relative
// layout, and returns the relative paths it copied.
//
// `skip` names relative paths NOT to copy — the caller is writing its own
// version of those. Comparison is exact, so "index.html" skips the root
// document and nothing else.
func CopyPrefix(ctx context.Context, t Target, src, dst string, skip ...string) ([]string, error) {
	if !designProjectPrefix.MatchString(src) || !designProjectPrefix.MatchString(dst) {
		return nil, fmt.Errorf("mediastore.CopyPrefix: %q → %q is not a design project copy", src, dst)
	}
	if src == dst {
		return nil, fmt.Errorf("mediastore.CopyPrefix: source and destination are the same")
	}
	bucket, _, _, err := resolveTarget(t)
	if err != nil {
		return nil, err
	}
	omit := map[string]bool{}
	for _, s := range skip {
		omit[s] = true
	}

	var copied []string
	var continuation *string
	for {
		out, err := mys3.Client.ListObjectsV2WithContext(ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(bucket),
			Prefix:            aws.String(src + "/"),
			ContinuationToken: continuation,
		})
		if err != nil {
			return copied, fmt.Errorf("mediastore.CopyPrefix: listing %s: %w", src, err)
		}
		for _, o := range out.Contents {
			key := aws.StringValue(o.Key)
			rel := strings.TrimPrefix(key, src+"/")
			if rel == "" || omit[rel] {
				continue
			}
			// A revision copied from a design ROOT must not drag the
			// design's other revisions along with it.
			if rel == "revisions" || strings.HasPrefix(rel, "revisions/") {
				continue
			}
			if _, err := mys3.Client.CopyObjectWithContext(ctx, &s3.CopyObjectInput{
				Bucket: aws.String(bucket),
				// The source is a /-joined, URL-ESCAPED bucket+key. An
				// unescaped space or '+' in a filename silently copies the
				// wrong object, or none.
				CopySource:        aws.String(url.PathEscape(bucket + "/" + key)),
				Key:               aws.String(dst + "/" + rel),
				MetadataDirective: aws.String("COPY"),
			}); err != nil {
				return copied, fmt.Errorf("mediastore.CopyPrefix: copying %s: %w", rel, err)
			}
			copied = append(copied, rel)
		}
		if out.IsTruncated == nil || !*out.IsTruncated {
			return copied, nil
		}
		continuation = out.NextContinuationToken
	}
}
