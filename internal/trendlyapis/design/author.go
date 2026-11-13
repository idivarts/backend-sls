package design

// Authoring a v3 design, asynchronously.
//
// ── Why this is not an HTTP request ────────────────────────────────────────
// API Gateway's integration timeout is 29 seconds and is not configurable.
// Writing a Hyperframes project is a frontier model emitting several HTML
// documents; the first attempt against the chat route returned 504 with
// nothing stored. Raising the Lambda timeout cannot help, because the gateway
// gives up first.
//
// So this follows the path internal/studiojobs already laid for the director
// plan and the scene design, for the same measured reason: the request stores
// an intention and returns, and a worker with 900 seconds does the work. The
// design document carries the status, so the app polls what it is already
// subscribed to rather than holding a connection open.
//
// ── Why a plain completion and not the chat tool ───────────────────────────
// `write_hyperframes_project` exists for the interactive chat, where the user
// is steering. This job IS the authoring, so there is nothing to steer: it
// asks for the project, validates it with the same hfproject.Build, and
// persists it. One round trip instead of a tool loop, and the same validator
// either way — a project written here cannot be structurally different from
// one written in the chat.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/internal/studiojobs"
	"github.com/idivarts/backend-sls/internal/trendlyapis/ai"
	"github.com/idivarts/backend-sls/pkg/hfproject"
	"github.com/idivarts/backend-sls/pkg/mysentry"
	"github.com/idivarts/backend-sls/pkg/openrouter"
)

type authorDesignRequest struct {
	// Brief is what the user asked for, in their words.
	Brief string `json:"brief"`
}

// AuthorDesign queues the writing of a design's project.
// POST /api/design/brands/:brandId/designs/:designId/author
func AuthorDesign(c *gin.Context) {
	brandID, designID, ok := guard(c, true)
	if !ok {
		return
	}
	var req authorDesignRequest
	_ = c.ShouldBindJSON(&req)
	if strings.TrimSpace(req.Brief) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "brief is required"})
		return
	}

	ctx := c.Request.Context()
	d, err := trendlymodels.GetBrandDesign(ctx, brandID, designID)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not read the design"})
		return
	}
	if d == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no such design"})
		return
	}
	if d.AuthorStatus == trendlymodels.DesignAuthorQueued ||
		d.AuthorStatus == trendlymodels.DesignAuthorWriting {
		c.JSON(http.StatusOK, gin.H{"status": d.AuthorStatus, "message": "This is already being written."})
		return
	}

	// Checked BEFORE the status is written. Without a queue the design would
	// sit at "queued" for ever, and a spinner that never resolves looks like
	// slowness rather than breakage — the worst failure shape available.
	if !studiojobs.Configured() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "authoring is not available right now"})
		return
	}

	// Gated HERE, in the request, because this is the last place that can tell
	// the user anything: once the message is on the queue the only surface
	// left is an authorError on the document, which reads as a fault rather
	// than a wall.
	//
	// This was missing. The handler discarded `locked` from PickModelFor and
	// never looked at the wallet, so a brand whose plan unlocks no design
	// model — and a brand with an empty wallet — both queued a frontier-model
	// call that the org then paid for. The two reasons stay distinct because
	// they send the user to different places: a spent wallet wants a top-up,
	// a locked plan wants a tier.
	orgID, _ := trendlymodels.OrgIDForBrand(brandID)
	if trendlymodels.TokensExhausted(orgID) {
		c.JSON(http.StatusPaymentRequired, gin.H{
			"error":   "upgrade_required",
			"reason":  "tokens_exhausted",
			"feature": "design_author",
		})
		return
	}
	if model, locked := ai.PickModelFor(ctx, brandID, openrouter.TaskDesign); locked || model == "" {
		c.JSON(http.StatusPaymentRequired, gin.H{
			"error":   "upgrade_required",
			"reason":  "plan_locked",
			"feature": "design_author",
		})
		return
	}
	// The brief is stored with the status, not just carried on the queue
	// message, so the app can show the user what this design was asked to be
	// when they come back to it.
	brief := strings.TrimSpace(req.Brief)
	if err := trendlymodels.QueueBrandDesignAuthoring(ctx, brandID, designID, brief); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not queue the writing"})
		return
	}
	if err := studiojobs.Enqueue(studiojobs.Message{
		Type:     studiojobs.JobDesignAuthor,
		BrandID:  brandID,
		DesignID: designID,
		Brief:    brief,
	}); err != nil {
		log.Printf("design author enqueue failed for %s/%s: %v", brandID, designID, err)
		mysentry.Capture(err, map[string]string{"op": "design.author.enqueue", "brandId": brandID, "designId": designID})
		_ = trendlymodels.MarkBrandDesignAuthorFailed(ctx, brandID, designID,
			"We couldn't start writing this design. Please try again.")
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not queue the writing"})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{"status": trendlymodels.DesignAuthorQueued})
}

// authoredProject is what the model is asked to return.
type authoredProject struct {
	Files []struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	} `json:"files"`
}

// ExecuteDesignAuthorJob is the worker side. Called from studio_sqs.
func ExecuteDesignAuthorJob(ctx context.Context, msg studiojobs.Message) error {
	brandID, designID := msg.BrandID, msg.DesignID

	d, err := trendlymodels.GetBrandDesign(ctx, brandID, designID)
	if err != nil {
		return err
	}
	if d == nil {
		// Deleted between the enqueue and the run. An ordinary outcome.
		log.Printf("studiojobs: design_author for a missing design %s/%s", brandID, designID)
		return nil
	}
	// Idempotency. SQS is at-least-once, and a redelivery would spend a
	// second frontier-model call and overwrite a project the user may have
	// rendered since.
	if d.AuthorStatus != trendlymodels.DesignAuthorQueued {
		log.Printf("studiojobs: design_author already %s for %s, skipping", d.AuthorStatus, designID)
		return nil
	}
	if err := trendlymodels.MarkBrandDesignAuthoring(ctx, brandID, designID, trendlymodels.DesignAuthorWriting); err != nil {
		return err
	}

	// The request already gated this, so a lock here means the plan changed
	// between the enqueue and the run. Failing with a sentence beats calling a
	// model the plan does not allow and billing the org for it.
	model, locked := ai.PickModelFor(ctx, brandID, openrouter.TaskDesign)
	if locked || model == "" {
		return trendlymodels.MarkBrandDesignAuthorFailed(ctx, brandID, designID,
			"Writing designs is not available on this plan.")
	}

	genCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	resp, err := openrouter.ChatCompletion(genCtx, openrouter.ChatRequest{
		Model: model,
		Messages: []openrouter.Message{
			{Role: "system", Content: hyperframesAuthoringSystemPrompt(d)},
			{Role: "user", Content: msg.Brief},
		},
	})
	if err != nil {
		reason := "The model did not answer."
		if errors.Is(err, context.DeadlineExceeded) {
			reason = "Writing this design took too long. Try again, or shorten the brief."
		}
		return trendlymodels.MarkBrandDesignAuthorFailed(ctx, brandID, designID, reason)
	}
	ai.MeterBrandUsage(brandID, resp.Usage)

	raw := firstContent(resp)
	parsed, perr := parseAuthoredProject(raw)
	if perr != nil {
		return trendlymodels.MarkBrandDesignAuthorFailed(ctx, brandID, designID, perr.Error())
	}

	files := make([]hfproject.File, 0, len(parsed.Files))
	for _, f := range parsed.Files {
		files = append(files, hfproject.File{Path: f.Path, Content: f.Content})
	}
	project, berr := hfproject.Build(files)
	if berr != nil {
		// The validator's messages are already written to be read by a
		// person as well as a model, so they are surfaced rather than
		// replaced with something vaguer.
		return trendlymodels.MarkBrandDesignAuthorFailed(ctx, brandID, designID, berr.Error())
	}

	prefix, uerr := hfproject.Upload(ctx, brandID, designID, project)
	if uerr != nil {
		mysentry.Capture(uerr, map[string]string{"op": "design.author.upload", "brandId": brandID, "designId": designID})
		return trendlymodels.MarkBrandDesignAuthorFailed(ctx, brandID, designID,
			"We wrote the design but could not save it. Please try again.")
	}

	// The scenes travel with the project so the app can draw a timeline of
	// the film it just wrote, rather than only a status line about it.
	scenes := make([]trendlymodels.DesignScene, 0, len(project.Scenes))
	for _, s := range project.Scenes {
		scenes = append(scenes, trendlymodels.DesignScene{
			ID:         s.ID,
			Src:        s.Src,
			StartMs:    s.StartMs,
			DurationMs: s.DurationMs,
			TrackIndex: s.TrackIndex,
			TrackKind:  s.TrackKind,
		})
	}
	return trendlymodels.SetBrandDesignProject(ctx, brandID, designID, prefix, project.DurationMs, scenes)
}

func firstContent(resp *openrouter.ChatResponse) string {
	if resp == nil || len(resp.Choices) == 0 {
		return ""
	}
	return resp.Choices[0].Message.Content
}

// parseAuthoredProject pulls the JSON out of the model's answer.
//
// Models fence JSON in ```json blocks and add a sentence before it however
// firmly they are told not to, so the fence is stripped rather than treated
// as a failure — refusing a correct project over its wrapper would waste a
// frontier-model call and tell the user their design was bad.
func parseAuthoredProject(raw string) (*authoredProject, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return nil, fmt.Errorf("The model returned nothing.")
	}
	if i := strings.Index(s, "```"); i >= 0 {
		rest := s[i+3:]
		if j := strings.Index(rest, "\n"); j >= 0 {
			rest = rest[j+1:]
		}
		if k := strings.LastIndex(rest, "```"); k >= 0 {
			rest = rest[:k]
		}
		s = strings.TrimSpace(rest)
	}
	start, end := strings.Index(s, "{"), strings.LastIndex(s, "}")
	if start < 0 || end <= start {
		return nil, fmt.Errorf("The model's answer was not a project.")
	}
	var p authoredProject
	if err := json.Unmarshal([]byte(s[start:end+1]), &p); err != nil {
		return nil, fmt.Errorf("The model's answer could not be read as a project.")
	}
	if len(p.Files) == 0 {
		return nil, fmt.Errorf("The model returned no files.")
	}
	return &p, nil
}

// hyperframesAuthoringSystemPrompt is the contract plus this design's canvas
// and the output shape.
//
// The contract itself lives in pkg/hfproject so the chat tool and this job
// cannot drift apart. The only thing added here is what differs per design.
func hyperframesAuthoringSystemPrompt(d *trendlymodels.Design) string {
	return fmt.Sprintf(`You write Hyperframes projects for a social video tool.

%s

THIS FILM'S CANVAS: %dx%d at %d fps. Use exactly these numbers for data-width
and data-height on the root and on every scene.

ANSWER WITH JSON AND NOTHING ELSE, in this shape:

{"files":[{"path":"index.html","content":"…"},{"path":"compositions/hook.html","content":"…"}]}

Every file the project needs must be in that array, including index.html.
Paths are relative, with forward slashes. Do not wrap the JSON in prose.`,
		hfproject.AuthoringContract, d.Format.Width, d.Format.Height, d.Format.FPS)
}
