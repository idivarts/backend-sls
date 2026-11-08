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
//
// RevisionID identifies both the revision to render and the document the worker
// reports into; DocType says which subcollection that is ("composition" means
// compositions/, anything else means designs/). One id rather than two, because
// no job ever needs both.
type RenderJob struct {
	BrandID    string `json:"brandId"`
	ContentID  string `json:"contentId"`
	RevisionID string `json:"revisionId"`
	DocType    string `json:"docType"`
	Force      bool   `json:"force,omitempty"`
	// SourceURL carries the video for an ingest or frame job, where RevisionID
	// is the media id rather than a revision.
	SourceURL string `json:"sourceUrl,omitempty"`
	// AtMs is how far into the source a `frame` job should cut. -1 means the
	// LAST frame, which is the one chaining actually wants and the one a
	// caller cannot compute without knowing the clip's exact length — the
	// worker has ffprobe and we do not.
	AtMs int `json:"atMs,omitempty"`
	// StudioID targets a Design Studio render. The worker reads the studio and
	// its scenes, converts them into a composition and renders that, so
	// `revisionId` carries the studio id for the status writers.
	StudioID string `json:"studioId,omitempty"`
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

	// An explicit revisionId names a DESIGN revision and bypasses the
	// composition precedence on purpose: it is how an older design revision is
	// re-rendered, and a caller that asked for one revision must not silently
	// get another.
	var target *RenderTarget
	if req.RevisionID != "" {
		revision, revErr := trendlymodels.GetDesignRevision(brandID, contentID, req.RevisionID)
		if revErr != nil || revision == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "revision not found"})
			return
		}
		target = DesignRenderTarget(revision)
	} else {
		target, err = ResolveRenderTarget(brandID, contentID, content)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "revision not found"})
			return
		}
	}
	if target == nil {
		// Distinguished so the message is true: a content with no media at all
		// is a different problem from one whose pointer has gone stale, and
		// "this content has no design" sent for the latter sends someone
		// looking in the wrong place.
		if content.DesignRef != nil || content.CompositionRef != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "revision not found"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "no_design", "message": "This content has no design to render."})
		return
	}

	// Already rendered and nobody asked for a new one: say so and charge nothing.
	if target.RenderURL != "" && !req.Force {
		c.JSON(http.StatusOK, gin.H{"status": trendlymodels.DesignRenderDone, "renderUrl": target.RenderURL})
		return
	}

	// A timeline with a clip that cannot render is refused BEFORE the charge.
	// The worker would reject it too, but by then the wallet has paid for an
	// attempt that was never going to produce anything.
	if target.Problem != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "not_renderable", "message": target.Problem})
		return
	}

	alreadyPaid := target.ChargedTokens > 0 && !req.Force

	// A user-initiated render is GATED on the wallet: refusing here is honest,
	// and the app can say what it would have cost.
	if !alreadyPaid && tokensExhausted(orgIDForBrand(brandID)) {
		c.JSON(http.StatusPaymentRequired, gin.H{
			"error":   "upgrade_required",
			"reason":  "tokens_exhausted",
			"feature": "design_render",
			"cost":    target.Tokens,
		})
		return
	}

	if err := SubmitTarget(brandID, contentID, target, req.Force, alreadyPaid); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error(), "message": "Could not start the render"})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"status":      trendlymodels.DesignRenderQueued,
		"tokensSpent": map[string]any{"charged": !alreadyPaid, "tokens": target.Tokens},
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
func SubmitTarget(brandID, contentID string, target *RenderTarget, force, alreadyPaid bool) error {
	if !alreadyPaid {
		if orgID := orgIDForBrand(brandID); orgID != "" {
			if _, err := trendlymodels.DeductTokens(orgID, target.Tokens); err != nil {
				log.Printf("render meter: deduct %d tokens for org %s: %v", target.Tokens, orgID, err)
			} else if err := target.markCharged(brandID, contentID, target.Tokens); err != nil {
				log.Printf("render meter: mark charged %s: %v", target.RevisionID, err)
			}
		}
	}

	// Stamp "queued" BEFORE dispatching, so the app shows the job the instant
	// this returns rather than sitting on a stale state until a worker starts.
	if err := target.markQueued(brandID, contentID, ""); err != nil {
		log.Printf("render: mark queued %s: %v", target.RevisionID, err)
	}

	err := dispatch(RenderJob{
		BrandID:    brandID,
		ContentID:  contentID,
		RevisionID: target.RevisionID,
		DocType:    target.DocType,
		Force:      force,
	})
	if err != nil {
		_ = target.markFailed(brandID, contentID,
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
	// A composition is a video encode with more steps, so it takes the video
	// lane: Fargate, not the image Lambda, whose ceiling an encode would hit.
	// Ingest joins the video lane: it downloads a whole file and writes a
	// dozen frames to disk, which the image Lambda's 512MB /tmp cannot take.
	// So does a frame extract, for the plainer reason that ffmpeg is in the
	// worker's container and nowhere else. A studio render IS a composition
	// render with one more resolution step in front of it, so it takes the
	// same lane for the same reason.
	if job.DocType == "video" || job.DocType == "composition" ||
		job.DocType == "ingest" || job.DocType == "frame" || job.DocType == "studio" {
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
		// The job travels as an environment variable, NOT a command override.
		// Batch's `command` is Docker CMD, which Docker appends as ARGUMENTS to
		// the image's ENTRYPOINT — and that entrypoint is already
		// `node .../cli.js`. Passing the full command here produced
		//     node cli.js node cli.js {"brandId":...}
		// so cli.js read argv[2] as the literal string "node" and died on
		// JSON.parse. RENDER_JOB is the other input cli.ts accepts and it is
		// immune to the entrypoint ever changing shape again.
		ContainerOverrides: &batch.ContainerOverrides{
			Environment: []*batch.KeyValuePair{
				{Name: aws.String("RENDER_JOB"), Value: aws.String(body)},
			},
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

// SubmitIngest asks the worker to look at an uploaded video.
//
// The video lane (Fargate), not the image Lambda: ingest downloads a whole
// file, probes it and writes a dozen frames to disk, which is exactly the
// shape the image Lambda's 512MB /tmp and short ceiling cannot take.
//
// Fire and forget, and deliberately not charged. Ingest is what makes an
// upload useful rather than a feature someone opted into, and billing a user
// for looking at their own file would be a strange place to meter. The vision
// pass that follows IS metered, where a model is actually called.
// SubmitStudioRender hands a whole studio to the worker.
//
// The worker resolves the studio and its scenes, converts them into a
// composition and renders that — so this carries only the three ids it needs to
// find them. Nothing about the timeline travels in the job, because a job that
// described the timeline could describe one that no longer matches the
// documents.
// ⚠️ `force` has to travel. It did not, and the consequence was a re-render
// that CHARGED AND DID NOTHING: the handler honoured `force` for its own gates
// and billed the wallet, the job went out with Force false, and the worker —
// which skips a studio that already has a renderUrl unless forced — logged
// "already rendered, skipping", exited successfully, and left the studio stuck
// on `queued` for ever. The user paid and watched a spinner that would never
// stop. SubmitTarget had carried force all along; only this one dropped it.
func SubmitStudioRender(brandID, contentID, studioID string, force bool) error {
	return dispatch(RenderJob{
		BrandID:    brandID,
		ContentID:  contentID,
		RevisionID: studioID,
		StudioID:   studioID,
		DocType:    "studio",
		Force:      force,
	})
}

// SubmitFrameExtract asks the worker to cut one frame out of a video and file
// it as its own piece of media.
//
// This is what makes chaining possible. A chained generation needs the LAST
// frame of the previous clip as its start frame, and `atMs: -1` says so rather
// than making the caller guess a duration — the worker has ffprobe, the backend
// does not, and a guess that lands past the end produces a black frame that
// then becomes the next clip's opening image.
//
// It is also how an anchor still gets made from a clip, and how "use the frame
// where she picks up the cup" becomes a real asset instead of a description.
func SubmitFrameExtract(brandID, sourceMediaID, url string, atMs int) error {
	return dispatch(RenderJob{
		BrandID:    brandID,
		ContentID:  "-",
		RevisionID: sourceMediaID,
		DocType:    "frame",
		SourceURL:  url,
		AtMs:       atMs,
	})
}

func SubmitIngest(brandID, assetID, url string) error {
	return dispatch(RenderJob{
		BrandID:    brandID,
		ContentID:  "-",
		RevisionID: assetID,
		DocType:    "ingest",
		SourceURL:  url,
	})
}
