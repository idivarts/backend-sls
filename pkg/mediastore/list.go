package mediastore

// What a design's project holds.
//
// Narrow like its neighbours: only a design project prefix, and only the
// relative paths. The media panel needs to show someone the files their
// film already has, and listing them is the one thing it cannot do from
// the document alone — a photograph resolved by the worker is in the
// directory before anything in the HTML mentions it.

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/idivarts/backend-sls/pkg/mys3"
)

// listCap bounds one listing. A project with more files than this is not
// a project any more.
const listCap = 500

// ListPrefix returns the paths under a design project prefix, relative to
// it.
func ListPrefix(ctx context.Context, t Target, prefix string) ([]string, error) {
	if !designProjectPrefix.MatchString(prefix) {
		return nil, fmt.Errorf("mediastore.ListPrefix: %q is not a design project", prefix)
	}
	bucket, _, _, err := resolveTarget(t)
	if err != nil {
		return nil, err
	}
	var out []string
	var continuation *string
	for len(out) < listCap {
		res, err := mys3.Client.ListObjectsV2WithContext(ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(bucket),
			Prefix:            aws.String(prefix + "/"),
			ContinuationToken: continuation,
		})
		if err != nil {
			return nil, fmt.Errorf("mediastore.ListPrefix: %s: %w", prefix, err)
		}
		for _, o := range res.Contents {
			rel := strings.TrimPrefix(aws.StringValue(o.Key), prefix+"/")
			// A design's other revisions live under its root; they are not
			// files of THIS one.
			if rel == "" || rel == "revisions" || strings.HasPrefix(rel, "revisions/") {
				continue
			}
			out = append(out, rel)
			if len(out) >= listCap {
				break
			}
		}
		if res.IsTruncated == nil || !*res.IsTruncated {
			break
		}
		continuation = res.NextContinuationToken
	}
	return out, nil
}
