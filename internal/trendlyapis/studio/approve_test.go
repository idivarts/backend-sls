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
