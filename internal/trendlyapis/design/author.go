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
	"github.com/idivarts/backend-sls/internal/trendlyapis/rendering"
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

// The model returns the HTML itself, not JSON carrying the HTML.
//
// ── Why the JSON wrapper went ─────────────────────────────────────────────
// It asked the model to escape 9 KB of HTML, CSS and JavaScript into a JSON
// string, and one bad escape destroys the whole answer. That is exactly how
// it failed on dev: finish_reason "stop", the braces balanced, the shape
// right — and json.Unmarshal refused it, because somewhere in the middle was
// a backslash that is legal in a CSS content property or a JS regex and
// illegal in JSON. The user saw "The model's answer could not be read as a
// project" and had no way to act on it.
//
// A project is ONE file now (see hfproject.AuthoringContract), so the array
// was carrying nothing anyway. Asking for the document itself removes the
// escaping, removes the wrapper's tokens, and removes the failure mode.

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

	// The brand kit, before the model call, because it changes the prompt.
	// A brand with no design system gets nil and everything below behaves
	// exactly as it did before this existed. A failed asset fetch is not an
	// authoring failure: the film is still worth making without the logo.
	var kit *brandKit
	if ds, dserr := trendlymodels.GetDesignSystem(ctx, brandID); dserr == nil {
		kit = buildBrandKit(ctx, ds)
	} else {
		log.Printf("design author: could not read the design system for %s: %v", brandID, dserr)
	}

	genCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	resp, err := openrouter.ChatCompletion(genCtx, openrouter.ChatRequest{
		Model: model,
		Messages: []openrouter.Message{
			{Role: "system", Content: hyperframesAuthoringSystemPrompt(d, kit)},
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

	// A truncated answer is NOT a bad answer, and must not be reported as
	// one. The model ran out of room mid-JSON, so the project is unparseable
	// for a reason the user can act on — a shorter film — rather than
	// anything wrong with what they asked for. Without this it surfaced as
	// "The model's answer could not be read as a project", which sends them
	// to rewrite a brief that was never the problem.
	if finishReason(resp) == "length" {
		log.Printf("design author truncated for %s/%s: %d completion tokens, %d chars",
			brandID, designID, completionTokens(resp), len(raw))
		return trendlymodels.MarkBrandDesignAuthorFailed(ctx, brandID, designID,
			"This design was too long to write in one go. Try a shorter film, or fewer scenes.")
	}

	indexHTML, perr := parseAuthoredIndex(raw)
	if perr != nil {
		// The raw answer is the ONLY evidence of why this failed, and it is
		// gone the moment this returns. A head and a tail rather than the
		// whole thing: the tail is where a truncation shows, the head is
		// where a refusal or a stray preamble does, and the middle is HTML
		// nobody needs in a log line.
		log.Printf("design author parse failed for %s/%s (%d chars, finish=%q): head=%q tail=%q",
			brandID, designID, len(raw), finishReason(resp), snippet(raw, 240), tailSnippet(raw, 240))
		mysentry.Capture(perr, map[string]string{
			"op": "design.author.parse", "brandId": brandID, "designId": designID,
			"finishReason": finishReason(resp), "chars": fmt.Sprint(len(raw)),
		})
		return trendlymodels.MarkBrandDesignAuthorFailed(ctx, brandID, designID, perr.Error())
	}

	// The brand's tokens go in whatever the model did, and its assets travel
	// with the project so a render needs no network. Both are no-ops for a
	// brand with no design system.
	files := []hfproject.File{{Path: "index.html", Content: indexHTML}}
	if kit != nil {
		files[0].Content = hfproject.InjectIntoHead(indexHTML, kit.CSS)
		files = append(files, kit.Files...)
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
	// The live player. Best-effort on purpose: a design whose player could
	// not be written still renders, previews as stills and exports, so this
	// must not fail an authoring run the user has already paid for.
	previewURL, perr2 := hfproject.UploadPlayer(ctx, brandID, designID, project)
	if perr2 != nil {
		log.Printf("design player upload failed for %s/%s: %v", brandID, designID, perr2)
		mysentry.Capture(perr2, map[string]string{"op": "design.author.player", "brandId": brandID, "designId": designID})
	}

	if err := trendlymodels.SetBrandDesignProject(ctx, brandID, designID, prefix, project.DurationMs, scenes, previewURL); err != nil {
		return err
	}

	// ── Finish the film without being asked ──────────────────────────────
	// Authoring produces a document that DECLARES its media; it does not
	// have any. Resolving it needs the Hyperframes CLI, which lives in the
	// worker, so until now the user had to press "Preview — free" to make
	// the photograph exist — and a film they opened before pressing it was
	// missing the very thing the brief asked for.
	//
	// So the same job runs automatically here. It resolves the media, puts
	// the film through `check`, and captures the stills. By the time the
	// design opens, the live player IS the finished film and the only
	// button left is Render.
	//
	// Best-effort, like the player upload above: a dispatch that fails must
	// not fail an authoring run the user has already paid for. The design
	// is written and playable; what it lacks is its media, and the error
	// says so.
	if err := markAndPrepare(ctx, brandID, designID); err != nil {
		log.Printf("design %s/%s: could not start preparation: %v", brandID, designID, err)
		mysentry.Capture(err, map[string]string{"op": "design.author.prepare", "brandId": brandID, "designId": designID})
		// The status was set to queued before dispatch so the app shows the
		// job immediately. A dispatch that then fails must not leave it
		// there: a spinner for a job nobody will run reads as slowness
		// rather than breakage, and the design never resolves. The same
		// rule the render handler follows, for the same reason.
		if merr := trendlymodels.MarkBrandDesignPreviewFailed(ctx, brandID, designID,
			"We wrote the design but could not start finishing it. Try preparing it again."); merr != nil {
			log.Printf("design %s/%s: could not clear the stranded preparation: %v", brandID, designID, merr)
		}
	}
	return nil
}

// markAndPrepare queues the work that turns an authored document into a
// finished film: media resolved, gate passed, stills captured.
//
// Queued before dispatch for the same reason the handler does it: the app
// should show the job the instant the design appears, rather than a film
// that looks done and is missing its photograph.
func markAndPrepare(ctx context.Context, brandID, designID string) error {
	if err := trendlymodels.MarkBrandDesignPreviewQueued(ctx, brandID, designID); err != nil {
		return err
	}
	orgID, _ := trendlymodels.OrgIDForBrand(brandID)
	return rendering.SubmitDesignV3Preview(brandID, designID, MediaCreditCeiling(orgID))
}

// finishReason says how the model stopped. "length" means it ran out of room.
func finishReason(resp *openrouter.ChatResponse) string {
	if resp == nil || len(resp.Choices) == 0 {
		return ""
	}
	return resp.Choices[0].FinishReason
}

func completionTokens(resp *openrouter.ChatResponse) int {
	if resp == nil || resp.Usage == nil {
		return 0
	}
	return resp.Usage.CompletionTokens
}

func snippet(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func tailSnippet(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

func firstContent(resp *openrouter.ChatResponse) string {
	if resp == nil || len(resp.Choices) == 0 {
		return ""
	}
	return resp.Choices[0].Message.Content
}

// parseAuthoredIndex pulls the HTML document out of the model's answer.
//
// Models fence code in ```html blocks and add a sentence before it however
// firmly they are told not to, so the fence is stripped rather than treated
// as a failure — refusing a correct project over its wrapper would waste a
// frontier-model call and tell the user their design was bad.
//
// It takes from the first "<" to the last ">", which drops a preamble and a
// sign-off without needing to understand either.
func parseAuthoredIndex(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", fmt.Errorf("The model returned nothing.")
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
	start, end := strings.Index(s, "<"), strings.LastIndex(s, ">")
	if start < 0 || end <= start {
		return "", fmt.Errorf("The model's answer was not an HTML document.")
	}
	s = strings.TrimSpace(s[start : end+1])

	// A cheap shape check so the real validator's errors are about the film
	// rather than about having been handed prose that happened to contain a
	// bracket.
	if !strings.Contains(s, "data-composition-id") {
		return "", fmt.Errorf("The model's answer has no root composition.")
	}
	return s, nil
}

// hyperframesAuthoringSystemPrompt is the contract plus this design's canvas
// and the output shape.
//
// The contract itself lives in pkg/hfproject so the chat tool and this job
// cannot drift apart. The only thing added here is what differs per design.
func hyperframesAuthoringSystemPrompt(d *trendlymodels.Design, kit *brandKit) string {
	brand := ""
	if kit != nil {
		brand = kit.Prompt
	}
	return fmt.Sprintf(`You write Hyperframes projects for a social video tool.

%s

THIS FILM'S CANVAS: %dx%d at %d fps. Use exactly these numbers for data-width
and data-height on the root and on every scene.%s

ANSWER WITH THE COMPLETE index.html AND NOTHING ELSE — no JSON, no code
fence, no explanation. Start at <html and end at </html>.`,
		hfproject.AuthoringContract, d.Format.Width, d.Format.Height, d.Format.FPS, brand)
}
