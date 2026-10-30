package publishing

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/internal/trendlyapis/rendering"
	sqshandler "github.com/idivarts/backend-sls/pkg/sqs_handler"
)

// ── Never publish a design that hasn't been rendered ────────────────────────
//
// This closes the hole the Media Stage's render bar exists to WARN about. A
// design is HTML until something renders it, and the attachments a content
// carries may have come from an earlier revision — so a scheduled post could
// quietly publish a version that doesn't match what the user approved. The bar
// could only warn, because rendering needed a human with a browser open.
//
// It doesn't any more. If the current revision has no render, the scheduler
// starts one and comes back, rather than publishing the wrong thing or failing.

// renderWaitDelaySeconds is how long to wait before re-checking a render that
// is still running. Long enough that a carousel is usually done on the first
// re-check, short enough that a scheduled post is not meaningfully late.
const renderWaitDelaySeconds int64 = 60

// maxRenderWaits bounds the deferral loop. Ten minutes is far longer than any
// render should take, so reaching it means something is wrong — and a publish
// that waits forever is indistinguishable to the user from one that is stuck.
const maxRenderWaits = 10

// ensureRendered reports whether the publish should be DEFERRED because the
// content's design still has to be rendered.
//
// Returns (false, nil) for everything that is ready to publish now — including
// content with no design at all, which is the common case.
func ensureRendered(brandID, contentID string, ct *trendlymodels.Content, attempt int) (bool, error) {
	// ResolveRenderTarget, not DesignRef directly: a content with a timeline
	// publishes the timeline, and a gate that checked the design instead would
	// wave through a single scene of a multi-clip reel as "already rendered".
	target, err := rendering.ResolveRenderTarget(brandID, contentID, ct)
	if err != nil || target == nil {
		// Can't tell, or nothing to render — publish what we have rather than
		// blocking on a read.
		return false, nil
	}
	if target.RenderURL != "" {
		return false, nil
	}

	if attempt >= maxRenderWaits {
		return false, fmt.Errorf("the design for this post could not be rendered in time")
	}

	if target.InFlight() {
		// Already running — just come back.
		return true, nil
	}

	// A timeline that cannot be rendered will never become publishable however
	// long the gate waits, so it fails now with the reason rather than burning
	// ten deferrals to arrive at a generic timeout.
	if target.Problem != "" {
		return false, fmt.Errorf("%s", target.Problem)
	}

	// Not rendered and nothing running: start it. Charged, but never gated —
	// see rendering.SubmitTarget.
	alreadyPaid := target.ChargedTokens > 0
	if err := rendering.SubmitTarget(brandID, contentID, target, false, alreadyPaid); err != nil {
		return false, fmt.Errorf("could not render the design for this post: %w", err)
	}
	log.Printf("publishing: deferred content %s pending render of revision %s", contentID, target.RevisionID)
	return true, nil
}

// PublishOrDeferForRender is what the queue worker calls instead of
// PublishContent: it publishes when the design is ready, and re-queues itself
// when a render still has to happen first.
func PublishOrDeferForRender(msg ScheduleMessage) error {
	ct, err := trendlymodels.GetContent(msg.BrandID, msg.ContentID)
	if err != nil || ct == nil {
		// Let PublishContent produce the real error for a missing content.
		return PublishContent(msg.BrandID, msg.ContentID, msg.OnlyDestinations...)
	}

	deferred, gateErr := ensureRendered(msg.BrandID, msg.ContentID, ct, msg.RenderWaitAttempt)
	if gateErr != nil {
		// Out of patience. Say so on the document — a post stuck on
		// "publishing" with no explanation is the worst of the options.
		failAllDestinations(msg.BrandID, msg.ContentID, ct, gateErr.Error())
		return gateErr
	}
	if deferred {
		next := msg
		next.RenderWaitAttempt = msg.RenderWaitAttempt + 1
		return requeueAfterRender(next)
	}

	return PublishContent(msg.BrandID, msg.ContentID, msg.OnlyDestinations...)
}

// requeueAfterRender puts the publish back on the queue with a delay, to be
// re-checked once the render has had time to finish.
func requeueAfterRender(msg ScheduleMessage) error {
	if os.Getenv("SEND_MESSAGE_QUEUE_ARN") == "" {
		// Local dev has no queue; publishing inline would skip the render the
		// gate just started, so stop here rather than publish the wrong media.
		log.Printf("publishing: no queue configured, cannot wait for render of %s", msg.ContentID)
		return nil
	}
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return sqshandler.SendToMessageQueue(string(body), renderWaitDelaySeconds)
}

// failAllDestinations records a content-level failure on every destination that
// was going to publish, so the app shows a real reason and a retry.
func failAllDestinations(brandID, contentID string, ct *trendlymodels.Content, reason string) {
	results := buildSeedResults(ct, map[string]bool{})
	for i := range results {
		if results[i].Status == pubStatusPublishing {
			results[i].Status = pubStatusFailed
			results[i].Error = reason
		}
	}
	writePublishState(brandID, contentID, results, true)
}
