package studio

import (
	"testing"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
)

func founder(anchor string) trendlymodels.CastMember {
	return trendlymodels.CastMember{
		ID:            "c_founder",
		Description:   "The founder",
		Look:          "mid-thirties, short dark hair, navy jumper",
		AnchorMediaID: anchor,
	}
}

func generatedScene(id string, castIDs ...string) trendlymodels.StudioScene {
	return trendlymodels.StudioScene{
		ID:          id,
		PrimaryKind: trendlymodels.SceneVideo,
		Gen:         &trendlymodels.SceneGeneration{CastIDs: castIDs, Model: "veo"},
	}
}

// Two appearances is the tolerance: a before and an after, where a slight
// difference in the face reads as time passing rather than as a different
// person. So two must be allowed without an anchor.
func TestTwoAppearancesNeedNoAnchor(t *testing.T) {
	s := &trendlymodels.Studio{Cast: []trendlymodels.CastMember{founder("")}}
	scenes := []trendlymodels.StudioScene{
		generatedScene("s1", "c_founder"),
		generatedScene("s2", "c_founder"),
	}

	// Generating s2 again: s2 itself plus s1 is two appearances.
	cast, refusal := resolveCast(s, []string{"c_founder"}, "s2", scenes)
	if refusal != nil {
		t.Fatalf("two appearances should be allowed without an anchor, got: %v", refusal.Body)
	}
	if len(cast) != 1 {
		t.Fatalf("expected the founder back, got %d members", len(cast))
	}
}

// By the third generated clip the face reads as a different person, and no
// prompt wording fixes it — so it is refused, with the reason and the fix.
func TestThirdAppearanceRequiresAnAnchor(t *testing.T) {
	s := &trendlymodels.Studio{Cast: []trendlymodels.CastMember{founder("")}}
	scenes := []trendlymodels.StudioScene{
		generatedScene("s1", "c_founder"),
		generatedScene("s2", "c_founder"),
		generatedScene("s3", "c_founder"),
	}

	_, refusal := resolveCast(s, []string{"c_founder"}, "s3", scenes)
	if refusal == nil {
		t.Fatal("a third generated clip without an anchor should be refused")
	}
	if refusal.Body["error"] != "anchor_required" {
		t.Errorf("error = %v, want anchor_required", refusal.Body["error"])
	}
	// The refusal has to carry WHICH cast member and HOW MANY clips, or the UI
	// can only say "something needs an anchor".
	if refusal.Body["castId"] != "c_founder" {
		t.Errorf("castId = %v, want c_founder", refusal.Body["castId"])
	}
	if refusal.Body["appearances"] != 3 {
		t.Errorf("appearances = %v, want 3", refusal.Body["appearances"])
	}
}

// With an anchor, any number of clips is fine — that is the entire point of
// having one.
func TestAnAnchorLiftsTheLimit(t *testing.T) {
	s := &trendlymodels.Studio{Cast: []trendlymodels.CastMember{founder("media_anchor")}}
	scenes := []trendlymodels.StudioScene{
		generatedScene("s1", "c_founder"),
		generatedScene("s2", "c_founder"),
		generatedScene("s3", "c_founder"),
		generatedScene("s4", "c_founder"),
	}

	cast, refusal := resolveCast(s, []string{"c_founder"}, "s4", scenes)
	if refusal != nil {
		t.Fatalf("an anchored cast member should pass at any count, got: %v", refusal.Body)
	}
	if cast[0].AnchorMediaID != "media_anchor" {
		t.Errorf("the anchor should come back for use as a reference image")
	}
}

// Appearances are counted ACROSS the studio, not within one request. The drift
// happens between separate generations, so a scene that is the person's third
// clip is refused even when the request names them once.
func TestAppearancesAreCountedAcrossTheStudio(t *testing.T) {
	s := &trendlymodels.Studio{Cast: []trendlymodels.CastMember{founder("")}}
	// Scenes that are NOT generated must not count: a person appearing in
	// uploaded footage has a real face already, and nothing is drifting.
	scenes := []trendlymodels.StudioScene{
		generatedScene("s1", "c_founder"),
		generatedScene("s2", "c_founder"),
		{ID: "s3", PrimaryKind: trendlymodels.SceneVideo}, // uploaded, no Gen
	}

	if _, refusal := resolveCast(s, []string{"c_founder"}, "s4", scenes); refusal == nil {
		t.Error("s4 is the third GENERATED appearance and should be refused")
	}

	only := []trendlymodels.StudioScene{{ID: "s3", PrimaryKind: trendlymodels.SceneVideo}}
	if _, refusal := resolveCast(s, []string{"c_founder"}, "s4", only); refusal != nil {
		t.Errorf("uploaded footage should not count toward the limit, got: %v", refusal.Body)
	}
}

// A cast id that is not in the studio is a client bug, and generating anyway
// would spend money on a prompt missing the look it was supposed to carry.
func TestUnknownCastIsRefused(t *testing.T) {
	s := &trendlymodels.Studio{}
	_, refusal := resolveCast(s, []string{"c_nobody"}, "s1", nil)
	if refusal == nil || refusal.Body["error"] != "unknown_cast" {
		t.Fatalf("an unknown cast id should be refused, got: %v", refusal)
	}
}

// The canvas decides the ratio, and only the two the providers accept exist.
func TestAspectFollowsTheCanvas(t *testing.T) {
	if got := aspectFor(1080, 1920); got != "9:16" {
		t.Errorf("portrait canvas = %q, want 9:16", got)
	}
	if got := aspectFor(1920, 1080); got != "16:9" {
		t.Errorf("landscape canvas = %q, want 16:9", got)
	}
	// A square-ish post has no label of its own; generating at 9:16 and
	// cropping beats refusing to generate at all.
	if got := aspectFor(1080, 1350); got != "9:16" {
		t.Errorf("4:5 canvas = %q, want 9:16", got)
	}
}
