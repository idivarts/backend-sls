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

	sceneIDs, problems := materialise(ctx, brandID, contentID, studioID, plan)

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
			cast = append(cast, trendlymodels.CastMember{
				ID:          m.ID,
				Description: m.Description,
				// Verbatim. Paraphrasing the wardrobe between clips is how a
				// navy jumper becomes a blue shirt.
				Look: m.Look,
				// The anchor is a MEDIA id, and the plan's anchor is an asset
				// request that has not run yet — so it is left empty and set
				// when the still lands. A plan cannot pre-fill a reference to
				// something that does not exist.
			})
		}
		updates = append(updates, firestore.Update{Path: "cast", Value: cast})
	}

	if len(plan.Locations) > 0 {
		locs := make([]trendlymodels.StudioLocation, 0, len(plan.Locations))
		for _, l := range plan.Locations {
			locs = append(locs, trendlymodels.StudioLocation{
				ID: l.ID, Description: l.Description, Look: l.Look,
			})
		}
		updates = append(updates, firestore.Update{Path: "locations", Value: locs})
	}

	if plan.Audio.VoiceID != "" {
		updates = append(updates, firestore.Update{
			Path: "audio",
			// One voice for the document. Two voices across a 30-second reel
			// reads as two videos spliced together.
			Value: &trendlymodels.StudioAudio{VoiceID: plan.Audio.VoiceID},
		})
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
		// the scene pending with the brief on it, so the per-scene endpoints
		// are self-contained — they take a scene id, not a plan.
		brief, needsMaking := "", false
		for li, lp := range sp.Layers {
			switch {
			case lp.ReuseMediaID != "":
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
			case lp.HTMLBrief != "":
				brief, needsMaking = lp.HTMLBrief, true
			case lp.AssetRequestID != "":
				if ar, ok := byRequest[lp.AssetRequestID]; ok {
					if ar.Prompt != "" {
						brief = ar.Prompt
					} else {
						brief = ar.Intent
					}
					needsMaking = true
				}
			}
		}

		if needsMaking {
			sc.Pending = true
			sc.Gen = &trendlymodels.SceneGeneration{
				Prompt: brief,
				Status: "queued",
			}
			// Which approach the planner chose, so the client asks the right
			// endpoint without re-reading the plan.
			if sp.Approach == trendlymodels.SceneVideo {
				sc.PrimaryKind = trendlymodels.SceneEmpty
				sc.Gen.Model = videoModelFor(sp, byRequest)
			}
		}
		if len(sc.Layers) > 0 {
			sc.PrimaryKind = primaryKindFor(ctx, brandID, sc.Layers)
		}

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
