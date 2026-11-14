package hfproject

// What is on the timeline.
//
// ── Why this exists when Scene already did ───────────────────────────────
// `Scene` reads elements carrying `data-scene`, which is the spine of a film
// and nothing else. A reel with music, two voiceovers, a photograph and two
// caption tracks has one scene row and five invisible things, which is
// exactly how the timeline came to be "completely useless": it was drawing
// everything it had ever been given.
//
// ── The contract is theirs, not ours ─────────────────────────────────────
// Hyperframes' composition contract already names the attributes:
//
//	data-start         when it begins
//	data-duration      how long it lasts
//	data-track-index   WHICH ROW it sits on   ← we never wrote this
//	data-end           derived
//	data-layer         the older spelling of the track
//
// Their `timeline` command reads and mutates exactly that, and `Scene` has
// parsed `data-track-index` into `TrackIndex` since Phase 1. Every value is
// zero because no film we write sets it. So this is not a new model; it is
// finally reading the one that was always there, over every timed element
// rather than over scenes alone.
//
// ── Why a parser and not another regex ───────────────────────────────────
// Scene's regex scans start tags, which is survivable for one attribute on
// one kind of element. A clip needs the tag name, several attributes and the
// element's own words, and `pkg/hfedit` already tokenises the document
// properly for the inspector. Phase 13 settled this argument once: regex
// over HTML works for a month and then quietly corrupts someone's design.

import (
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/idivarts/backend-sls/pkg/hfedit"
)

// Clip is one thing on the timeline.
type Clip struct {
	// ElementID is how an edit addresses it. A clip without one cannot be
	// moved, so it is not reported.
	ElementID string `json:"elementId" firestore:"elementId"`

	// Kind is what it is, in the user's terms: scene, voice, music, sound,
	// image, video, captions.
	Kind string `json:"kind" firestore:"kind"`

	// TrackIndex is the row. Authored where the film says so, inferred from
	// the kind where it does not — see trackFor.
	TrackIndex int `json:"trackIndex" firestore:"trackIndex"`

	// Authored records whether the film SAID which row this is on, so the
	// app can tell a deliberate layout from a guess, and so a later pass
	// can stamp the guesses in rather than re-deriving them for ever.
	Authored bool `json:"authored,omitempty" firestore:"authored,omitempty"`

	StartMs    int `json:"startMs" firestore:"startMs"`
	DurationMs int `json:"durationMs" firestore:"durationMs"`

	// Label is what the row shows: a scene's name, a file's basename, the
	// first few words of a line.
	Label string `json:"label,omitempty" firestore:"label,omitempty"`

	// Src is the file behind it, where it has one.
	Src string `json:"src,omitempty" firestore:"src,omitempty"`
}

// Track kinds, in the order they are drawn. The order is the point: the
// spine on top, then what you see, then what you hear, then the words over
// the top.
const (
	ClipScene    = "scene"
	ClipVideo    = "video"
	ClipImage    = "image"
	ClipVoice    = "voice"
	ClipMusic    = "music"
	ClipSound    = "sound"
	ClipCaptions = "captions"
)

// defaultTrack is the row a clip sits on when the film does not say.
//
// ── The read-time fallback the plan asked for ────────────────────────────
// Authoring will set data-track-index from now on, but every film written
// before today has none, and a timeline that drew all of them on row 0
// would be the same useless timeline with extra steps. So a kind implies a
// row, and an authored value overrides it.
var defaultTrack = map[string]int{
	ClipScene:    0,
	ClipVideo:    1,
	ClipImage:    1,
	ClipVoice:    2,
	ClipMusic:    3,
	ClipSound:    3,
	ClipCaptions: 4,
}

// DefaultTrackFor is the row a kind of media belongs on.
//
// Exported because whatever WRITES media into a film should put it on the
// same row this would have inferred for it — otherwise the attribute and
// the fallback disagree, and a clip jumps rows the first time someone
// edits the film.
//
// The argument is a media-use type (image, icon, logo, bgm, sfx, voice) or
// a clip kind; anything unknown lands on the media row, which is the one a
// person expects a new picture on.
func DefaultTrackFor(kind string) int {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "voice":
		return defaultTrack[ClipVoice]
	case "bgm", "music":
		return defaultTrack[ClipMusic]
	case "sfx", "sound":
		return defaultTrack[ClipSound]
	case ClipScene:
		return defaultTrack[ClipScene]
	case ClipCaptions:
		return defaultTrack[ClipCaptions]
	}
	return defaultTrack[ClipImage]
}

// maxClips bounds what one film reports. A timeline with more rows than
// this is not a timeline anyone reads, and the document it is stored on has
// a size limit.
const maxClips = 200

// maxLabel keeps a row's name to something that fits on it.
const maxLabel = 48

// Clips reads every timed thing in the film, in the order it plays.
//
// ── What is deliberately NOT a clip ──────────────────────────────────────
// The headlines and paragraphs inside a scene. They are timed, they are
// addressable, and putting each on the timeline would give a three-scene
// reel thirty rows. The layer tree already shows them, which is the right
// place for structure inside a scene; the timeline is for the things a
// person drags around — the scenes, the media and the captions.
func Clips(index string) []Clip {
	out := make([]Clip, 0, 8)

	for _, el := range hfedit.FindAll(index, func(e hfedit.Element) bool {
		if e.Attrs["id"] == "" {
			return false
		}
		if _, isScene := e.Attrs["data-scene"]; isScene {
			return true
		}
		if _, isRoot := e.Attrs["data-composition-id"]; isRoot {
			return false
		}
		return clipKind(e) != ""
	}) {
		kind := clipKind(el)
		if kind == "" {
			continue
		}
		track, authored := trackFor(el, kind)
		c := Clip{
			ElementID:  el.Attrs["id"],
			Kind:       kind,
			TrackIndex: track,
			Authored:   authored,
			StartMs:    msOf(el.Attrs["data-start"]),
			DurationMs: durationOf(el),
			Src:        strings.TrimSpace(el.Attrs["src"]),
		}
		c.Label = labelFor(index, el, kind, c.Src)
		out = append(out, c)
		if len(out) >= maxClips {
			break
		}
	}

	// By track, then by time: the order a row is drawn in, so the app can
	// walk the list once.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].TrackIndex != out[j].TrackIndex {
			return out[i].TrackIndex < out[j].TrackIndex
		}
		return out[i].StartMs < out[j].StartMs
	})
	return out
}

// clipKind says what an element is, or "" when it is not a clip.
func clipKind(el hfedit.Element) string {
	if _, isScene := el.Attrs["data-scene"]; isScene {
		return ClipScene
	}
	for _, c := range strings.Fields(el.Attrs["class"]) {
		if c == "caption-track" {
			return ClipCaptions
		}
	}
	if _, isCaptions := el.Attrs["data-captions"]; isCaptions {
		return ClipCaptions
	}

	// A media element says what it is twice: by tag, and — while it is
	// still a declaration — by data-media. The declaration is the better
	// answer, because an <audio> is music or a voiceover and the tag
	// cannot tell you which.
	switch strings.ToLower(strings.TrimSpace(el.Attrs["data-media"])) {
	case "voice":
		return ClipVoice
	case "bgm":
		return ClipMusic
	case "sfx":
		return ClipSound
	case "image", "icon", "logo":
		return ClipImage
	}

	switch strings.ToLower(el.TagName) {
	case "audio":
		// A resolved voiceover keeps data-media="voice", so anything left
		// here is a track rather than a line. Checked after the
		// declaration above for exactly that reason.
		return ClipMusic
	case "video":
		return ClipVideo
	case "img":
		return ClipImage
	}
	return ""
}

// durationOf is how long a clip lasts.
//
// data-duration where the film gives one, and data-end minus data-start
// where it gives that instead — both are in the composition contract, and a
// clip reported as zero-length draws as a bar with no width, which looks
// like a bug in the timeline rather than in the film.
func durationOf(el hfedit.Element) int {
	if d := msOf(el.Attrs["data-duration"]); d > 0 {
		return d
	}
	if end := msOf(el.Attrs["data-end"]); end > 0 {
		if d := end - msOf(el.Attrs["data-start"]); d > 0 {
			return d
		}
	}
	return 0
}

// trackFor returns the row and whether the film said so.
func trackFor(el hfedit.Element, kind string) (int, bool) {
	for _, name := range []string{"data-track-index", "data-layer"} {
		raw := strings.TrimSpace(el.Attrs[name])
		if raw == "" {
			continue
		}
		if n, err := strconv.Atoi(raw); err == nil && n >= 0 {
			return n, true
		}
	}
	return defaultTrack[kind], false
}

// labelFor is what the clip's row shows.
func labelFor(index string, el hfedit.Element, kind, src string) string {
	// A scene is known by the name its author gave it.
	if kind == ClipScene {
		if n := strings.TrimSpace(el.Attrs["data-scene"]); n != "" {
			return shorten(n)
		}
		return shorten(el.Attrs["id"])
	}
	// A voiceover is known by what it says, which is the one label a
	// person can tell two of them apart by.
	if words := strings.TrimSpace(el.Attrs["data-media-intent"]); words != "" {
		return shorten(words)
	}
	// Then the file, which is at least real.
	if src != "" {
		return shorten(path.Base(src))
	}
	// A caption track is known by its words.
	if kind == ClipCaptions {
		if text := strings.TrimSpace(collapse(innerTextOf(index, el))); text != "" {
			return shorten(text)
		}
	}
	return shorten(el.Attrs["id"])
}

// innerTextOf is the element's content with its tags removed.
func innerTextOf(index string, el hfedit.Element) string {
	if !el.HasChildren() || el.End > len(index) {
		return ""
	}
	inner := index[el.TagEnd : el.End-len("</"+el.TagName+">")]
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

func collapse(s string) string { return strings.Join(strings.Fields(s), " ") }

func shorten(s string) string {
	s = collapse(s)
	if len(s) <= maxLabel {
		return s
	}
	return s[:maxLabel-1] + "…"
}

// msOf reads a seconds attribute as milliseconds.
func msOf(raw string) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0
	}
	return int(f*1000 + 0.5)
}
