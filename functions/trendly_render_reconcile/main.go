// Package main implements the scheduled sweep that closes out renders which
// never reported back.
//
// A render is marked "queued" when it is submitted, "rendering" by the worker
// when it picks the job up, and "done" or "failed" when it finishes. It can
// strand at either of the first two:
//
//	"rendering" — the process vanished between its own writes: a Fargate task
//	              killed after its last Spot retry, an OOM, a Lambda timeout.
//	"queued"    — the job never started, so nothing has written to the document
//	              at all. A container that dies before it can parse its input
//	              cannot report its own failure; only this sweep can close it.
//
// Nothing else would ever correct those documents: the app shows a spinner
// nobody can cancel, and the publish gate waits for a job that is never coming
// back. So anything sitting in either status well past any plausible time is
// marked failed, with a message that offers the only useful next step.
package main

import (
	"context"
	"log"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/mysentry"
)

// stuckRenderingAfter is how long a render may sit in "rendering" before it is
// presumed dead. Generous on purpose: the Batch job definition allows 30
// minutes per attempt and up to 3 attempts, so this must not fire while work is
// genuinely still in flight. Declaring a live render dead would be worse than
// leaving a dead one a few minutes longer.
const stuckRenderingAfter = 45 * time.Minute

// stuckQueuedAfter covers the other way a render strands: it never starts at
// all. Shorter, because nothing is legitimately running — this is pure queue
// wait, and the only thing that can stretch it is a genuine backlog (the Spot
// compute environment caps at ~16 concurrent renders, so even a hundred queued
// jobs clear well inside this). It has to be a sweep rather than the worker
// reporting its own failure: a container that dies before it can parse its
// input has nothing to report with, and has not yet written to the document.
const stuckQueuedAfter = 30 * time.Minute

// sweepLimit bounds one run. A backlog larger than this means something
// systemic, and the next run picks up the rest.
const sweepLimit = 500

// revisionKind is one of the two subcollections a render reports into. They
// carry identical render fields but are separate collection groups, so a single
// query cannot see both — hence a sweep per kind rather than one that forgets
// the newer one exists.
type revisionKind struct {
	name string
	list func(ctx context.Context, status string, cutoffMs int64, limit int) ([]trendlymodels.StuckRender, error)
	fail func(brandID, contentID, revisionID, reason string) error
}

var revisionKinds = []revisionKind{
	{"designs", trendlymodels.ListStuckRenders, trendlymodels.MarkDesignRenderFailed},
	{"compositions", trendlymodels.ListStuckCompositionRenders, trendlymodels.MarkCompositionRenderFailed},
}

// sweep closes out one status in one subcollection. The message is the only
// thing the user will see, so it names what actually happened rather than a
// generic failure.
func sweep(ctx context.Context, kind revisionKind, status string, after time.Duration, message string) {
	cutoff := time.Now().Add(-after).UnixMilli()

	stuck, err := kind.list(ctx, status, cutoff, sweepLimit)
	if err != nil {
		log.Printf("render reconcile: %s %s query failed: %v", kind.name, status, err)
		mysentry.Capture(err, map[string]string{
			"lambda": "render_reconcile", "op": "query", "status": status, "kind": kind.name,
		})
		// Partial results are still worth acting on.
	}
	if len(stuck) == 0 {
		return
	}

	log.Printf("render reconcile: %d %s render(s) stuck in %q for over %s",
		len(stuck), kind.name, status, after)
	failed := 0
	for _, s := range stuck {
		markErr := kind.fail(s.BrandID, s.ContentID, s.RevisionID, message)
		if markErr != nil {
			log.Printf("render reconcile: could not fail %s %s/%s/%s: %v",
				kind.name, s.BrandID, s.ContentID, s.RevisionID, markErr)
			continue
		}
		failed++
	}
	log.Printf("render reconcile: closed %d/%d %s stuck in %q", failed, len(stuck), kind.name, status)
}

func handler(ctx context.Context) error {
	for _, kind := range revisionKinds {
		sweep(ctx, kind, trendlymodels.DesignRenderRendering, stuckRenderingAfter,
			"This render stopped unexpectedly. Please try again.")
		sweep(ctx, kind, trendlymodels.DesignRenderQueued, stuckQueuedAfter,
			"This render never started. Please try again.")
	}
	return nil
}

func main() {
	mysentry.Init()
	lambda.Start(mysentry.WrapCtx(handler))
}
