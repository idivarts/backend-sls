package ai

import (
	"strings"
	"testing"

	"github.com/idivarts/backend-sls/pkg/elevenlabs"
)

const videoDesign = `<!doctype html><html><body>
<div data-carousel>
  <section data-slide data-duration="3000"><h1>One</h1></section>
  <section data-slide data-duration="4500"><h1>Two</h1></section>
  <section data-slide><h1>Three has no duration</h1></section>
</div></body></html>`

func TestSceneDurations(t *testing.T) {
	got := sceneDurations(videoDesign)
	want := []int{3000, 4500, 0}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("scene %d: got %d, want %d", i+1, got[i], want[i])
		}
	}
}

// A missing data-duration reads as 0 rather than the renderer's 3000ms
// fallback: reporting a fit the render will not reproduce is worse than
// saying the duration is not set.
func TestSceneDurationsDoesNotGuessTheFallback(t *testing.T) {
	if got := sceneDurations(videoDesign); got[2] != 0 {
		t.Errorf("an unset duration should be 0, got %d", got[2])
	}
}

const sceneScript = `## Hook (first 3 seconds)
**On-screen text:** "18 hours for one cup."
**VO:** "Eighteen hours. For coffee you drink cold."

## Body
**Scene 1 — the question (4s)**
**Visual:** Cut to grounds steeping.
**On-screen text:** "Why not just use hot water?"
**VO:** "Hot water is fast. It also pulls the bitterness out."

**Scene 2 — the mechanism (5s)**
**VO:** "Cold water works slower, so it only takes the smooth part."

## CTA
**VO:** "Send this to someone who gave up on cold brew."
`

func TestScriptSceneLinesTakesOnlyWhatIsSpoken(t *testing.T) {
	scenes := scriptSceneLines(sceneScript)
	if len(scenes) != 2 {
		t.Fatalf("want 2 scenes, got %d: %#v", len(scenes), scenes)
	}
	// Visual direction and on-screen text are not narrated, and counting them
	// would make every scene look wildly over budget.
	if strings.Contains(scenes[0], "Cut to grounds") || strings.Contains(scenes[0], "Why not just") {
		t.Errorf("scene 1 picked up production lines: %q", scenes[0])
	}
	if !strings.Contains(scenes[0], "Hot water is fast") {
		t.Errorf("scene 1 lost its spoken line: %q", scenes[0])
	}
	if !strings.Contains(scenes[1], "Cold water works slower") {
		t.Errorf("scene 2 lost its spoken line: %q", scenes[1])
	}
}

func TestSpokenMs(t *testing.T) {
	// 25 words at 2.5 w/s is 10s.
	if got := spokenMs(25); got != 10000 {
		t.Errorf("got %d, want 10000", got)
	}
	if got := spokenMs(0); got != 0 {
		t.Errorf("empty should be 0, got %d", got)
	}
}

func TestSceneBudgetBriefStatesAWordCountPerScene(t *testing.T) {
	b := sceneBudgetBrief([]int{3000, 4500, 0})
	for _, want := range []string{"Scene 1: 3.0s", "7 words", "Scene 2: 4.5s", "11 words", "Scene 3: duration not set"} {
		if !strings.Contains(b, want) {
			t.Errorf("brief is missing %q:\n%s", want, b)
		}
	}
	// It must point at the tool, or the model writes to budget once and never
	// checks what the voice actually came out as.
	if !strings.Contains(b, toolCheckAudioFit) {
		t.Error("the brief should tell the model to verify afterwards")
	}
}

func TestSceneBudgetBriefIsEmptyWithoutScenes(t *testing.T) {
	if b := sceneBudgetBrief(nil); b != "" {
		t.Errorf("want empty, got %q", b)
	}
}

// ── Retiming ────────────────────────────────────────────────────────────────

func timingsFor(text string, msPerChar int) []elevenlabs.CharTiming {
	out := make([]elevenlabs.CharTiming, 0, len([]rune(text)))
	for i, r := range []rune(text) {
		out = append(out, elevenlabs.CharTiming{
			Char: string(r), StartMs: i * msPerChar, EndMs: (i + 1) * msPerChar,
		})
	}
	return out
}

func TestSceneBoundariesAreExactWhenTheCountsAgree(t *testing.T) {
	scenes := []string{"One two", "Three four five", "Six"}
	full := strings.Join(scenes, sceneJoiner)
	bounds, exact := sceneBoundariesFrom(scenes, sceneJoiner, timingsFor(full, 10))
	if !exact {
		t.Fatal("character counts match, so the mapping should be exact")
	}
	if len(bounds) != 3 {
		t.Fatalf("want 3 boundaries, got %d", len(bounds))
	}
	if bounds[0].StartMs != 0 {
		t.Errorf("scene 1 should start at 0, got %d", bounds[0].StartMs)
	}
	// Each boundary must start no earlier than the previous one ends.
	for i := 1; i < len(bounds); i++ {
		if bounds[i].StartMs < bounds[i-1].EndMs {
			t.Errorf("scene %d starts (%d) before scene %d ends (%d)",
				i+1, bounds[i].StartMs, i, bounds[i-1].EndMs)
		}
	}
	if bounds[2].EndMs <= bounds[2].StartMs {
		t.Error("the last scene has no duration")
	}
}

// The provider normalises numbers and abbreviations before speaking them, so
// the counts can disagree. Scaling is still useful; claiming it is exact is not.
func TestSceneBoundariesFallBackToScalingAndSaySo(t *testing.T) {
	scenes := []string{"18 hours", "for one cup"}
	full := strings.Join(scenes, sceneJoiner)
	short := timingsFor(full, 10)[:len([]rune(full))-6] // as if it spoke fewer characters
	bounds, exact := sceneBoundariesFrom(scenes, sceneJoiner, short)
	if exact {
		t.Error("counts disagree, so this must not be reported as exact")
	}
	if len(bounds) != 2 {
		t.Fatalf("want 2 boundaries, got %d", len(bounds))
	}
}

func TestSceneBoundariesHandlesNothing(t *testing.T) {
	if b, _ := sceneBoundariesFrom(nil, sceneJoiner, nil); b != nil {
		t.Error("no scenes should give no boundaries")
	}
	if b, _ := sceneBoundariesFrom([]string{"a"}, sceneJoiner, nil); b != nil {
		t.Error("no timings should give no boundaries")
	}
}

func TestRetimeScenesRewritesInOrder(t *testing.T) {
	got, changed := retimeScenes(videoDesign, []int{4000, 4500, 2000})
	// Scene 1 is rewritten; scene 2 already reads 4500 so it must not count as
	// a change; scene 3 carries no data-duration attribute, so there is
	// nothing to rewrite — runAlignScenes refuses that case separately rather
	// than inventing an attribute here.
	if changed != 1 {
		t.Errorf("want 1 change, got %d", changed)
	}
	if !strings.Contains(got, `data-duration="4000"`) {
		t.Error("scene 1 was not retimed")
	}
	if !strings.Contains(got, `data-duration="4500"`) {
		t.Error("scene 2 should still read 4500")
	}
}

func TestRetimeScenesLeavesEverythingElseAlone(t *testing.T) {
	got, _ := retimeScenes(videoDesign, []int{4000, 4500, 2000})
	for _, want := range []string{"<h1>One</h1>", "<h1>Two</h1>", "data-carousel", "<!doctype html>"} {
		if !strings.Contains(got, want) {
			t.Errorf("the rewrite lost %q", want)
		}
	}
}
