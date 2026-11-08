package mediajobs

// Hosting a provider's output.
//
// This used to put the bytes in the bucket itself. It now goes through
// pkg/mediastore, which stores AND files in one step — the two must not be
// separable, because a generated clip that is stored but not filed is a clip
// the brand paid for and can never find again.

import (
	"context"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/mediastore"
)

// hostFromURL copies a provider's output into our own bucket, files it in the
// brand's library, and returns the CDN url plus the library id.
//
// Not optional for generated video. The provider hands back a SIGNED url that
// expires about an hour out, and a timeline is a document people re-render
// weeks later — a clip pointing at an expired url is a timeline nobody can
// render again.
//
// The library row carries the model and the prompt, which is how "the clip
// where she picks up the cup" is findable afterwards.
func hostFromURL(ctx context.Context, url string, job *trendlymodels.MediaJob) (string, string, error) {
	stored, err := mediastore.HostFromURL(ctx, mediastore.Spec{
		BrandID: job.BrandID,
		Target:  mediastore.TargetImage,
		Kind:    trendlymodels.AssetKindVideo,
		Source:  trendlymodels.AssetSourceGenerated,
		Title:   job.Prompt,
		Origin: &trendlymodels.AssetOrigin{
			Kind:      trendlymodels.AssetOriginGenerated,
			Provider:  job.Provider,
			Model:     job.ModelID,
			Prompt:    job.Prompt,
			JobID:     job.ID,
			ContentID: job.ContentID,
		},
		ContentID: job.ContentID,
		CreatedBy: job.CreatedBy,
	}, url)
	if err != nil {
		return "", "", err
	}
	return stored.URL, stored.MediaID, nil
}
