package design

// The film as words.
//
// ── Who this is for ──────────────────────────────────────────────────────
// Phase 17: "editing the film as words, for the people who think in copy
// rather than in elements." Someone writing an ad does not want to click a
// headline to change it; they want to read the thing top to bottom and fix
// the second line.
//
// ── What counts as a line ────────────────────────────────────────────────
// Two kinds, and both are copy:
//
//   - TEXT on screen: an element with an id that holds words and not other
//     elements. A <section> that contains two headlines is not a line, it
//     is where two lines live.
//   - A VOICEOVER: what the film SAYS. Its words are its
//     data-media-intent, which is why the media resolver now keeps that
//     attribute — a film that threw it away could show a script of its
//     on-screen text and nothing of its voice.
//
// ── One write path, not two ──────────────────────────────────────────────
// Editing a line goes through the same endpoint as the inspector, because
// the plan asks for exactly that: "there is one write path, not two". A
// voiceover line is the exception that proves it — it writes an intent
// rather than text, so it goes through the same handler with a different
// field, not through a second endpoint.

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/hfedit"
	"github.com/idivarts/backend-sls/pkg/mysentry"
)

// scriptLine is one piece of copy in the film.
type scriptLine struct {
	ElementID string `json:"elementId"`
	SceneID   string `json:"sceneId"`
	// Kind is "text" for words on screen, "voice" for words spoken.
	Kind string `json:"kind"`
	Tag  string `json:"tag"`
	Text string `json:"text"`
	// StartMs orders the script by when it is heard or seen, not by where
	// it happens to sit in the document.
	StartMs int `json:"startMs"`
}

// maxScriptLine bounds one line. A paragraph of body copy is a line; a
// pasted document is not.
const maxScriptLine = 2000

// captionWord is one word of a caption track, with the moment it appears.
type captionWord struct {
	Text       string `json:"text"`
	StartMs    int    `json:"startMs"`
	DurationMs int    `json:"durationMs"`
}

// captionTrack is one set of word-synced captions.
//
// Reported separately from the script's lines, and deliberately NOT as
// editable copy: a caption track is transcribed from the audio of the
// voiceover it follows, and rebuilt whenever that line changes. Editing
// the words here would be editing a transcript of something about to be
// replaced — the place to change them is the voiceover.
type captionTrack struct {
	ElementID string `json:"elementId"`
	SceneID   string `json:"sceneId"`
	// Follows is the id of the voiceover these words came out of.
	Follows string `json:"follows"`
	// Spoken is what that voiceover was asked to say, which is not always
	// what the transcriber heard — "three" comes back as "3". Showing both
	// is the point: these words are what a viewer READS, and they are
	// taken from what a viewer HEARS.
	Spoken  string        `json:"spoken"`
	StartMs int           `json:"startMs"`
	Words   []captionWord `json:"words"`
}

// GetDesignScript reads the film as a script.
// GET /api/design/brands/:brandId/designs/:designId/script
func GetDesignScript(c *gin.Context) {
	brandID, designID, ok := guard(c, false)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	d, err := trendlymodels.GetBrandDesign(ctx, brandID, designID)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not read the design"})
		return
	}
	if d == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no such design"})
		return
	}
	if d.ProjectPrefix == "" {
		// Not an error: a design nobody has written has an empty script,
		// and the panel says so rather than showing a failure.
		c.JSON(http.StatusOK, gin.H{"lines": []scriptLine{}, "captions": []captionTrack{}})
		return
	}
	source, rerr := readCurrentDocument(ctx, d)
	if rerr != nil {
		mysentry.Capture(rerr, map[string]string{"op": "design.script.read", "designId": designID})
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not read this film"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"lines":    readScript(source),
		"captions": readCaptions(source),
	})
}

// readScript pulls every line of copy out of a film, in the order it is
// seen or heard.
func readScript(source string) []scriptLine {
	scenes := hfedit.FindAll(source, func(e hfedit.Element) bool {
		_, has := e.Attrs["data-scene"]
		return has
	})
	name := func(s hfedit.Element) string {
		if n := strings.TrimSpace(s.Attrs["data-scene"]); n != "" {
			return n
		}
		return s.Attrs["id"]
	}
	sceneOf := func(el hfedit.Element) (string, int) {
		best := ""
		start := 0
		for _, s := range scenes {
			if s.Start <= el.Start && s.End >= el.End {
				// The innermost scene wins, for a film that nests them.
				best = name(s)
				start, _ = msAttr(s, "data-start")
			}
		}
		if best != "" {
			return best, start
		}
		// ── A line that sits outside every scene ─────────────────────────
		// Voiceovers routinely do: the director writes them at the root of
		// the composition, before the sections, because they are the
		// film's soundtrack rather than part of any one scene's markup.
		// Structurally they belong to no scene; to a person reading a
		// script they obviously belong to the scene they are HEARD in.
		//
		// Found on dev, where a script of two scenes came out as six
		// groups — "Hook", "Scene 2", "Outro", "Scene 4" — inventing
		// scenes that do not exist around lines that had nowhere to go.
		at, ok := msAttr(el, "data-start")
		if !ok {
			return "", 0
		}
		for _, s := range scenes {
			from, _ := msAttr(s, "data-start")
			dur, has := msAttr(s, "data-duration")
			if !has {
				continue
			}
			if at >= from && at < from+dur {
				return name(s), from
			}
		}
		return "", at
	}

	lines := make([]scriptLine, 0, 12)
	for _, el := range hfedit.FindAll(source, func(e hfedit.Element) bool { return e.Attrs["id"] != "" }) {
		sceneID, sceneStart := sceneOf(el)
		at, ok := msAttr(el, "data-start")
		if !ok {
			at = sceneStart
		}

		// A voiceover: what the film says.
		if strings.EqualFold(el.TagName, "audio") &&
			strings.EqualFold(strings.TrimSpace(el.Attrs["data-media"]), "voice") {
			words := strings.TrimSpace(el.Attrs["data-media-intent"])
			if words != "" {
				lines = append(lines, scriptLine{
					ElementID: el.Attrs["id"], SceneID: sceneID, Kind: "voice",
					Tag: el.TagName, Text: words, StartMs: at,
				})
			}
			continue
		}

		// Text on screen. Elements holding other elements are where lines
		// live, not lines themselves — and a caption track is generated
		// from the voiceover, so editing its words would be editing a
		// transcript of something that is about to be regenerated.
		// A <section> holding two headlines is where two lines live, not
		// a line. A headline holding an <em> is one line — the formatting
		// is part of the words, and flattening it on an edit is the trade
		// this panel makes knowingly (the inspector keeps the markup).
		if el.HoldsAddressable(source) || !el.HasChildren() {
			continue
		}
		// ── A caption track is not a line ────────────────────────────────
		// Captions are generated from the voiceover and rebuilt whenever
		// it changes, so editing them would be editing a transcript of
		// something about to be replaced. Three ways to recognise one,
		// because a track that has already been FILLED no longer carries
		// data-captions — the worker removes it once the words are in —
		// and its spans have no ids, so it read as an ordinary paragraph
		// and showed up in the script.
		if _, isCaptions := el.Attrs["data-captions"]; isCaptions {
			continue
		}
		if strings.Contains(el.Attrs["class"], "caption-track") {
			continue
		}
		if el.HoldsTimed(source) {
			continue
		}
		if _, isRoot := el.Attrs["data-composition-id"]; isRoot {
			continue
		}
		text := strings.TrimSpace(collapseSpaces(innerText(source, el)))
		if text == "" {
			continue
		}
		lines = append(lines, scriptLine{
			ElementID: el.Attrs["id"], SceneID: sceneID, Kind: "text",
			Tag: el.TagName, Text: text, StartMs: at,
		})
	}

	// By when it is seen or heard. A stable sort, so two lines at the same
	// moment keep the order the film puts them in.
	for i := 1; i < len(lines); i++ {
		for j := i; j > 0 && lines[j-1].StartMs > lines[j].StartMs; j-- {
			lines[j-1], lines[j] = lines[j], lines[j-1]
		}
	}
	return lines
}

// readCaptions pulls every caption track out of a film, in the order they
// appear on screen.
//
// A track is recognised the same three ways the worker recognises one:
// the attribute, the class the authoring contract names, or the fact that
// it holds timed children. A FILLED track may have lost its attribute on
// an older film, and one that has never been filled has no children —
// both are still caption tracks, and both belong in this panel, one of
// them showing that it is empty.
func readCaptions(source string) []captionTrack {
	scenes := hfedit.FindAll(source, func(e hfedit.Element) bool {
		_, has := e.Attrs["data-scene"]
		return has
	})
	sceneName := func(el hfedit.Element) string {
		for _, s := range scenes {
			if s.Start <= el.Start && s.End >= el.End {
				if n := strings.TrimSpace(s.Attrs["data-scene"]); n != "" {
					return n
				}
				return s.Attrs["id"]
			}
		}
		return ""
	}

	out := make([]captionTrack, 0, 4)
	for _, el := range hfedit.FindAll(source, isCaptionTrack(source)) {
		follows := strings.TrimSpace(el.Attrs["data-captions"])
		track := captionTrack{
			ElementID: el.Attrs["id"],
			SceneID:   sceneName(el),
			Follows:   follows,
			Words:     readCaptionWords(source, el),
		}
		track.StartMs, _ = msAttr(el, "data-start")
		if follows != "" {
			for _, v := range hfedit.FindAll(source, func(e hfedit.Element) bool {
				return e.Attrs["id"] == follows
			}) {
				track.Spoken = strings.TrimSpace(v.Attrs["data-media-intent"])
				break
			}
		}
		out = append(out, track)
	}
	return out
}

// isCaptionTrack is the predicate, kept next to readCaptions so the three
// ways of recognising one stay together.
func isCaptionTrack(source string) func(hfedit.Element) bool {
	return func(e hfedit.Element) bool {
		if _, has := e.Attrs["data-captions"]; has {
			return true
		}
		for _, c := range strings.Fields(e.Attrs["class"]) {
			if c == "caption-track" {
				return true
			}
		}
		return false
	}
}

// readCaptionWords reads the timed spans inside a track.
func readCaptionWords(source string, track hfedit.Element) []captionWord {
	words := make([]captionWord, 0, 12)
	for _, el := range hfedit.FindAll(source, func(e hfedit.Element) bool {
		if e.Start <= track.Start || e.End > track.End {
			return false
		}
		_, has := e.Attrs["data-start"]
		return has
	}) {
		text := strings.TrimSpace(collapseSpaces(innerText(source, el)))
		if text == "" {
			continue
		}
		w := captionWord{Text: text}
		w.StartMs, _ = msAttr(el, "data-start")
		w.DurationMs, _ = msAttr(el, "data-duration")
		words = append(words, w)
	}
	return words
}

// innerText is an element's content with tags taken out.
//
// Crude on purpose: a line of copy with a <br> or an <em> in it is still
// one line, and the script panel shows what it says rather than how it is
// marked up. An edit writes plain text back, which is the trade — a
// deliberate one, and the inspector is where the markup stays intact.
func innerText(source string, el hfedit.Element) string {
	if !el.HasChildren() {
		return ""
	}
	inner := source[el.TagEnd : el.End-len("</"+el.TagName+">")]
	var b strings.Builder
	depth := 0
	for i := 0; i < len(inner); i++ {
		switch inner[i] {
		case '<':
			depth++
		case '>':
			if depth > 0 {
				depth--
			}
		default:
			if depth == 0 {
				b.WriteByte(inner[i])
			}
		}
	}
	return b.String()
}

func collapseSpaces(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
