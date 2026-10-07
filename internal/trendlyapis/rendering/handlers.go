// Package rendering starts server-side renders of a content's HTML design.
//
// A design is HTML until something renders it. That something used to be the
// user's own browser (html2canvas + WebCodecs), which meant renders were
// impossible on native, impossible without a tab open, and painted by a CSS
// reimplementation rather than by Chromium. The render now happens on our
// infrastructure; this package is the door to it.
//
// Two lanes, because the two jobs bill differently:
//
//	image / carousel → SQS → a Lambda running the worker container
//	video            → AWS Batch → a Fargate Spot task, which exits when done
//
// Both are fire-and-forget: the handler returns 202 and the app watches the
// revision document it is already subscribed to.
package rendering

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/batch"
	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	sqshandler "github.com/idivarts/backend-sls/pkg/sqs_handler"
)

// RenderJob is the worker's input. Keep in sync with the worker's types.ts.
type RenderJob struct {
	BrandID    string `json:"brandId"`
	ContentID  string `json:"contentId"`
	RevisionID string `json:"revisionId"`
	DocType    string `json:"docType"`
	Force      bool   `json:"force,omitempty"`
}

type startRenderRequest struct {
	// RevisionID defaults to the content's current designRef revision.
	RevisionID string `json:"revisionId"`
	// Force re-renders a revision that already has a renderUrl, and charges
	// again — it is a deliberate new request, not a retry.
	Force bool `json:"force"`
}

// StartRender queues a render of the content's current design revision.
// POST /api/v2/brands/:brandId/contents/:contentId/render
func StartRender(c *gin.Context) {
	brandID := c.Param("brandId")
	contentID := c.Param("contentId")

	// Rendering is an edit of the post's media, not a publish — an editor may
	// render, and the publish gate stays where it is.
	if _, ok := middlewares.RequireFeaturePrivilege(c, brandID,
		trendlymodels.FeatureContentCalendar, trendlymodels.PrivCalendarEditor); !ok {
		return
	}

	var req startRenderRequest
	_ = c.ShouldBindJSON(&req) // an empty body is the common case

	content, err := trendlymodels.GetContent(brandID, contentID)
	if err != nil || content == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "content not found"})
		return
	}
	if content.DesignRef == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no_design", "message": "This content has no design to render."})
		return
	}

	revisionID := req.RevisionID
	if revisionID == "" {
		revisionID = content.DesignRef.RevisionID
	}
	revision, err := trendlymodels.GetDesignRevision(brandID, contentID, revisionID)
	if err != nil || revision == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "revision not found"})
		return
	}

	// Already rendered and nobody asked for a new one: say so and charge nothing.
	if revision.RenderURL != "" && !req.Force {
		c.JSON(http.StatusOK, gin.H{"status": trendlymodels.DesignRenderDone, "renderUrl": revision.RenderURL})
		return
	}

	tokens := trendlymodels.RenderTokenCost(revision.DocType, revision.SlideCount, revision.DurationMs)
	alreadyPaid := revision.RenderChargedTokens > 0 && !req.Force

	// A user-initiated render is GATED on the wallet: refusing here is honest,
	// and the app can say what it would have cost.
	if !alreadyPaid && tokensExhausted(orgIDForBrand(brandID)) {
		c.JSON(http.StatusPaymentRequired, gin.H{
			"error":   "upgrade_required",
			"reason":  "tokens_exhausted",
			"feature": "design_render",
			"cost":    tokens,
		})
		return
	}

	if err := Submit(brandID, contentID, revisionID, revision.DocType, req.Force, alreadyPaid, tokens); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error(), "message": "Could not start the render"})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"status":      trendlymodels.DesignRenderQueued,
		"tokensSpent": map[string]any{"charged": !alreadyPaid, "tokens": tokens},
	})
}

// Submit charges, stamps and dispatches a render. Shared by the HTTP route and
// by the scheduler's pre-publish check, so there is one definition of what
// starting a render means.
//
// It does NOT gate on the wallet — the caller decides that. A person pressing
// Render is refused when they are out of tokens; a scheduled post is not, since
// failing a publish the user set up days ago over a token balance would be a
// worse outcome than letting the wallet go slightly negative.
func Submit(brandID, contentID, revisionID, docType string, force, alreadyPaid bool, tokens int64) error {
	if !alreadyPaid {
		if orgID := orgIDForBrand(brandID); orgID != "" {
			if _, err := trendlymodels.DeductTokens(orgID, tokens); err != nil {
				log.Printf("render meter: deduct %d tokens for org %s: %v", tokens, orgID, err)
			} else if err := trendlymodels.MarkDesignRenderCharged(brandID, contentID, revisionID, tokens); err != nil {
				log.Printf("render meter: mark charged %s: %v", revisionID, err)
			}
		}
	}

	// Stamp "queued" BEFORE dispatching, so the app shows the job the instant
	// this returns rather than sitting on a stale state until a worker starts.
	if err := trendlymodels.MarkDesignRenderQueued(brandID, contentID, revisionID, ""); err != nil {
		log.Printf("render: mark queued %s: %v", revisionID, err)
	}

	err := dispatch(RenderJob{
		BrandID:    brandID,
		ContentID:  contentID,
		RevisionID: revisionID,
		DocType:    docType,
		Force:      force,
	})
	if err != nil {
		_ = trendlymodels.MarkDesignRenderFailed(brandID, contentID, revisionID,
			"We couldn't start the render. Please try again.")
	}
	return err
}

// dispatch sends the job to the lane that fits its output.
func dispatch(job RenderJob) error {
	body, err := json.Marshal(job)
	if err != nil {
		return err
	}
	if job.DocType == "video" {
		return submitBatchJob(job, string(body))
	}
	queueURL := os.Getenv("IMAGE_RENDER_QUEUE_URL")
	if queueURL == "" {
		return errNoQueue
	}
	return sqshandler.SendToQueue(queueURL, string(body), 0)
}

// submitBatchJob hands a video render to AWS Batch. Batch IS the queue here —
// it already provides ordering, retries and Spot-interruption handling, so
// putting SQS in front of it would be a second queue doing nothing.
func submitBatchJob(job RenderJob, body string) error {
	queue := os.Getenv("VIDEO_RENDER_JOB_QUEUE")
	definition := os.Getenv("VIDEO_RENDER_JOB_DEFINITION")
	if queue == "" || definition == "" {
		return errNoQueue
	}
	sess, err := session.NewSession()
	if err != nil {
		return err
	}
	_, err = batch.New(sess).SubmitJob(&batch.SubmitJobInput{
		JobName:       aws.String("render-" + job.RevisionID),
		JobQueue:      aws.String(queue),
		JobDefinition: aws.String(definition),
		ContainerOverrides: &batch.ContainerOverrides{
			Command: []*string{aws.String("node"), aws.String("dist/services/render-worker/src/cli.js"), aws.String(body)},
		},
	})
	return err
}

// orgIDForBrand resolves a brand's parent organization. An empty result means
// the brand predates organizations — those are never metered and never blocked,
// exactly as the AI token meter treats them.
func orgIDForBrand(brandID string) string {
	var b trendlymodels.Brand
	if err := b.Get(brandID); err != nil || b.OrganizationID == nil {
		return ""
	}
	return *b.OrganizationID
}

// tokensExhausted blocks only an org that HAS a wallet and has drained it.
// An org without a provisioned wallet is never locked out.
func tokensExhausted(orgID string) bool {
	if orgID == "" {
		return false
	}
	w, err := trendlymodels.GetTokenWallet(orgID)
	if err != nil || w == nil {
		return false
	}
	return (w.Balance + w.TopupBalance) <= 0
}

// errNoQueue means the stage has no render infrastructure wired yet. Surfaced
// rather than silently no-oping: a render that never happens must not look like
// a render in progress.
var errNoQueue = errors.New("render queue is not configured for this stage")
