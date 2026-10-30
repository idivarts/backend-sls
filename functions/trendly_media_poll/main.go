// Package main implements the scheduled sweep that collects generated media
// jobs the provider never called back about.
//
// Generation is asynchronous and the provider can notify us by webhook, but a
// webhook is not a guarantee: it can be unconfigured on a workspace, blocked by
// a network, or simply never sent. Without this sweep every one of those leaves
// a clip queued on a timeline forever, and the first symptom anyone sees is a
// composition that will not finish.
//
// It also closes out jobs that are genuinely lost — pending far past any
// plausible generation time — and refunds them. A clip stuck on "queued"
// forever is worse than one that admits it did not happen.
package main

import (
	"context"
	"log"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/idivarts/backend-sls/internal/trendlyapis/mediajobs"
	"github.com/idivarts/backend-sls/pkg/mysentry"
)

func handler(ctx context.Context) error {
	if err := mediajobs.PollPending(ctx); err != nil {
		log.Printf("media poll: %v", err)
		mysentry.Capture(err, map[string]string{"lambda": "media_poll"})
	}
	return nil
}

func main() {
	mysentry.Init()
	lambda.Start(mysentry.WrapCtx(handler))
}
