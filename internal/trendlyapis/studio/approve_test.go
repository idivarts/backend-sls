package studio

import (
	"testing"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/internal/trendlyapis/ai/director"
)

// Provenance has to survive the trip from plan to scene, or the rule is only
// enforced at planning time and the timeline forgets where its words came from.
func TestNarrationSourceSurvivesMaterialisation(t *testing.T) {
	for _, source := range []string{"script", "caption", "brand", "paraphrase", "illustrative"} {
		if got := normaliseNarrationSource(source); got != source {
			t.Errorf("source %q became %q", source, got)
		}
	}
}

// A plan stored before the rule existed would otherwise materialise a scene
// with a source nothing recognises. `paraphrase` is the safe landing because it
// is the honest default: the user's material, reworded.
func TestAnUnknownSourceLandsOnParaphrase(t *testing.T) {
	for _, source := range []string{"", "vibes", "SCRIPT"} {
		if got := normaliseNarrationSource(source); got != trendlymodels.NarrationParaphrase {
			t.Errorf("source %q should land on paraphrase, got %q", source, got)
		}
	}
}

// One layer in a scene means the full frame. A plan that omitted the slot, or
// named one the studio does not have, must not produce an invisible layer.
func TestAMissingOrUnknownSlotBecomesFullFrame(t *testing.T) {
	for _, slot := range []string{"", "middle-ish", "centre"} {
		if got := orSlot(slot); got != trendlymodels.SlotFull {
			t.Errorf("slot %q should default to full, got %q", slot, got)
		}
	}
	if got := orSlot(trendlymodels.SlotPipBR); got != trendlymodels.SlotPipBR {
		t.Errorf("a real slot should be kept, got %q", got)
	}
}

// The client asks the right endpoint from the scene alone, so the model the
// planner chose has to be carried onto it — otherwise approving a plan means
// re-reading it to find out what to generate with.
func TestTheChosenVideoModelIsCarriedOntoTheScene(t *testing.T) {
	requests := map[string]director.AssetRequest{
		"a_clip":  {ID: "a_clip", Kind: "video", Model: "veo-3.1-fast-generate-001"},
		"a_still": {ID: "a_still", Kind: "image", Model: "some-image-model"},
	}
	scene := director.ScenePlan{
		Layers: []director.LayerPlan{
			{Kind: "media", Slot: "full", AssetRequestID: "a_still"},
			{Kind: "media", Slot: "full", AssetRequestID: "a_clip"},
		},
	}

	if got := videoModelFor(scene, requests); got != "veo-3.1-fast-generate-001" {
		t.Errorf("model = %q, want the video request's model", got)
	}

	// A scene with no video request has no video model, and must not borrow
	// the image one — asking a still model for a clip is a wasted call.
	stillOnly := director.ScenePlan{
		Layers: []director.LayerPlan{{Kind: "media", Slot: "full", AssetRequestID: "a_still"}},
	}
	if got := videoModelFor(stillOnly, requests); got != "" {
		t.Errorf("model = %q, want empty for a scene with no video request", got)
	}
}

// The plan's music was dropped on the floor at approval. These lock in the two
// halves of the fix: an actionable intent survives, and a hollow one does not
// become an offer the user can tap for silence.
func TestMusicRequestSurvivesApproval(t *testing.T) {
	req := musicRequestFor(director.AudioPlan{
		MusicApproach: "generate",
		MusicModel:    "music_v2",
		MusicPrompt:   "  Warm lo-fi, 32 seconds, builds at 0:18  ",
	})
	if req == nil {
		t.Fatal("a generate approach with a prompt should park a request")
	}
	if req.Approach != "generate" || req.ModelID != "music_v2" {
		t.Errorf("approach/model not carried: %+v", req)
	}
	// Trimmed, because the prompt is shown to the user as the offer's body.
	if req.Prompt != "Warm lo-fi, 32 seconds, builds at 0:18" {
		t.Errorf("prompt not trimmed: %q", req.Prompt)
	}

	reuse := musicRequestFor(director.AudioPlan{MusicApproach: "reuse", MusicMediaID: "med_1"})
	if reuse == nil || reuse.Approach != "reuse" || reuse.MediaID != "med_1" {
		t.Errorf("reuse not carried: %+v", reuse)
	}
}

// An empty offer is worse than no offer: the user taps it and gets silence.
func TestAnEmptyMusicIntentIsNotAnOffer(t *testing.T) {
	cases := []director.AudioPlan{
		{MusicApproach: "none"},
		{},
		{MusicApproach: "generate"}, // no prompt
		{MusicApproach: "generate", MusicPrompt: " "},
		{MusicApproach: "reuse"},                   // no library row
		{MusicApproach: "vibes", MusicPrompt: "x"}, // the validator already refused this
	}
	for i, a := range cases {
		if got := musicRequestFor(a); got != nil {
			t.Errorf("case %d should park nothing, got %+v", i, got)
		}
	}
}

// ── What a video scene carries off the plan ────────────────────────────────
//
// These lock in the shape of the bug found on dev run LM5bL96dIRTP81YE9YNE:
// six scenes marked "generate with Veo", each carrying a prompt that described
// a text overlay, with no cast, no location and no duration. Nothing submitted
// them, so the cost was six scenes stuck at "Making…" rather than six videos
// of a headline card — but the next change in this file is the one that
// submits, and then it would have been the second.

func videoPlan() (director.ScenePlan, map[string]director.AssetRequest) {
	sp := director.ScenePlan{
		Beat:     "chaos",
		Approach: trendlymodels.SceneVideo,
		Layers: []director.LayerPlan{
			// Order matters to the bug: the overlay comes SECOND, and used to
			// win because the walk kept one brief.
			{AssetRequestID: "a_vid_chaos"},
			{HTMLBrief: "Editable bold top-third hook. Text: “2 hours/day”."},
		},
	}
	byRequest := map[string]director.AssetRequest{
		"a_vid_chaos": {
			ID:          "a_vid_chaos",
			Kind:        "video",
			Model:       "veo-3.1-fast-generate-001",
			Prompt:      "Faceless creator at a dawn desk, over-the-shoulder, pastel illustration.",
			DurationS:   8,
			CastIDs:     []string{"c_faceless_creator"},
			LocationID:  "l_creator_desk",
			NativeAudio: false,
		},
	}
	return sp, byRequest
}

func TestAVideoScenePromptIsTheFootageNotTheOverlay(t *testing.T) {
	gen := genIntent(videoPlan())
	if gen == nil {
		t.Fatal("a scene with a video request has something to make")
	}
	if gen.Prompt != "Faceless creator at a dawn desk, over-the-shoulder, pastel illustration." {
		t.Errorf("the video model must be sent the FOOTAGE prompt, got %q", gen.Prompt)
	}
	if gen.Model != "veo-3.1-fast-generate-001" {
		t.Errorf("model = %q", gen.Model)
	}
}

func TestTheOverlayIsKeptRatherThanDropped(t *testing.T) {
	// Fixing the collision must not lose the caption: the plan wants type over
	// the clip, and this is the only record of what it says.
	gen := genIntent(videoPlan())
	if gen.OverlayBrief == "" {
		t.Fatal("the overlay brief should survive beside the footage prompt")
	}
	if gen.OverlayBrief == gen.Prompt {
		t.Error("the overlay and the footage are two briefs, not one")
	}
}

func TestTheGenerationIntentIsCarriedWhole(t *testing.T) {
	// Each of these is a rule that silently stops applying when the field is
	// missing — the cast most of all: no castIds means no `look` appended and
	// no anchor attached, so three clips of one person become three people.
	gen := genIntent(videoPlan())
	if gen.AssetRequestID != "a_vid_chaos" {
		t.Errorf("assetRequestId = %q — without it, scenes sharing one generation cannot be grouped", gen.AssetRequestID)
	}
	if len(gen.CastIDs) != 1 || gen.CastIDs[0] != "c_faceless_creator" {
		t.Errorf("castIds = %v", gen.CastIDs)
	}
	if gen.LocationID != "l_creator_desk" {
		t.Errorf("locationId = %q", gen.LocationID)
	}
	if gen.DurationSecs != 8 {
		t.Errorf("durationSecs = %d — the CLIP's length, not the scene's", gen.DurationSecs)
	}
	if gen.Status != trendlymodels.SceneGenQueued {
		t.Errorf("status = %q", gen.Status)
	}
}

func TestTwoScenesSharingOneRequestSaySo(t *testing.T) {
	// The planner serves two 4-second scenes off one 8-second generation when
	// they share a cast member, a place and a framing. Submitting per scene
	// would generate both and bill twice, so the shared id has to reach the
	// scene for anything downstream to group by it.
	sp, byRequest := videoPlan()
	a := genIntent(sp, byRequest)
	b := genIntent(sp, byRequest)
	if a.AssetRequestID != b.AssetRequestID || a.AssetRequestID == "" {
		t.Errorf("two scenes off one request must carry the same id, got %q and %q",
			a.AssetRequestID, b.AssetRequestID)
	}
}

func TestADesignSceneStillGetsItsOwnBrief(t *testing.T) {
	// The regression that matters: an economy plan is all HTML scenes and no
	// video at all, and it has to behave exactly as it did before.
	sp := director.ScenePlan{
		Approach: trendlymodels.SceneMotion,
		Layers:   []director.LayerPlan{{HTMLBrief: "A kinetic list of five words."}},
	}
	gen := genIntent(sp, map[string]director.AssetRequest{})
	if gen == nil {
		t.Fatal("an html scene has something to make")
	}
	if gen.Prompt != "A kinetic list of five words." {
		t.Errorf("prompt = %q", gen.Prompt)
	}
	if gen.Model != "" {
		t.Errorf("a motion scene gets no video model, got %q", gen.Model)
	}
	if gen.AssetRequestID != "" || len(gen.CastIDs) != 0 {
		t.Error("a motion scene carries no footage intent")
	}
}

func TestASceneWithNothingToMakeIsNotPending(t *testing.T) {
	// A scene built entirely from reuse. Marking it pending would strand it at
	// "Making…" with nothing to make — the exact failure shape this is fixing.
	sp := director.ScenePlan{
		Approach: trendlymodels.SceneVideo,
		Layers:   []director.LayerPlan{{ReuseMediaID: "m_existing"}},
	}
	if gen := genIntent(sp, map[string]director.AssetRequest{}); gen != nil {
		t.Errorf("a reuse-only scene has nothing to make, got %+v", gen)
	}
}

func TestAnIntentFallsBackToIntentWhenThePlanGaveNoPrompt(t *testing.T) {
	sp, byRequest := videoPlan()
	ar := byRequest["a_vid_chaos"]
	ar.Prompt = ""
	ar.Intent = "person laughing, mid-shot, warm light"
	byRequest["a_vid_chaos"] = ar

	if got := genIntent(sp, byRequest).Prompt; got != "person laughing, mid-shot, warm light" {
		t.Errorf("prompt = %q, want the intent as the fallback", got)
	}
}

func TestALayerPointingAtAMissingRequestIsSkipped(t *testing.T) {
	// A plan stored before the validator enforced this. Better to make nothing
	// than to submit a generation with an empty prompt.
	sp := director.ScenePlan{
		Approach: trendlymodels.SceneVideo,
		Layers:   []director.LayerPlan{{AssetRequestID: "a_gone"}},
	}
	if gen := genIntent(sp, map[string]director.AssetRequest{}); gen != nil {
		t.Errorf("a dangling request should make nothing, got %+v", gen)
	}
}

func TestAVideoSceneWithNoModelDoesNotClaimToBeOne(t *testing.T) {
	// An approved plan cannot reach this — the validator makes a missing or
	// unenabled model fatal. But if one did, carrying a video intent with
	// nothing to submit it to would mark the scene "Making…" with no job
	// behind it, which is the exact failure this change exists to end.
	sp, byRequest := videoPlan()
	ar := byRequest["a_vid_chaos"]
	ar.Model = ""
	byRequest["a_vid_chaos"] = ar

	gen := genIntent(sp, byRequest)
	if gen == nil {
		t.Fatal("there is still something to make")
	}
	if gen.AssetRequestID != "" || len(gen.CastIDs) != 0 || gen.DurationSecs != 0 {
		t.Errorf("no model means no video intent, got %+v", gen)
	}
}
