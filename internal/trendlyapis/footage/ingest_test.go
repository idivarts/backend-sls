package footage

import (
	"strings"
	"testing"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
)

func TestStripFenceSurvivesAFencedReply(t *testing.T) {
	// Models add a ```json wrapper despite being told JSON only, often enough
	// that stripping it is cheaper than a retry.
	want := `{"shots":[]}`
	for _, in := range []string{
		"```json\n" + want + "\n```",
		"```\n" + want + "\n```",
		want,
		"  " + want + "  ",
	} {
		if got := strings.TrimSpace(stripFence(in)); got != want {
			t.Errorf("stripFence(%q) = %q", in, got)
		}
	}
}

func TestShotTimesComeFromFrameIndices(t *testing.T) {
	// The model is good at "these frames are one shot" and bad at arithmetic
	// on timestamps, so it is asked for indices and the conversion happens
	// where the sampling interval is actually known.
	ingest := &trendlymodels.AssetIngest{
		DurationMs:      12000,
		SampleFrameURLs: make([]string, 12), // one slice per second
	}
	shots := shotsFromIndices(ingest, []indexedShot{
		{From: 0, To: 2, Description: "a wide shot of the shop"},
		{From: 3, To: 11, Description: "close on the counter"},
	})
	if len(shots) != 2 {
		t.Fatalf("got %+v", shots)
	}
	// Frames 0..2 cover the first three slices: 0ms to 3000ms.
	if shots[0].StartMs != 0 || shots[0].EndMs != 3000 {
		t.Errorf("first shot = %d..%d, want 0..3000", shots[0].StartMs, shots[0].EndMs)
	}
	// The last shot must reach the end of the footage, not stop a slice short.
	if shots[1].EndMs != 12000 {
		t.Errorf("last shot ends at %d, want 12000", shots[1].EndMs)
	}
}

func TestShotIndicesAreClampedAndJunkDropped(t *testing.T) {
	ingest := &trendlymodels.AssetIngest{DurationMs: 4000, SampleFrameURLs: make([]string, 4)}
	shots := shotsFromIndices(ingest, []indexedShot{
		// Past the end: a model that invents a thirteenth frame must not
		// produce a shot running past the footage.
		{From: 2, To: 99, Description: "overruns"},
		{From: -5, To: 1, Description: "starts before the start"},
		// Nonsense and empties are dropped rather than stored as blank shots.
		{From: 3, To: 1, Description: "backwards"},
		{From: 0, To: 1, Description: "   "},
	})
	if len(shots) != 2 {
		t.Fatalf("expected 2 usable shots, got %+v", shots)
	}
	for _, s := range shots {
		if s.EndMs > ingest.DurationMs || s.StartMs < 0 {
			t.Errorf("shot %+v escapes the footage", s)
		}
	}
}
