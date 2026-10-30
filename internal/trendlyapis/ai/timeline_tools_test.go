package ai

import (
	"testing"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
)

// The parts of cut_footage that decide what gets rendered, without Firestore.
// A model reading a shot list rounds past the end of a take routinely, and
// every one of these cases is a clip that would otherwise freeze, strobe or
// render something nobody asked for.

func TestClampWindowKeepsThePieceInsideTheSource(t *testing.T) {
	cases := []struct {
		name               string
		start, end, source int
		wantStart, wantEnd int
	}{
		{"inside", 1000, 2000, 5000, 1000, 2000},
		{"past the end", 4000, 9000, 5000, 4000, 5000},
		{"entirely past the end", 7000, 8000, 5000, 5000, 5000},
		{"negative start", -500, 1000, 5000, 0, 1000},
		// Reversed rather than rejected: the pieces are still the ones the model
		// meant, and a swap is the only reading that keeps them.
		{"reversed", 3000, 1000, 5000, 1000, 3000},
		// Nothing known about the source (an upload that reported no duration):
		// clamping to zero would delete every piece.
		{"unknown length", 1000, 60000, 0, 1000, 60000},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			start, end := clampWindow(c.start, c.end, c.source)
			if start != c.wantStart || end != c.wantEnd {
				t.Fatalf("clampWindow(%d,%d,%d) = %d,%d; want %d,%d",
					c.start, c.end, c.source, start, end, c.wantStart, c.wantEnd)
			}
		})
	}
}

func TestOnScreenMsDividesByTheSpeed(t *testing.T) {
	// At 2x a 3s window is on screen for 1.5s. Multiplying instead is how a
	// sped-up clip runs out of source halfway through.
	if got := onScreenMs(3000, 2); got != 1500 {
		t.Fatalf("2x of 3000ms = %d; want 1500", got)
	}
	if got := onScreenMs(3000, 0.5); got != 6000 {
		t.Fatalf("half speed of 3000ms = %d; want 6000", got)
	}
	if got := onScreenMs(3000, 0); got != 3000 {
		t.Fatalf("speed 0 must fall back to 1x, got %d", got)
	}
	if got := onScreenMs(10, 1); got != minPieceMs {
		t.Fatalf("a sliver must still be renderable: got %d, want %d", got, minPieceMs)
	}
}

func designComposition() *trendlymodels.Composition {
	return trendlymodels.CompositionFromDesign("rev1", 1080, 1920, 30, 6000)
}

func TestFirstDesignClipDropsTheOldTransition(t *testing.T) {
	c := designComposition()
	c.Tracks[0].Clips[0].TransitionIn = &trendlymodels.CompositionTransition{
		Kind: trendlymodels.CompositionTransitionFade, Ms: 400,
	}
	clip := firstDesignClip(c)
	if clip == nil {
		t.Fatal("the design clip was not found")
	}
	// It is about to open or close a different sequence; whatever it used to
	// enter on described the old one.
	if clip.TransitionIn != nil {
		t.Fatalf("expected the transition to be dropped, got %+v", clip.TransitionIn)
	}
}

func TestFirstDesignClipIsNilWithoutOne(t *testing.T) {
	in, out, speed, vol := 0, 1000, 1.0, 0.0
	c := &trendlymodels.Composition{
		Width: 1080, Height: 1920, FPS: 30,
		Tracks: []trendlymodels.CompositionTrack{{
			Kind: trendlymodels.CompositionTrackVideo,
			Clips: []trendlymodels.CompositionClip{{
				ID: "f", DurationMs: 1000,
				Source: trendlymodels.CompositionClipSource{
					Kind: trendlymodels.ClipSourceFootage, URL: "https://x/v.mp4",
					InMs: &in, OutMs: &out, Speed: &speed, Volume: &vol,
				},
			}},
		}},
	}
	if firstDesignClip(c) != nil {
		t.Fatal("a footage-only timeline has no design clip to keep")
	}
	if !hasFootage(c.Tracks[0].Clips) {
		t.Fatal("hasFootage missed a footage clip")
	}
}

func TestDescribeCompositionReportsMeasuredNumbers(t *testing.T) {
	in, out, speed, vol := 500, 3500, 2.0, 1.0
	c := &trendlymodels.Composition{
		Width: 1080, Height: 1920, FPS: 30,
		Tracks: []trendlymodels.CompositionTrack{{
			Kind: trendlymodels.CompositionTrackVideo,
			Clips: []trendlymodels.CompositionClip{
				{
					ID: "a", DurationMs: 2000,
					Source: trendlymodels.CompositionClipSource{
						Kind: trendlymodels.ClipSourceDesign, RevisionID: "rev1",
					},
				},
				{
					ID: "b", DurationMs: 1500,
					TransitionIn: &trendlymodels.CompositionTransition{
						Kind: trendlymodels.CompositionTransitionFade, Ms: 400,
					},
					Source: trendlymodels.CompositionClipSource{
						Kind: trendlymodels.ClipSourceFootage, URL: "https://x/v.mp4",
						AssetID: "asset1", InMs: &in, OutMs: &out, Speed: &speed, Volume: &vol,
					},
				},
			},
		}},
	}
	// Deliberately WRONG stored caches, to prove the description is measured:
	// quoting a cached total the renderer disagrees with is how a model ends up
	// confidently describing a video nobody will see.
	c.DurationMs = 99999
	c.Tracks[0].Clips[1].StartMs = 99999

	got := describeComposition(c)
	if got["durationMs"] != 2000+1500-400 {
		t.Fatalf("durationMs = %v; want %d", got["durationMs"], 2000+1500-400)
	}
	clips, ok := got["clips"].([]map[string]any)
	if !ok || len(clips) != 2 {
		t.Fatalf("expected two described clips, got %v", got["clips"])
	}
	if clips[1]["startMs"] != 1600 {
		t.Fatalf("second clip startMs = %v; want 1600", clips[1]["startMs"])
	}
	if clips[1]["entersWith"] != "fade 400ms" {
		t.Fatalf("entersWith = %v", clips[1]["entersWith"])
	}
	if clips[1]["assetId"] != "asset1" {
		t.Fatalf("the asset id has to travel, so a follow-up call can name it: %v", clips[1])
	}
	window, ok := clips[1]["sourceWindowMs"].([]int)
	if !ok || window[0] != 500 || window[1] != 3500 {
		t.Fatalf("sourceWindowMs = %v", clips[1]["sourceWindowMs"])
	}
	if clips[1]["speed"] != 2.0 || clips[1]["ownSound"] != 1.0 {
		t.Fatalf("speed/volume not reported: %v", clips[1])
	}
	// A cut has no fade to report, and index 0 can never have one.
	if clips[0]["entersWith"] != "cut" {
		t.Fatalf("first clip entersWith = %v", clips[0]["entersWith"])
	}
}

func TestDescribeCompositionFlagsAnUnlandedGeneration(t *testing.T) {
	c := &trendlymodels.Composition{
		Width: 1080, Height: 1920, FPS: 30,
		Tracks: []trendlymodels.CompositionTrack{{
			Kind: trendlymodels.CompositionTrackVideo,
			Clips: []trendlymodels.CompositionClip{{
				ID: "g", DurationMs: 4000,
				Source: trendlymodels.CompositionClipSource{
					Kind: trendlymodels.ClipSourceGenVideo, Model: "m", Prompt: "p",
				},
			}},
		}},
	}
	clips := describeComposition(c)["clips"].([]map[string]any)
	if clips[0]["note"] == nil {
		t.Fatal("a generated clip with no url cannot render, and the model has to be told")
	}
}
