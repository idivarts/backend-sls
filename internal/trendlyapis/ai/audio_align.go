package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/elevenlabs"
	"github.com/idivarts/backend-sls/pkg/openrouter"
)

// Retiming the video to the voice.
//
// A budget keeps a script roughly the right length and check_audio_fit says
// when it is not, but neither makes the picture change at the moment the line
// does — TTS pace varies between takes of the same words, so "roughly" is as
// good as estimating gets.
//
// ElevenLabs will return character-level timings alongside the audio, which
// turns the end of each scene's line into a measured fact. So the scenes are
// retimed to the voice rather than the script being trimmed until it happens
// to fit. The user hears one natural read; the video follows it.
//
// The alternative — one TTS clip per scene — also gives exact boundaries, at
// the cost of a prosody reset and a seam at every cut. One take read straight
// through sounds like a person; five takes stitched together do not.

const toolAlignScenes = "align_scenes_to_voiceover"

// A scene ends on the last syllable of its line, which cuts the picture a beat
// too early to watch comfortably. This is the breath after it.
const sceneTailMs = 250

// What goes between two scenes' lines in the single take. A blank line reads
// as a beat rather than a hard stop, which is what a cut between scenes
// sounds like anyway.
const sceneJoiner = "\n\n"

func alignScenesTool() openrouter.Tool {
	return openrouter.NewFunctionTool(
		toolAlignScenes,
		"Retime the video's scenes to the voiceover that was generated for them, so each "+
			"scene lasts exactly as long as its own line takes to say. Call this after "+
			"generate_voiceover when you passed sceneTexts — it uses the character-level "+
			"timings captured during generation, so it is exact rather than estimated, and "+
			"it costs nothing.\n"+
			"This is how the voice stops describing a scene that is no longer on screen. "+
			"Prefer it to trimming lines: the words the user approved stay as written and "+
			"the picture adapts.",
		openrouter.ObjectSchema(map[string]any{}, []string{}),
	)
}

// sceneBoundariesFrom maps each scene's text onto the generated audio.
//
// The timings are index-matched to the characters of the text as the model
// received it. When the counts agree that is a direct lookup; when they do not
// — the provider normalises numbers and abbreviations before speaking them —
// the position is scaled instead, and the caller is told the result is
// approximate rather than being handed a precise-looking lie.
func sceneBoundariesFrom(sceneTexts []string, sep string, timings []elevenlabs.CharTiming) ([]trendlymodels.SceneBoundary, bool) {
	if len(sceneTexts) == 0 || len(timings) == 0 {
		return nil, false
	}
	var full strings.Builder
	ends := make([]int, 0, len(sceneTexts))
	starts := make([]int, 0, len(sceneTexts))
	for i, t := range sceneTexts {
		if i > 0 {
			full.WriteString(sep)
		}
		starts = append(starts, len([]rune(full.String())))
		full.WriteString(t)
		ends = append(ends, len([]rune(full.String())))
	}
	total := len([]rune(full.String()))
	if total == 0 {
		return nil, false
	}

	exact := len(timings) == total
	at := func(runeIdx int) (int, int) {
		var i int
		if exact {
			i = runeIdx
		} else {
			i = int(float64(runeIdx) / float64(total) * float64(len(timings)))
		}
		if i < 0 {
			i = 0
		}
		if i >= len(timings) {
			i = len(timings) - 1
		}
		return timings[i].StartMs, timings[i].EndMs
	}

	out := make([]trendlymodels.SceneBoundary, 0, len(sceneTexts))
	for i := range sceneTexts {
		s, _ := at(starts[i])
		_, e := at(ends[i] - 1)
		if e < s {
			e = s
		}
		out = append(out, trendlymodels.SceneBoundary{Scene: i + 1, StartMs: s, EndMs: e})
	}
	return out, exact
}

var reDataDuration = regexp.MustCompile(`data-duration\s*=\s*"(\d+)"`)

// retimeScenes rewrites each scene's data-duration in document order.
//
// A regex rather than a parse-and-serialise round trip: re-emitting the
// document through an HTML serialiser rewrites entities and self-closing tags
// across the whole design, which would show up as a huge diff on a revision
// whose only real change is a handful of numbers.
func retimeScenes(doc string, durations []int) (string, int) {
	idx, changed := 0, 0
	out := reDataDuration.ReplaceAllStringFunc(doc, func(m string) string {
		i := idx
		idx++
		if i >= len(durations) || durations[i] <= 0 {
			return m
		}
		cur := reDataDuration.FindStringSubmatch(m)
		if len(cur) == 2 {
			if n, err := strconv.Atoi(cur[1]); err == nil && n == durations[i] {
				return m
			}
		}
		changed++
		return fmt.Sprintf(`data-duration="%d"`, durations[i])
	})
	return out, changed
}

func runAlignScenes(ctx context.Context, brandID, contentID string) (string, error) {
	if contentID == "" {
		return jsonResult(map[string]any{"ok": false, "error": "no content in context"}), nil
	}
	content, err := trendlymodels.GetContent(brandID, contentID)
	if err != nil || content == nil {
		return jsonResult(map[string]any{"ok": false, "error": "content not found"}), nil
	}
	if content.Audio == nil || len(content.Audio.SceneBoundaries) == 0 {
		return jsonResult(map[string]any{
			"ok": false,
			"error": "no scene timings were captured. Generate the voiceover again passing " +
				"sceneTexts (one entry per scene) — that is what records where each line " +
				"lands in the audio.",
		}), nil
	}
	if content.DesignRef == nil || content.DesignRef.RevisionID == "" {
		return jsonResult(map[string]any{"ok": false, "error": "no design to retime"}), nil
	}
	rev, err := trendlymodels.GetDesignRevision(brandID, contentID, content.DesignRef.RevisionID)
	if err != nil || rev == nil {
		return jsonResult(map[string]any{"ok": false, "error": "current design not found"}), nil
	}
	if rev.DocType != "video" {
		return jsonResult(map[string]any{"ok": false, "error": "this design is not a video, so there is nothing to retime"}), nil
	}

	existing := sceneDurations(rev.HTML)
	bounds := content.Audio.SceneBoundaries
	if len(bounds) != len(existing) {
		return jsonResult(map[string]any{
			"ok": false,
			"error": fmt.Sprintf(
				"the voiceover was recorded for %d scenes but the design has %d. Regenerate "+
					"the voiceover with one sceneTexts entry per scene, or change the design to match.",
				len(bounds), len(existing)),
		}), nil
	}

	// A scene with no data-duration has no attribute to rewrite, so it would
	// be silently skipped and left at the renderer's 3000ms fallback — the one
	// scene most likely to be the cause of the drift being reported.
	for i, d := range existing {
		if d <= 0 {
			return jsonResult(map[string]any{
				"ok": false,
				"error": fmt.Sprintf(
					"scene %d has no data-duration, so it cannot be retimed and would stay at the "+
						"3000ms fallback. Set it with apply_design_edits first, then call this again.", i+1),
			}), nil
		}
	}

	want := make([]int, len(bounds))
	type change struct {
		Scene  int `json:"scene"`
		FromMs int `json:"fromMs"`
		ToMs   int `json:"toMs"`
	}
	var changes []change
	for i, b := range bounds {
		d := b.EndMs - b.StartMs + sceneTailMs
		if d < 500 {
			// A scene with no line still has to be on screen long enough to
			// register. Retiming it to nothing would make the video stutter.
			d = 500
		}
		want[i] = d
		if i < len(existing) && existing[i] != d {
			changes = append(changes, change{Scene: i + 1, FromMs: existing[i], ToMs: d})
		}
	}

	html, changed := retimeScenes(rev.HTML, want)
	if changed == 0 {
		return jsonResult(map[string]any{
			"ok":      true,
			"changed": 0,
			"summary": "The scenes already match the voiceover — nothing to retime.",
		}), nil
	}
	if problems := validateDesignHTML(html, rev.DocType, 0); len(problems) > 0 {
		// Lengthening a scene cannot break the design, but shortening one can
		// cut an animation off mid-flight — and that check already exists.
		return designProblemsResult(problems), nil
	}

	revID, err := persistDesign(brandID, contentID, wrapHTML(html), rev.Width, rev.Height,
		rev.SlideCount, totalMs(want), rev.DocType, "edit", rev.ID)
	if err != nil {
		return jsonResult(map[string]any{"ok": false, "error": err.Error()}), err
	}

	return jsonResult(map[string]any{
		"ok":          true,
		"revisionId":  revID,
		"changed":     changed,
		"changes":     changes,
		"approximate": !content.Audio.SceneTimingsExact,
		"totalMs":     totalMs(want),
		"note": "Scenes retimed to the voiceover. Call render_design_preview and look at it: " +
			"a scene that got longer may now hold on a finished animation.",
	}), nil
}

func totalMs(d []int) int {
	t := 0
	for _, v := range d {
		t += v
	}
	return t
}

// sceneTextsArg is the optional per-scene breakdown on generate_voiceover.
type sceneTextsArg struct {
	SceneTexts []string `json:"sceneTexts"`
}

func parseSceneTexts(arguments string) []string {
	var a sceneTextsArg
	if err := json.Unmarshal([]byte(arguments), &a); err != nil {
		return nil
	}
	out := make([]string, 0, len(a.SceneTexts))
	for _, t := range a.SceneTexts {
		out = append(out, strings.TrimSpace(t))
	}
	return out
}
