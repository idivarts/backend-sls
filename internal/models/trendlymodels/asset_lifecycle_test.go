package trendlymodels

import (
	"testing"

	"cloud.google.com/go/firestore"
)

// A row written before the media registry existed has NO status, and it has to
// stay pickable. Reading an empty status as "not finished uploading" would hide
// every asset a brand had before this shipped — which is the one regression a
// library migration must not have.
func TestAssetWithNoStatusIsStillPickable(t *testing.T) {
	cases := map[string]bool{
		"":                  true,
		AssetStatusReady:    true,
		AssetStatusPending:  false,
		AssetStatusArchived: false,
		AssetStatusFailed:   false,
	}
	for status, want := range cases {
		if got := assetIsPickable(Asset{Status: status}); got != want {
			t.Errorf("status %q: pickable = %v, want %v", status, got, want)
		}
	}
}

// Search has to find a generation by the words it was ASKED to be. A generated
// clip has no filename anybody chose, so the prompt is the only description
// attached to it.
func TestSearchMatchesOnThePrompt(t *testing.T) {
	a := Asset{
		Kind:   AssetKindVideo,
		Origin: &AssetOrigin{Kind: AssetOriginGenerated, Prompt: "founder laughing at her desk"},
	}
	if !assetMatches(a, []string{"founder", "laughing"}) {
		t.Error("should match words from the prompt")
	}
	// Every word has to appear: "founder laughing" must not match every clip
	// that merely has a founder in it.
	if assetMatches(a, []string{"founder", "cycling"}) {
		t.Error("should not match when one word is absent")
	}
}

func TestSearchMatchesOnTranscriptAndShots(t *testing.T) {
	a := Asset{
		Kind: AssetKindVideo,
		Ingest: &AssetIngest{
			Text:  "so the Sunday night problem is really about planning",
			Shots: []Shot{{Description: "a woman picks up a cup"}},
		},
	}
	if !assetMatches(a, []string{"sunday", "planning"}) {
		t.Error("should match the transcript")
	}
	if !assetMatches(a, []string{"picks", "cup"}) {
		t.Error("should match a shot description")
	}
}

// Reference counts move for a SET of media — a scene references everything its
// layers reference — and the same media placed twice in one scene must only
// count once, or clearing that scene leaves the count stuck above zero and the
// media can never be archived.
func TestRefCountTargetsAreDeduped(t *testing.T) {
	got := dedupeStrings([]string{"a", "b", "a", "", "c", "b"})
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

// UpdateAsset is for descriptive fields. The lifecycle fields also move the
// per-brand storage rollup, and a size written without its rollup is a storage
// figure that silently drifts — so the model refuses rather than trusting every
// future caller to remember.
func TestUpdateAssetRefusesLifecycleFields(t *testing.T) {
	for _, path := range []string{"status", "sizeBytes", "refCount", "archivedAt"} {
		// The refusal happens before any Firestore call, so a nil context is
		// fine here and the test needs no emulator.
		err := UpdateAsset(nil, "brand", "asset", []firestore.Update{{Path: path, Value: 1}})
		if err == nil {
			t.Errorf("UpdateAsset should refuse %q", path)
		}
	}
}
