// Package mediajobs owns the life of an asynchronous generation: submitting
// one, collecting it when it lands, and putting the result where it belongs.
//
// A generated clip takes minutes, so none of this can happen inside the request
// that asked for it. The job gets a document, the timeline shows the clip as
// queued, and the result arrives later — by webhook when the provider calls us,
// by the polling cron when it does not.
//
// ⚠️ Both paths exist on purpose. A webhook that is misconfigured, blocked, or
// simply never sent would otherwise leave every clip queued forever, and the
// first symptom would be a timeline that never finishes. Resolution is
// idempotent precisely so the two can race.
package mediajobs

import (
	"context"
	"log"
	"time"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/mediagen"
)

// Resolve records a terminal result for a job and puts the clip on the
// timeline. Safe to call twice for the same job — only the first call does
// anything, which is what lets a webhook and a poll arrive in either order.
func Resolve(ctx context.Context, job *trendlymodels.MediaJob, res mediagen.Result) error {
	switch res.Status {
	case mediagen.StatusPending:
		return nil
	case mediagen.StatusSucceeded:
		return resolveSucceeded(ctx, job, res)
	default:
		return resolveFailed(ctx, job, res.Error)
	}
}

func resolveSucceeded(ctx context.Context, job *trendlymodels.MediaJob, res mediagen.Result) error {
	if len(res.Assets) == 0 || res.Assets[0].URL == "" {
		return resolveFailed(ctx, job, "The clip finished but came back empty.")
	}

	// Copy the bytes BEFORE marking the job done. The provider's URL is signed
	// and expires about an hour out, so a job marked succeeded against that URL
	// is a clip that stops working by lunchtime — and the composition it sits
	// in is a document people re-render weeks later.
	//
	// Done first, too, so a failed copy leaves the job pending and the next
	// poll tries again with a freshly signed URL, rather than leaving a
	// succeeded job pointing at nothing.
	hosted, err := hostFromURL(res.Assets[0].URL, job.BrandID)
	if err != nil || hosted == "" {
		log.Printf("mediajobs: could not copy %s to storage: %v", job.ID, err)
		return err
	}

	changed, err := trendlymodels.ResolveMediaJob(ctx, job.ID, trendlymodels.MediaJobSucceeded, hosted, "")
	if err != nil {
		return err
	}
	if !changed {
		// Someone else got there first; the clip is already placed.
		return nil
	}

	placed, err := trendlymodels.SetCompositionClipURL(
		job.BrandID, job.ContentID, job.CompositionID, job.ClipID, hosted)
	if err != nil {
		return err
	}
	if !placed {
		// By the time a generation finishes the user may well have deleted the
		// clip. That is an ordinary outcome, not an error — but the asset is
		// still theirs, and it is already in the library below.
		log.Printf("mediajobs: clip %s was gone by the time %s landed", job.ClipID, job.ID)
	}

	// Into the library either way, so a clip that was paid for is reusable even
	// if the timeline it was made for has moved on.
	if _, _, aerr := trendlymodels.RecordAsset(ctx, job.BrandID, &trendlymodels.Asset{
		Kind:            trendlymodels.AssetKindVideo,
		Source:          trendlymodels.AssetSourceGenerated,
		URL:             hosted,
		Title:           job.Prompt,
		Provider:        job.Provider,
		OriginContentID: job.ContentID,
	}); aerr != nil {
		log.Printf("mediajobs: could not file %s in the library: %v", job.ID, aerr)
	}
	return nil
}

func resolveFailed(ctx context.Context, job *trendlymodels.MediaJob, reason string) error {
	if reason == "" {
		reason = "The clip couldn't be generated. Please try again."
	}
	changed, err := trendlymodels.ResolveMediaJob(ctx, job.ID, trendlymodels.MediaJobFailed, "", reason)
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}

	// Give the tokens back. The provider does not charge for a failed
	// generation, so neither should we — and the refund is guarded by its own
	// flag because a webhook and a poll can both reach this.
	if job.ChargedTokens > 0 && job.OrganizationID != "" {
		ok, rerr := trendlymodels.MarkMediaJobRefunded(ctx, job.ID)
		if rerr != nil {
			log.Printf("mediajobs: refund guard for %s: %v", job.ID, rerr)
			return nil
		}
		if ok {
			if _, aerr := trendlymodels.RefundTokens(job.OrganizationID, job.ChargedTokens); aerr != nil {
				log.Printf("mediajobs: could not refund %d tokens for %s: %v",
					job.ChargedTokens, job.ID, aerr)
			}
		}
	}
	return nil
}

// pollLimit bounds one sweep. A backlog bigger than this means something
// systemic, and the next run picks up the rest.
const pollLimit = 200

// staleAfter is when a job that never reported is presumed lost. Generous:
// a long clip on a busy provider can genuinely take many minutes, and calling
// a live generation dead is worse than leaving a dead one a little longer.
const staleAfter = 45 * time.Minute

// PollPending asks the provider about every job still in flight.
//
// This is what makes the webhook optional rather than load-bearing. It also
// closes out jobs the provider has forgotten: a generation still pending long
// past any plausible time is marked failed and refunded, because a clip stuck
// on "queued" forever is worse than one that admits it did not happen.
func PollPending(ctx context.Context) error {
	jobs, err := trendlymodels.ListPendingMediaJobs(ctx, pollLimit)
	if err != nil {
		return err
	}
	cutoff := time.Now().Add(-staleAfter).UnixMilli()

	for i := range jobs {
		job := jobs[i]
		res, perr := mediagen.Get(ctx, job.Provider, job.ID)
		if perr != nil {
			log.Printf("mediajobs: poll %s: %v", job.ID, perr)
			// A provider we cannot reach is not a failed job — unless it has
			// been pending so long that nothing is coming.
			if job.SubmittedAt < cutoff {
				_ = resolveFailed(ctx, &job, "This clip didn't finish in time. Please try again.")
			}
			continue
		}
		if res.Status == mediagen.StatusPending && job.SubmittedAt < cutoff {
			_ = resolveFailed(ctx, &job, "This clip didn't finish in time. Please try again.")
			continue
		}
		if rerr := Resolve(ctx, &job, res); rerr != nil {
			log.Printf("mediajobs: resolve %s: %v", job.ID, rerr)
		}
	}
	return nil
}
