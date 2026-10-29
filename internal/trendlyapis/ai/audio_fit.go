package ai

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/openrouter"
	"golang.org/x/net/html"
)

// Keeping the voice and the scenes together.
//
// ── The problem ─────────────────────────────────────────────────────────────
// A voiceover is one clip generated from the whole script; the video is a
// sequence of scenes with their own data-duration. Nothing ties a word to a
// scene, so the clip simply plays from zero and whatever happens, happens. Two
// failures follow, and both are routine:
//
//   - the voice is still describing scene 2 while scene 3 is on screen, because
//     the script's scene 2 line takes longer to say than scene 2 lasts;
//   - the voice is cut off mid-word at the end, because the whole script takes
//     longer to say than the video runs.
//
// ── The approach ────────────────────────────────────────────────────────────
// The standard answer in narration-driven video is to constrain at write time
// and verify after generation, because neither half is sufficient alone. A
// model given a per-scene word budget writes to length most of the time; TTS
// pace still varies, so the result has to be measured rather than assumed.
//
// So: `sceneBudgetBrief` puts the budget in the prompt before anything is
// written, and `check_audio_fit` measures afterwards and says exactly which
// scene is over and by how much. The model is told to call it after generating
// and to fix what it reports — the same loop the design already runs with
// render_design_preview.

const toolCheckAudioFit = "check_audio_fit"

// Words per second of conversational delivery. The same figure the script
// checks use, so a writer is not told two different things about pace.
const spokenWordsPerSecond = 2.5

// How much a scene may overrun before it is worth reporting. Under this, the
// drift is smaller than the variation between two takes of the same line.
const sceneOverrunToleranceMs = 400

func audioFitTool() openrouter.Tool {
	return openrouter.NewFunctionTool(
		toolCheckAudioFit,
		"Check that the voiceover fits the video and that each scene's narration fits that "+
			"scene. Call this AFTER generating a voiceover, and again after changing the "+
			"script or any scene's duration. It costs nothing.\n"+
			"It reports, per scene, how long that scene runs and how long its line takes to "+
			"say, plus whether the whole voiceover overruns the video. Fix what it reports: "+
			"shorten the line, or lengthen the scene with apply_design_edits (data-duration). "+
			"A voiceover that overruns is cut off mid-word, and a scene whose line is too long "+
			"leaves the voice describing a scene that is no longer on screen.",
		openrouter.ObjectSchema(map[string]any{}, []string{}),
	)
}

var reSceneHeading = regexp.MustCompile(`(?im)^\s*(?:#{1,6}\s*)?(?:\*\*)?\s*(?:scene|beat)\s*(\d+)`)

// sceneDurations reads each scene's data-duration, in order. Missing or
// unparseable values come back as 0 so the caller can say "not set" rather
// than quietly substituting the 3000ms fallback and reporting a fit that the
// render will not reproduce.
func sceneDurations(doc string) []int {
	root, err := html.Parse(strings.NewReader(doc))
	if err != nil {
		return nil
	}
	out := []int{}
	for _, s := range slideNodes(root) {
		raw, ok := attr(s, "data-duration")
		if !ok {
			out = append(out, 0)
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil || n <= 0 {
			out = append(out, 0)
			continue
		}
		out = append(out, n)
	}
	return out
}

// scriptSceneLines splits the script's spoken words by scene.
//
// Only what is actually said counts. A scene's block carries visual direction,
// on-screen text and bracketed notes, and narrating those would be both wrong
// and a wildly pessimistic estimate — so this reuses the same spoken/production
// classification the script checks use.
func scriptSceneLines(script string) []string {
	plain := stripCommentary(plainScript(script))
	lines := normaliseScript(plain)

	var scenes []string
	var cur strings.Builder
	started := false

	flush := func() {
		if started {
			scenes = append(scenes, strings.TrimSpace(cur.String()))
			cur.Reset()
		}
	}

	for _, l := range lines {
		raw := strings.TrimSpace(l.Label + " " + l.Body)
		if reSceneHeading.MatchString(raw) {
			flush()
			started = true
			continue
		}
		if !started {
			continue
		}
		// Spoken lines, plus bare quoted narration that carries no label. A
		// production label wins over the quotes: on-screen text is written in
		// quotes too and counting it would make every scene look wildly over
		// budget, when in fact nobody reads it aloud.
		if l.Spoken || (l.Quoted && !l.Note) {
			if cur.Len() > 0 {
				cur.WriteString(" ")
			}
			cur.WriteString(l.Body)
		}
	}
	flush()
	return scenes
}

func wordsIn(s string) int { return len(strings.Fields(s)) }

func spokenMs(words int) int {
	if words <= 0 {
		return 0
	}
	return int(float64(words) / spokenWordsPerSecond * 1000)
}

// sceneBudgetBrief is appended to the content chat's prompt when the design is
// a video. Constraining at write time is the cheap half of keeping the voice
// and the picture together: a model that is told scene 2 has room for eleven
// words rarely writes twenty-five.
func sceneBudgetBrief(durations []int) string {
	if len(durations) == 0 {
		return ""
	}
	var sb strings.Builder
	total := 0
	sb.WriteString("SCENE TIMING — THE VOICEOVER MUST FIT IT.\n")
	sb.WriteString("The video's scenes, and how many spoken words each has room for at a " +
		"natural pace:\n")
	for i, d := range durations {
		total += d
		if d <= 0 {
			sb.WriteString(fmt.Sprintf("- Scene %d: duration not set\n", i+1))
			continue
		}
		sb.WriteString(fmt.Sprintf("- Scene %d: %.1fs ≈ %d words\n",
			i+1, float64(d)/1000, int(float64(d)/1000*spokenWordsPerSecond)))
	}
	sb.WriteString(fmt.Sprintf("Total: %.1fs ≈ %d words.\n\n",
		float64(total)/1000, int(float64(total)/1000*spokenWordsPerSecond)))
	sb.WriteString("Write each scene's spoken line to its own budget, not the total. A line " +
		"that overruns its scene leaves the voice describing something that is no longer on " +
		"screen, and the whole voiceover overrunning means it is cut off mid-word — neither " +
		"is recoverable at render. If a line genuinely needs longer, lengthen that scene's " +
		"data-duration with apply_design_edits instead of trimming the sentence to nonsense. " +
		"Then call " + toolCheckAudioFit + " and fix whatever it reports.")
	return sb.String()
}

// runCheckAudioFit measures what was actually produced.
func runCheckAudioFit(ctx context.Context, brandID, contentID string) (string, error) {
	if contentID == "" {
		return jsonResult(map[string]any{"ok": false, "error": "no content in context"}), nil
	}
	content, err := trendlymodels.GetContent(brandID, contentID)
	if err != nil || content == nil {
		return jsonResult(map[string]any{"ok": false, "error": "content not found"}), nil
	}
	if content.DesignRef == nil || content.DesignRef.RevisionID == "" {
		return jsonResult(map[string]any{"ok": false, "error": "no design yet, so there are no scenes to fit the voice to"}), nil
	}
	rev, err := trendlymodels.GetDesignRevision(brandID, contentID, content.DesignRef.RevisionID)
	if err != nil || rev == nil {
		return jsonResult(map[string]any{"ok": false, "error": "current design not found"}), nil
	}
	if rev.DocType != "video" {
		return jsonResult(map[string]any{"ok": false, "error": "this design is not a video, so nothing has to line up in time"}), nil
	}

	durations := sceneDurations(rev.HTML)
	lines := scriptSceneLines(content.Script)

	videoMs := 0
	for _, d := range durations {
		videoMs += d
	}

	type sceneFit struct {
		Scene       int    `json:"scene"`
		DurationMs  int    `json:"durationMs"`
		Words       int    `json:"words"`
		EstimatedMs int    `json:"estimatedMs"`
		OverrunMs   int    `json:"overrunMs,omitempty"`
		Line        string `json:"line,omitempty"`
	}

	var scenes []sceneFit
	var problems []string
	for i, d := range durations {
		line := ""
		if i < len(lines) {
			line = lines[i]
		}
		w := wordsIn(line)
		est := spokenMs(w)
		f := sceneFit{Scene: i + 1, DurationMs: d, Words: w, EstimatedMs: est, Line: truncate(line, 90)}
		if d > 0 && est > d+sceneOverrunToleranceMs {
			f.OverrunMs = est - d
			problems = append(problems, fmt.Sprintf(
				"Scene %d: its line is %d words (≈%.1fs) but the scene runs %.1fs. Cut about %d words, or raise that scene's data-duration to %dms.",
				i+1, w, float64(est)/1000, float64(d)/1000, w-int(float64(d)/1000*spokenWordsPerSecond), est+200))
		}
		if d > 0 && w == 0 {
			problems = append(problems, fmt.Sprintf(
				"Scene %d has no spoken line, so the voice will be silent over it or still finishing the previous scene. Give it a line or shorten the scene.",
				i+1))
		}
		scenes = append(scenes, f)
	}

	if len(lines) > len(durations) {
		problems = append(problems, fmt.Sprintf(
			"The script has %d scenes but the design has %d. The extra lines have no scene to play over and will run past the end.",
			len(lines), len(durations)))
	}

	out := map[string]any{
		"ok":      true,
		"videoMs": videoMs,
		"scenes":  scenes,
	}

	// The measured clip, when the app has measured it. This is the only number
	// here that is a fact rather than an estimate, so it is reported separately
	// and it is what decides the cut-off verdict.
	if content.Audio != nil && content.Audio.VoiceoverURL != "" {
		vo := map[string]any{"present": true}
		if content.Audio.VoiceoverMs > 0 {
			vo["measuredMs"] = content.Audio.VoiceoverMs
			if videoMs > 0 && int(content.Audio.VoiceoverMs) > videoMs+sceneOverrunToleranceMs {
				over := int(content.Audio.VoiceoverMs) - videoMs
				vo["overrunMs"] = over
				problems = append(problems, fmt.Sprintf(
					"The generated voiceover is %.1fs but the video is %.1fs, so the last %.1fs is cut off mid-word. Shorten the script or lengthen the scenes.",
					float64(content.Audio.VoiceoverMs)/1000, float64(videoMs)/1000, float64(over)/1000))
			}
		} else {
			vo["measuredMs"] = nil
			vo["note"] = "not measured yet — the app measures it when the content is open"
		}
		out["voiceover"] = vo
	} else {
		out["voiceover"] = map[string]any{"present": false}
	}

	out["problems"] = problems
	if len(problems) == 0 {
		out["summary"] = "The voiceover fits: every scene's line fits its scene, and the whole thing fits the video."
	} else {
		out["summary"] = fmt.Sprintf("%d thing(s) to fix before this renders cleanly.", len(problems))
	}
	return jsonResult(out), nil
}
