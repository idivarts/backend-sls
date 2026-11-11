package studio

// Approving a plan turns it into scenes.
//
// ── What materialising means ────────────────────────────────────────────────
// Every scene the plan describes is written AT ONCE — with its beat as a title,
// its length, its transition, its narration and the brief for whatever has to
// be made. Scenes that need generating are marked `pending`, so the rail shows
// the whole shape of the video within a second of approval and then fills in
// visibly.
//
// Layers that REUSE something the brand already owns are filled immediately,
// because they cost nothing and are the planner's cheapest answer. That is also
// the point of reuse-before-spend being in the plan at all: an approved plan
// that reuses four library rows and generates one clip is four scenes that are
// already finished.
//
// ── What it does not do ────────────────────────────────────────────────────
// It does not run a graph. The per-scene generate endpoints fill the pending
// scenes, driven by the client, one at a time — the same choice as the motion
// generation in Phase 3 and for the same reasons: each request stays inside the
// Lambda budget, the rail fills visibly, and a user can stop after two scenes
// without paying for six.
//
// ⚠️ A PLAN WITH A FATAL PROBLEM CANNOT BE APPROVED HERE, whatever a client
// sends. The storyboard shows warnings and lets them be overridden; fatals are
// the gate, and the gate is at the API rather than only in the UI.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"cloud.google.com/go/firestore"
	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/internal/trendlyapis/ai/director"
)

// ApproveRun turns a stored plan into scenes.
// POST …/studios/:studioId/runs/:runId/approve
func ApproveRun(c *gin.Context) {
	brandID, contentID, studioID, ok := guard(c, true)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	runID := c.Param("runId")

	run, err := trendlymodels.GetStudioRun(ctx, brandID, contentID, studioID, runID)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not read the run"})
		return
	}
	if run == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no such run"})
		return
	}

	// The gate, at the API. A client that offered an Approve button on a
	// refused plan still cannot run it.
	for _, p := range run.Problems {
		if p.Fatal {
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"error":   "plan_not_runnable",
				"message": "This plan has a problem that has to be fixed first: " + p.Message,
			})
			return
		}
	}

	// Still being planned. A plan that does not exist yet cannot be approved,
	// and saying so beats letting it fall through to "that plan can't be read"
	// — which is the message for a CORRUPT plan and would send the user off to
	// re-plan something that is about to arrive on its own.
	//
	// Guarded at the API because the client cannot be relied on to hide the
	// button: `refused` there is computed from fatal problems, so a run with no
	// problems yet reads as approvable. Same principle as the fatal gate above.
	if run.Status == trendlymodels.RunPlanning {
		c.JSON(http.StatusConflict, gin.H{
			"error":   "plan_not_ready",
			"message": "The director is still working on this plan. Give it a moment.",
		})
		return
	}

	var plan director.Plan
	if uerr := json.Unmarshal([]byte(run.PlanJSON), &plan); uerr != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error":   "plan_unreadable",
			"message": "That plan can't be read any more. Plan it again.",
		})
		return
	}

	// Approved exactly once, in a transaction. Two taps on a slow button is
	// the ordinary way a run starts twice, and a run that starts twice
	// generates everything twice and charges for both.
	approved, err := trendlymodels.ApproveStudioRun(ctx, brandID, contentID, studioID, runID)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not approve the run"})
		return
	}
	if !approved {
		// Not an error: the second tap should simply do nothing, and the
		// client should show the scenes that already exist.
		c.JSON(http.StatusOK, gin.H{
			"status":  run.Status,
			"message": "This plan has already been approved.",
		})
		return
	}

	sceneIDs, problems := materialise(ctx, brandID, contentID, studioID, runID, plan)

	updates := []firestore.Update{
		{Path: "sceneIds", Value: sceneIDs},
		{Path: "status", Value: trendlymodels.RunRunning},
	}
	if len(problems) > 0 {
		// A partial materialisation is reported rather than hidden: the user
		// can see exactly how far it got, where "it worked" followed by three
		// missing scenes is a mystery.
		updates = append(updates, firestore.Update{Path: "error", Value: strings.Join(problems, " ")})
	}
	_ = trendlymodels.UpdateStudioRun(ctx, brandID, contentID, studioID, runID, updates)

	s, err := trendlymodels.GetStudio(ctx, brandID, contentID, studioID)
	if err != nil || s == nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not re-read the studio"})
		return
	}
	respondWithStudio(c, brandID, contentID, s)
}

// materialise writes the plan's cast, audio, settings and scenes.
//
// Returns the scene ids it created, in plan order, and anything that could not
// be written. It keeps going past a failed scene rather than stopping: five
// scenes and one gap is more useful than two scenes and an error, and the gap
// is visible on the rail.
func materialise(
	ctx context.Context,
	brandID, contentID, studioID string,
	// runID is stamped on every scene this plan creates, so the rail can draw
	// one run's scenes as one batch without loading the runs themselves.
	runID string,
	plan director.Plan,
) (sceneIDs []string, problems []string) {
	// ── Document level, first ──
	//
	// Before the scenes, because a scene that references a cast member whose
	// row does not exist yet cannot be checked — and the anchor rule counts
	// appearances across the studio.
	updates := []firestore.Update{}

	if len(plan.Cast) > 0 {
		cast := make([]trendlymodels.CastMember, 0, len(plan.Cast))
		for _, m := range plan.Cast {
			member := trendlymodels.CastMember{
				ID:          m.ID,
				Description: m.Description,
				// Verbatim. Paraphrasing the wardrobe between clips is how a
				// navy jumper becomes a blue shirt.
				Look: m.Look,
			}
			// ⚠️ A PLAN'S ANCHOR IS SOMETIMES ALREADY REAL. Two fields express
			// it: `anchorAssetId` is a still that has yet to be generated, and
			// cannot be carried — but `anchorMediaId` is a library row that
			// exists right now, and dropping it is not caution, it is data
			// loss. The cost is specific: `resolveCast` refuses a member above
			// two generated clips without an anchor, so a plan that correctly
			// chose an existing still had its third clip rejected by a rule
			// the plan had already satisfied.
			//
			// Checked against the library rather than trusted: an id the model
			// invented would otherwise become a reference URL lookup that
			// fails mid-generation, where here it is simply not an anchor.
			//
			// `noAiInput` is checked for the same reason. A still whose licence
			// forbids feeding it to a model is not an anchor, however real it
			// is — carrying it would satisfy the appearance count at approval
			// and then refuse at submission, which moves the complaint from
			// "this plan needs an anchor", said while the user is reading the
			// plan, to "that still can't be used", said minutes later.
			if m.AnchorMediaID != "" {
				if a, err := trendlymodels.GetAsset(ctx, brandID, m.AnchorMediaID); err == nil && a != nil && !a.NoAIInput {
					member.AnchorMediaID = m.AnchorMediaID
				}
			}
			cast = append(cast, member)
		}
		updates = append(updates, firestore.Update{Path: "cast", Value: cast})
	}

	if len(plan.Locations) > 0 {
		locs := make([]trendlymodels.StudioLocation, 0, len(plan.Locations))
		for _, l := range plan.Locations {
			loc := trendlymodels.StudioLocation{
				ID: l.ID, Description: l.Description, Look: l.Look,
			}
			// Same rule as the cast above: an anchor that already exists and
			// may be fed to a model is carried; an asset request is not.
			if l.AnchorMediaID != "" {
				if a, err := trendlymodels.GetAsset(ctx, brandID, l.AnchorMediaID); err == nil && a != nil && !a.NoAIInput {
					loc.AnchorMediaID = l.AnchorMediaID
				}
			}
			locs = append(locs, loc)
		}
		updates = append(updates, firestore.Update{Path: "locations", Value: locs})
	}

	// ── The plan's sound ──
	//
	// Written FIELD BY FIELD, never as a whole StudioAudio. Writing the object
	// replaced it: a studio approved onto after the user had added a bed or a
	// voiceover lost those lanes, silently, at the moment they said yes to
	// something unrelated. Approval may add to the sound; it may not reset it.
	if plan.Audio.VoiceID != "" {
		// One voice for the document. Two voices across a 30-second reel reads
		// as two videos spliced together.
		updates = append(updates, firestore.Update{Path: "audio.voiceId", Value: plan.Audio.VoiceID})
	}
	// The music is PARKED, not applied. Generating a bed is a metered call the
	// user has not made, and a studio may already have one — so the plan's
	// intent waits for a tap in the sound panel. Dropping it, which is what
	// happened before, lost the hardest part of the plan to re-describe: "32
	// seconds, builds at 0:18" is a judgement about the edit.
	if req := musicRequestFor(plan.Audio); req != nil {
		updates = append(updates, firestore.Update{Path: "audio.musicRequest", Value: req})
	}
	if plan.Canvas.Fps > 0 {
		updates = append(updates, firestore.Update{Path: "fps", Value: plan.Canvas.Fps})
	}

	if len(updates) > 0 {
		if err := trendlymodels.UpdateStudio(ctx, brandID, contentID, studioID, updates); err != nil {
			problems = append(problems, "The cast and sound settings didn't save.")
		}
	}

	// ── The scenes ──
	byRequest := map[string]director.AssetRequest{}
	for _, ar := range plan.AssetRequests {
		byRequest[ar.ID] = ar
	}

	for i, sp := range plan.Scenes {
		sc := &trendlymodels.StudioScene{
			Title:       strings.TrimSpace(sp.Beat),
			DurationMs:  sp.DurationMs,
			PrimaryKind: trendlymodels.SceneEmpty,
			Layers:      []trendlymodels.StudioLayer{},
		}

		// The incoming transition. The first scene always cuts: there is
		// nothing before it to dissolve from.
		if i > 0 && sp.TransitionIn.Kind != "" && sp.TransitionIn.Kind != trendlymodels.TransitionCut {
			if knownTransition(sp.TransitionIn.Kind) {
				ms := sp.TransitionIn.Ms
				if ms <= 0 {
					ms = 400
				}
				sc.TransitionIn = &trendlymodels.Transition{Kind: sp.TransitionIn.Kind, DurationMs: ms}
			}
		}

		if line := strings.TrimSpace(sp.Narration); line != "" {
			// Provenance travels from the plan onto the scene, which is what
			// makes the rule enforceable twice: the validator refused a plan
			// without it, and the scene now carries the answer for anyone
			// reading the timeline later.
			sc.Narration = &trendlymodels.SceneNarration{
				Line:           line,
				Source:         normaliseNarrationSource(sp.NarrationSource),
				BrandFactID:    sp.NarrationBrandFactID,
				FitToNarration: sp.FitToNarration,
			}
		}

		// Layers. A reuse is filled now; anything that has to be made leaves
		// the scene pending with the brief on it, so whatever submits it is
		// self-contained — it takes a scene id, not a plan.
		for li, lp := range sp.Layers {
			if lp.ReuseMediaID == "" {
				continue
			}
			sc.Layers = append(sc.Layers, trendlymodels.StudioLayer{
				ID:        fmt.Sprintf("l%d_%d", li, nowMs()),
				Kind:      "media",
				Slot:      orSlot(lp.Slot),
				MediaID:   lp.ReuseMediaID,
				Fit:       "cover",
				TrimInMs:  lp.TrimInMs,
				TrimOutMs: lp.TrimOutMs,
				Volume:    lp.Volume,
			})
		}

		if gen := genIntent(sp, byRequest); gen != nil {
			sc.Pending = true
			sc.Gen = gen
		}

		if len(sc.Layers) > 0 {
			sc.PrimaryKind = primaryKindFor(ctx, brandID, sc.Layers)
		}
		sc.RunID = runID

		id, err := trendlymodels.CreateStudioScene(ctx, brandID, contentID, studioID, sc)
		if err != nil {
			problems = append(problems,
				fmt.Sprintf("Scene %d (%s) didn't save.", i+1, sp.Beat))
			continue
		}
		sceneIDs = append(sceneIDs, id)
	}

	return sceneIDs, problems
}

// genIntent decides what a planned scene has to have made, or nil when nothing
// does. Everything a submission needs, read off the plan once.
//
// ⚠️ THE OVERLAY AND THE FOOTAGE ARE TWO BRIEFS, NOT ONE. This used to keep a
// single `brief` that each layer overwrote as the walk went on, so a video
// scene carrying type over it ended up with the model the plan chose for the
// footage and the words the plan wrote for the CAPTION: "generate with Veo",
// prompt "Editable bold top-third hook…". Nothing submitted it, which is the
// only reason that never became an 8-second video of a text box charged to a
// brand. They are kept apart and chosen between by the approach the plan
// declared.
//
// Pulled out of the materialisation loop so it can be tested without a
// Firestore client — the bug above was invisible in a 60-line loop and obvious
// the moment it had a name.
func genIntent(
	sp director.ScenePlan,
	byRequest map[string]director.AssetRequest,
) *trendlymodels.SceneGeneration {
	htmlBrief, assetBrief := "", ""
	var footage *director.AssetRequest
	needsMaking := false

	for _, lp := range sp.Layers {
		switch {
		case lp.ReuseMediaID != "":
			// Already on the scene; nothing to make.
		case lp.HTMLBrief != "":
			htmlBrief, needsMaking = lp.HTMLBrief, true
		case lp.AssetRequestID != "":
			ar, ok := byRequest[lp.AssetRequestID]
			if !ok {
				// A layer pointing at a request the plan does not contain.
				// The validator refuses that, so reaching here means a stored
				// plan from before the rule — skipped rather than turned into
				// a generation with an empty prompt.
				continue
			}
			assetBrief, needsMaking = promptFor(ar), true
			if ar.Kind == "video" {
				req := ar
				footage = &req
			}
		}
	}

	if !needsMaking {
		return nil
	}

	gen := &trendlymodels.SceneGeneration{
		Prompt: assetBrief,
		Status: trendlymodels.SceneGenQueued,
	}
	if gen.Prompt == "" {
		gen.Prompt = htmlBrief
	}
	model := ""
	if footage != nil {
		model = videoModelFor(sp, byRequest)
	}
	// ⚠️ NO MODEL, NO VIDEO INTENT. An approved plan cannot get here — the
	// validator makes an unenabled or missing model a fatal problem — but if
	// one ever did, marking the scene as a video generation with nothing to
	// submit it to would strand it at "Making…" with no job, which is the
	// precise failure this whole change exists to end. Falling through leaves
	// a pending scene with a prompt and no model, which the design fill can
	// pick up and a person can see.
	if sp.Approach != trendlymodels.SceneVideo || footage == nil || model == "" {
		return gen
	}

	// The whole generation intent, carried.
	//
	// Without it a submission has a model and a prompt and nothing else — no
	// cast, so no `look` appended and no anchor attached, which between them
	// are every rule that keeps one face across three clips. The plan knew all
	// of this; the scene simply dropped it on the floor.
	gen.Model = model
	gen.Prompt = promptFor(*footage)
	gen.AssetRequestID = footage.ID
	gen.CastIDs = footage.CastIDs
	gen.LocationID = footage.LocationID
	gen.DurationSecs = footage.DurationS
	gen.NativeAudio = footage.NativeAudio
	// Kept, not dropped: the plan wants type over the clip, and this is the
	// only record of what it should say.
	gen.OverlayBrief = htmlBrief
	return gen
}

// promptFor is what to actually send a model for a request: its prompt, or its
// intent when the planner gave no prompt. Intent is the fallback on purpose —
// it survives a model change, where a prompt is written for one model.
func promptFor(ar director.AssetRequest) string {
	if p := strings.TrimSpace(ar.Prompt); p != "" {
		return p
	}
	return strings.TrimSpace(ar.Intent)
}

// videoModelFor finds the model the plan chose for a scene's generation, so the
// client does not have to re-read the plan to ask for the right one.
func videoModelFor(sp director.ScenePlan, byRequest map[string]director.AssetRequest) string {
	for _, lp := range sp.Layers {
		if lp.AssetRequestID == "" {
			continue
		}
		if ar, ok := byRequest[lp.AssetRequestID]; ok && ar.Kind == "video" {
			return ar.Model
		}
	}
	return ""
}

// normaliseNarrationSource keeps only the five the scene model knows.
//
// The validator has already refused an unknown one, so this is the belt to that
// braces: a plan stored before the rule existed would otherwise materialise a
// scene with a source nothing recognises. `paraphrase` is the safe landing
// because it is the honest default — the user's material, reworded.
func normaliseNarrationSource(source string) string {
	switch source {
	case trendlymodels.NarrationFromScript, trendlymodels.NarrationFromCaption,
		trendlymodels.NarrationFromBrand, trendlymodels.NarrationParaphrase,
		trendlymodels.NarrationIllustrative:
		return source
	default:
		return trendlymodels.NarrationParaphrase
	}
}

// orSlot defaults a missing slot to the full frame, which is what one layer in
// a scene means.
func orSlot(slot string) string {
	if slot == "" || !knownSlot(slot) {
		return trendlymodels.SlotFull
	}
	return slot
}

// musicRequestFor turns a plan's music intent into a pending request, or nil
// when the plan asked for no music.
//
// A plan that names an approach but neither a prompt nor a library row has said
// nothing actionable, so it produces nothing: an empty offer in the sound panel
// is worse than no offer, because the user taps it and gets silence.
func musicRequestFor(a director.AudioPlan) *trendlymodels.MusicRequest {
	switch a.MusicApproach {
	case "generate":
		if strings.TrimSpace(a.MusicPrompt) == "" {
			return nil
		}
		return &trendlymodels.MusicRequest{
			Approach: "generate",
			ModelID:  a.MusicModel,
			Prompt:   strings.TrimSpace(a.MusicPrompt),
		}
	case "reuse":
		if strings.TrimSpace(a.MusicMediaID) == "" {
			return nil
		}
		return &trendlymodels.MusicRequest{
			Approach: "reuse",
			MediaID:  strings.TrimSpace(a.MusicMediaID),
		}
	default:
		// "none", or an approach the validator already refused.
		return nil
	}
}
