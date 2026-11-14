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
	"github.com/idivarts/backend-sls/pkg/mediastore"
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

// ── How many times a film gets written ────────────────────────────────────
//
// A gate failure is not an opinion. "Contrast is 2.83:1; WCAG AA requires 3:1"
// and "two text blocks overlap" are measurements, and a model holding the
// measurement can fix the film — which is why the run does not stop at the
// first refusal and hand the user a design they cannot render.
//
// It stops at two: one pass, one repair.
//
// Not because a third would never help, but because of what an attempt costs.
// Each one lands in a fresh revision whose media is resolved from scratch — the
// model's document carries no data-media-resolved — so an attempt is not only
// another frontier-model call, it is another trip to the paid media APIs. The
// ceiling those calls share is divided by this number (see
// MediaCreditCeilingPerAttempt), which means raising it does not raise the
// spend; it shrinks what each attempt may buy. Two is where the repair is
// worth the most and the division costs the least.
const maxAuthorAttempts = 2

// authorLoopBudget is the wall clock the whole run may spend, first attempt
// included.
//
// Not a comfort margin. studio_sqs is capped at 900s and the queue's
// visibility timeout is 960s, so a run that overshoots does not merely take
// too long: SQS redelivers the message, the idempotency guard below sees
// "writing" rather than "queued", and the redelivery is dropped — leaving a
// design stuck mid-rewrite with nobody working on it. Twelve minutes leaves
// the Lambda three to finish whatever it is in the middle of.
const authorLoopBudget = 12 * time.Minute

// prepareWait is how long the loop waits for a preparation to report before it
// gives up on learning anything from it.
//
// Measured on dev: authoring lands around 45s, preparation around 70s. Four
// minutes is room for a cold Batch queue. Running out is not a failure — the
// preparation carries on in the worker and writes its own result — it only
// means this run does not get to rewrite the film.
const prepareWait = 4 * time.Minute

// maxGenerationTime caps one model call, and minGenerationTime is the least
// time a call is worth making with: below it the model would be cut off
// mid-document, which costs the tokens and produces nothing.
const (
	maxGenerationTime = 10 * time.Minute
	minGenerationTime = 2 * time.Minute
)

// minRetryBudget is the least time worth beginning another attempt with.
//
// ⚠️ Derived, not chosen, and that is the point. The first version of this was
// a flat four minutes — the same as prepareWait — so a rewrite could clear the
// check with exactly enough budget to WAIT and none left to write: the poll ate
// all four minutes, the budget went negative, and generationTimeout read a
// non-positive remainder as "no constraint" and handed the call the full ten
// minutes. Eight minutes of first attempt plus four of polling plus ten of
// rewriting is twenty-two, against a Lambda that stops at fifteen — and a
// run killed there leaves the design at "writing" with nobody working on it,
// because the redelivery's idempotency guard sees exactly that and skips.
//
// So it is the wait, plus a call worth making, plus a minute for the upload and
// the dispatch between them.
const minRetryBudget = prepareWait + minGenerationTime + time.Minute

// ExecuteDesignAuthorJob is the worker side. Called from studio_sqs.
//
// ── Why this writes the film more than once ───────────────────────────────
// Authoring produces a document; Hyperframes' `check` decides whether it can
// become a film. Roughly half the designs written against a real brief were
// coming back refused — a contrast ratio a few tenths under the bar, two text
// blocks overlapping — and the user was handed a finished-looking design with
// an error under it and nothing to do but press the button again.
//
// So the job reads the refusal and writes the film again with the findings in
// hand. The alternative was to weaken the gate, and 3:1 on a phone in daylight
// is not a bar worth lowering: a film nobody can read is worse than a film
// that took a second pass.
func ExecuteDesignAuthorJob(ctx context.Context, msg studiojobs.Message) error {
	brandID, designID := msg.BrandID, msg.DesignID
	started := time.Now()

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

	orgID, _ := trendlymodels.OrgIDForBrand(brandID)
	// Read once, for the whole run, and divided across the attempts it may
	// make. A design that has to be written twice must not be allowed to buy
	// twice as much media as one that got it right first time.
	mediaCeiling := MediaCreditCeilingPerAttempt(orgID, maxAuthorAttempts)
	run := &authorRun{
		brandID:  brandID,
		designID: designID,
		design:   d,
		model:    model,
		kit:      kit,
		brief:    msg.Brief,
	}

	for attempt := 1; ; attempt++ {
		run.attempt = attempt
		run.remaining = authorLoopBudget - time.Since(started)

		written, werr := run.write(ctx)
		if werr != nil || written == "" {
			// Either an error the queue should retry, or an attempt that
			// failed for a reason now written on the design. Neither leaves
			// anything to rewrite from.
			return werr
		}

		// ── Finish the film without being asked ──────────────────────────
		// Authoring produces a document that DECLARES its media; it does not
		// have any. Resolving it needs the Hyperframes CLI, which lives in
		// the worker, so until this ran automatically the user had to press
		// "Preview — free" to make the photograph exist — and a film they
		// opened before pressing it was missing the very thing the brief
		// asked for.
		//
		// It is also what produces the gate's verdict, which is what the
		// loop below is waiting for.
		if perr := markAndPrepare(ctx, brandID, designID, mediaCeiling); perr != nil {
			log.Printf("design %s/%s: could not start preparation: %v", brandID, designID, perr)
			mysentry.Capture(perr, map[string]string{"op": "design.author.prepare", "brandId": brandID, "designId": designID})
			// The status was set to queued before dispatch so the app shows
			// the job immediately. A dispatch that then fails must not leave
			// it there: a spinner for a job nobody will run reads as slowness
			// rather than breakage, and the design never resolves. The same
			// rule the render handler follows, for the same reason.
			if merr := trendlymodels.MarkBrandDesignPreviewFailed(ctx, brandID, designID,
				"We wrote the design but could not start finishing it. Try preparing it again."); merr != nil {
				log.Printf("design %s/%s: could not clear the stranded preparation: %v", brandID, designID, merr)
			}
			return nil
		}

		// ── Is another pass possible at all? ─────────────────────────────
		// Asked BEFORE waiting on the preparation, because the wait is only
		// worth four minutes if its answer can be used. On the last attempt
		// the preparation carries on in the worker and writes its own
		// result; this job has nothing left to add.
		if attempt >= maxAuthorAttempts {
			return nil
		}
		if left := authorLoopBudget - time.Since(started); left < minRetryBudget {
			log.Printf("design %s/%s: %s left of the budget, not enough to rewrite", brandID, designID, left.Round(time.Second))
			return nil
		}

		findings := awaitGateFindings(ctx, brandID, designID, prepareWait)
		if len(findings) == 0 {
			return nil
		}

		// A rewrite is another frontier-model call on the org's wallet. The
		// request gated the first one; nothing has gated this one, and a
		// wallet emptied by the attempt that just ran must not be overdrawn
		// by its retry. The design keeps the gate's own error, which is the
		// honest thing to show: the film has a problem, and we stopped.
		if trendlymodels.TokensExhausted(orgID) {
			log.Printf("design %s/%s: the gate refused attempt %d but the wallet is empty; not rewriting", brandID, designID, attempt)
			return nil
		}

		log.Printf("design %s/%s: the gate refused attempt %d (%d finding(s)); rewriting", brandID, designID, attempt, len(findings))
		run.fix, run.previous = findings, written
		if merr := trendlymodels.MarkBrandDesignRewriting(ctx, brandID, designID, attempt+1); merr != nil {
			// Without this write the app would show a failed preparation
			// while a rewrite is in flight, and the rewrite would then
			// overwrite a design the user has been told is broken. Stopping
			// leaves them the gate's error, which is at least true.
			log.Printf("design %s/%s: could not mark the rewrite: %v", brandID, designID, merr)
			mysentry.Capture(merr, map[string]string{"op": "design.author.rewrite", "brandId": brandID, "designId": designID})
			return nil
		}
	}
}

// authorRun is one authoring job's worth of state: what does not change
// between attempts, and what the last attempt learned.
//
// A struct rather than a dozen parameters. The alternative was a function
// taking brand, design, document, model, kit, brief, findings, previous
// document, attempt number and remaining budget — ten arguments of which six
// are strings, which is a call nobody can read and a signature that silently
// accepts them in the wrong order.
type authorRun struct {
	brandID, designID string
	design            *trendlymodels.Design
	model             string
	kit               *brandKit
	brief             string

	// attempt counts from 1. fix and previous are empty on the first attempt
	// and carry the last one's findings and document on a rewrite.
	attempt  int
	fix      []string
	previous string

	// remaining is what is left of authorLoopBudget as this attempt starts.
	remaining time.Duration
}

// write runs ONE attempt: the model call, the validation, the upload and the
// revision. It returns the document the model produced.
//
// ⚠️ An empty document with a nil error means the attempt failed AND the
// design already carries the sentence saying why. The caller stops; it does
// not retry, and the job is over. A non-nil error is for the QUEUE — something
// went wrong that a redelivery could fix.
//
// The document returned is the model's own output, before the brand kit's CSS
// is injected into it. That is deliberate: it is exactly what the model sees
// on a rewrite, so feeding it back cannot inject the same token block twice.
func (a *authorRun) write(ctx context.Context) (string, error) {
	brandID, designID := a.brandID, a.designID

	genCtx, cancel := context.WithTimeout(ctx, a.generationTimeout())
	defer cancel()

	resp, err := openrouter.ChatCompletion(genCtx, openrouter.ChatRequest{
		Model: a.model,
		Messages: []openrouter.Message{
			{Role: "system", Content: hyperframesAuthoringSystemPrompt(a.design, a.kit)},
			{Role: "user", Content: a.userMessage()},
		},
	})
	if err != nil {
		reason := "The model did not answer."
		if errors.Is(err, context.DeadlineExceeded) {
			reason = "Writing this design took too long. Try again, or shorten the brief."
		}
		return "", trendlymodels.MarkBrandDesignAuthorFailed(ctx, brandID, designID, reason)
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
		log.Printf("design author truncated for %s/%s on attempt %d: %d completion tokens, %d chars",
			brandID, designID, a.attempt, completionTokens(resp), len(raw))
		return "", trendlymodels.MarkBrandDesignAuthorFailed(ctx, brandID, designID,
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
		return "", trendlymodels.MarkBrandDesignAuthorFailed(ctx, brandID, designID, perr.Error())
	}

	// The brand's tokens go in whatever the model did, and its assets travel
	// with the project so a render needs no network. Both are no-ops for a
	// brand with no design system.
	files := []hfproject.File{{Path: "index.html", Content: indexHTML}}
	if a.kit != nil {
		files[0].Content = hfproject.InjectIntoHead(indexHTML, a.kit.CSS)
		files = append(files, a.kit.Files...)
	}

	project, berr := hfproject.Build(files)
	if berr != nil {
		// The validator's messages are already written to be read by a
		// person as well as a model, so they are surfaced rather than
		// replaced with something vaguer.
		return "", trendlymodels.MarkBrandDesignAuthorFailed(ctx, brandID, designID, berr.Error())
	}

	// ── Every write is a revision ────────────────────────────────────────
	// The number is reserved before the bytes go up, so two writers racing
	// cannot land in the same directory. A reservation that then fails is a
	// skipped number and nothing worse.
	rev, rerr := trendlymodels.AllocateDesignRevision(ctx, brandID, designID)
	if rerr != nil {
		mysentry.Capture(rerr, map[string]string{"op": "design.author.revision", "brandId": brandID, "designId": designID})
		return "", trendlymodels.MarkBrandDesignAuthorFailed(ctx, brandID, designID,
			"We wrote the design but could not save it. Please try again.")
	}

	prefix, uerr := hfproject.Upload(ctx, brandID, designID, rev, project)
	if uerr != nil {
		mysentry.Capture(uerr, map[string]string{"op": "design.author.upload", "brandId": brandID, "designId": designID})
		return "", trendlymodels.MarkBrandDesignAuthorFailed(ctx, brandID, designID,
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
	previewURL, perr2 := hfproject.UploadPlayer(ctx, brandID, designID, rev, project)
	if perr2 != nil {
		log.Printf("design player upload failed for %s/%s: %v", brandID, designID, perr2)
		mysentry.Capture(perr2, map[string]string{"op": "design.author.player", "brandId": brandID, "designId": designID})
	}

	if err := trendlymodels.SetBrandDesignProject(ctx, brandID, designID, prefix, project.DurationMs, scenes, previewURL); err != nil {
		return "", err
	}

	// The pointer and the list, after the files are up. Best-effort in the
	// same sense the player is: a design whose revision could not be
	// recorded still plays and still renders — it has simply lost its way
	// back, which is worth a line in Sentry and not worth failing a run the
	// user has already paid for.
	pruned, cerr := trendlymodels.CommitDesignRevision(ctx, brandID, designID, trendlymodels.DesignRevision{
		N:          rev,
		Prefix:     prefix,
		PreviewURL: previewURL,
		By:         trendlymodels.DesignRevisionAuthored,
		Note:       a.revisionNote(),
		DurationMs: project.DurationMs,
		Scenes:     scenes,
	})
	if cerr != nil {
		log.Printf("design revision commit failed for %s/%s: %v", brandID, designID, cerr)
		mysentry.Capture(cerr, map[string]string{"op": "design.author.commitRevision", "brandId": brandID, "designId": designID})
	}
	dropRevisionFiles(ctx, pruned)

	return indexHTML, nil
}

// generationTimeout caps one model call at ten minutes, or at whatever is left
// of the run's budget when that is less.
//
// The clamp is what keeps the loop inside the Lambda: a call that overshoots
// the 900s cap is killed mid-write, taking the tokens and leaving the design
// at "writing" for ever.
//
// ⚠️ A spent budget is floored, NOT treated as unlimited. Reading a
// non-positive remainder as "no constraint" is the bug this comment exists for:
// it turned the one case that most needed a short call into the one case that
// got the longest. The floor cannot be reached through the loop — a retry is
// only begun with minRetryBudget in hand — so this is the belt to that
// braces, and it fails small rather than large.
func (a *authorRun) generationTimeout() time.Duration {
	if a.remaining >= maxGenerationTime {
		return maxGenerationTime
	}
	if a.remaining <= minGenerationTime {
		return minGenerationTime
	}
	return a.remaining
}

// revisionNote is the one line the revisions list shows. A rewrite says so,
// because a list of three versions all labelled with the same brief tells the
// user nothing about why there are three.
func (a *authorRun) revisionNote() string {
	note := revisionNote(a.brief)
	if a.attempt > 1 {
		return fmt.Sprintf("Fix %d · %s", a.attempt-1, note)
	}
	return note
}

// userMessage is the brief on the first attempt, and the brief plus the film
// that failed plus what the gate said on a rewrite.
//
// ── Why the document goes back in ─────────────────────────────────────────
// A rewrite from the brief alone is a fresh roll of the dice: it discards a
// film that was wrong in one measurable way and may come back wrong in a
// different one. Handing the model its own document and the findings makes
// this a repair — the scenes, the timings, the words and the media intents all
// survive, and only the thing the gate measured changes.
//
// All the findings, not the first. A rewrite told about one defect fixes one
// defect and fails on the next, spending a frontier-model call per finding.
func (a *authorRun) userMessage() string {
	if len(a.fix) == 0 || a.previous == "" {
		return a.brief
	}
	var b strings.Builder
	b.WriteString(a.brief)
	b.WriteString("\n\n──────────\n\nTHIS IS A REWRITE. You wrote the document at the end of this\n")
	b.WriteString("message, and Hyperframes' check refused it:\n\n")
	for i, f := range a.fix {
		fmt.Fprintf(&b, "  %d. %s\n", i+1, f)
	}
	b.WriteString("\nEvery line above is a measurement, not an opinion. The same check runs\n")
	b.WriteString("again on what you return, and the film cannot render until all of them\n")
	b.WriteString("pass.\n\n")
	b.WriteString("Keep the film it already is: the same scenes, the same ids, the same\n")
	b.WriteString("timings, the same words, the same data-media-intent values. Change only\n")
	b.WriteString("what these findings require.\n\n")
	b.WriteString("Answer with the complete corrected index.html and nothing else.\n\n")
	b.WriteString("── THE DOCUMENT THAT FAILED ──\n")
	b.WriteString(a.previous)
	return b.String()
}

// awaitGateFindings waits for the preparation to report, and returns what the
// gate measured when it refused the film.
//
// ── Why this polls ────────────────────────────────────────────────────────
// The preparation runs in the render worker, which writes its outcome to the
// design document and has no route back into this process. A push would mean
// an endpoint, a credential and a retry policy for a signal this job is
// already sitting still for. The document is the channel both sides already
// share.
//
// nil covers every outcome that is not "the gate refused the film": it passed,
// it broke on infrastructure, the design was deleted, or nothing reported in
// time. All of them end the run, and most of them end it well.
func awaitGateFindings(ctx context.Context, brandID, designID string, within time.Duration) []string {
	const poll = 5 * time.Second
	deadline := time.Now().Add(within)

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(poll):
		}

		d, err := trendlymodels.GetBrandDesign(ctx, brandID, designID)
		switch {
		case err != nil:
			// A read that failed is not an answer. Keep waiting — the
			// deadline below is what ends this, not one bad read.
			log.Printf("design %s/%s: could not read the preparation: %v", brandID, designID, err)
		case d == nil:
			// Deleted while we waited.
			return nil
		case d.PreviewStatus == trendlymodels.DesignPreviewDone:
			return nil
		case d.PreviewStatus == trendlymodels.DesignPreviewFailed:
			if d.PreviewFailure != trendlymodels.DesignPreviewFailureCheck {
				// Infrastructure, not the film. Rewriting it would spend the
				// org's tokens on a fault the film did not cause.
				log.Printf("design %s/%s: preparation failed without the gate (%q); not rewriting",
					brandID, designID, d.PreviewError)
				return nil
			}
			if len(d.PreviewFindings) == 0 {
				// Marked as the gate's, carrying nothing to aim at. A
				// rewrite with no findings is the dice roll this exists to avoid.
				log.Printf("design %s/%s: the gate refused it with no findings; not rewriting", brandID, designID)
				return nil
			}
			return d.PreviewFindings
		}

		if time.Now().After(deadline) {
			log.Printf("design %s/%s: the preparation had not reported after %s; not rewriting",
				brandID, designID, within)
			return nil
		}
	}
}

// markAndPrepare queues the work that turns an authored document into a
// finished film: media resolved, gate passed, stills captured.
//
// Queued before dispatch for the same reason the handler does it: the app
// should show the job the instant the design appears, rather than a film
// that looks done and is missing its photograph.
// revisionNote is the one line the revisions list shows for an authoring
// run: the brief, shortened, because what a person recognises a version by
// is what they asked for.
func revisionNote(brief string) string {
	brief = strings.Join(strings.Fields(brief), " ")
	const max = 90
	if len(brief) > max {
		return brief[:max-1] + "…"
	}
	return brief
}

// dropRevisionFiles deletes the files of revisions the retention cap pushed
// out of the list.
//
// Best-effort and never fatal: bytes left behind cost a few hundred
// kilobytes, while a failed delete that fails the write would cost the user
// the design they just made. Each failure is logged so a prefix that never
// goes away is visible rather than merely absent.
func dropRevisionFiles(ctx context.Context, prefixes []string) {
	for _, p := range prefixes {
		if p == "" {
			continue
		}
		if n, err := mediastore.DeletePrefix(ctx, mediastore.TargetAttachment, p); err != nil {
			log.Printf("design revision cleanup failed for %s: %v", p, err)
			mysentry.Capture(err, map[string]string{"op": "design.revision.cleanup", "prefix": p})
		} else {
			log.Printf("design revision cleanup: removed %d file(s) under %s", n, p)
		}
	}
}

func markAndPrepare(ctx context.Context, brandID, designID string, creditCeiling int) error {
	if err := trendlymodels.MarkBrandDesignPreviewQueued(ctx, brandID, designID); err != nil {
		return err
	}
	// The ceiling is passed IN rather than read here, because what one
	// preparation may spend depends on how many the caller is going to make.
	// The user pressing "prepare" makes one and gets the whole cap; the
	// authoring job may make two and divides it.
	return rendering.SubmitDesignV3Preview(brandID, designID, creditCeiling)
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
