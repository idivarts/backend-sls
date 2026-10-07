// Package main implements the scheduled sweep that closes out renders which
// never reported back.
//
// A render is marked "rendering" by the worker when it picks a job up, and
// "done" or "failed" when it finishes. Between those two writes the process can
// simply vanish — a Fargate task killed after its last Spot retry, an OOM, a
// Lambda timeout. Nothing else would ever correct that document: the app shows
// a spinner nobody can cancel, and the publish gate waits for a job that is
// never coming back.
//
// So anything still "rendering" well past any plausible render time is marked
// failed, with a message that offers the only useful next step.
package main

import (
	"context"
	"log"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/mysentry"
)

// stuckAfter is how long a render may sit in "rendering" before it is presumed
// dead. Generous on purpose: the Batch job definition allows 30 minutes per
// attempt and up to 3 attempts, so this must not fire while work is genuinely
// still in flight. Declaring a live render dead would be worse than leaving a
// dead one a few minutes longer.
const stuckAfter = 45 * time.Minute

// sweepLimit bounds one run. A backlog larger than this means something
// systemic, and the next run picks up the rest.
const sweepLimit = 500

func handler(ctx context.Context) error {
	cutoff := time.Now().Add(-stuckAfter).UnixMilli()

	stuck, err := trendlymodels.ListStuckRenders(ctx, cutoff, sweepLimit)
	if err != nil {
		log.Printf("render reconcile: query failed: %v", err)
		mysentry.Capture(err, map[string]string{"lambda": "render_reconcile", "op": "query"})
		// Partial results are still worth acting on.
	}
	if len(stuck) == 0 {
		return nil
	}

	log.Printf("render reconcile: %d stuck render(s) older than %s", len(stuck), stuckAfter)
	failed := 0
	for _, s := range stuck {
		markErr := trendlymodels.MarkDesignRenderFailed(s.BrandID, s.ContentID, s.RevisionID,
			"This render stopped unexpectedly. Please try again.")
		if markErr != nil {
			log.Printf("render reconcile: could not fail %s/%s/%s: %v",
				s.BrandID, s.ContentID, s.RevisionID, markErr)
			continue
		}
		failed++
	}
	log.Printf("render reconcile: closed %d/%d", failed, len(stuck))
	return nil
}

func main() {
	mysentry.Init()
	lambda.Start(mysentry.WrapCtx(handler))
}
