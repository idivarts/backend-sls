package ai

import (
	"testing"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
)

func frames(n int) []trendlymodels.PreviewFrame {
	out := make([]trendlymodels.PreviewFrame, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, trendlymodels.PreviewFrame{URL: string(rune('a'+i)) + ".png", TimeMs: i * 1000})
	}
	return out
}

func TestPickScoreFramesKeepsThemAllWhenThereAreFew(t *testing.T) {
	got := pickScoreFrames(frames(4), 6)
	if len(got) != 4 || got[0] != "a.png" || got[3] != "d.png" {
		t.Fatalf("got %v", got)
	}
}

// Every frame is an image on a vision call and the bill scales with them, so
// a long film gets a spread rather than a slideshow.
func TestPickScoreFramesCapsALongFilm(t *testing.T) {
	got := pickScoreFrames(frames(40), 6)
	if len(got) != 6 {
		t.Fatalf("expected 6 frames, got %d", len(got))
	}
}

// The first frame is the thumbnail social players use and the last carries
// the ask. Taking the first n would judge the opening and never see the end.
func TestPickScoreFramesKeepsBothEnds(t *testing.T) {
	all := frames(40)
	got := pickScoreFrames(all, 6)
	if got[0] != all[0].URL {
		t.Fatalf("the first frame was dropped: %v", got[0])
	}
	if got[len(got)-1] != all[len(all)-1].URL {
		t.Fatalf("the last frame was dropped: %v", got[len(got)-1])
	}
}

func TestPickScoreFramesIsSpreadNotClustered(t *testing.T) {
	got := pickScoreFrames(frames(40), 5)
	seen := map[string]bool{}
	for _, u := range got {
		if seen[u] {
			t.Fatalf("the same frame was sent twice: %v", got)
		}
		seen[u] = true
	}
}

func TestPickScoreFramesHandlesNothing(t *testing.T) {
	if got := pickScoreFrames(nil, 6); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
	if got := pickScoreFrames(frames(3), 0); got != nil {
		t.Fatalf("expected nil for max 0, got %v", got)
	}
}
