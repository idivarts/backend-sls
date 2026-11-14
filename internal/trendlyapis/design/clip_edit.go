package design

// Moving ONE thing on the timeline, without moving the film.
//
// ── Why this is not a scene op ────────────────────────────────────────────
// Scenes play one after another, so every scene op RIPPLES: lengthen the
// first and everything after it moves later (structure.go states that rule
// and shows it). A photograph, a voiceover or a music track is the opposite
// — it sits at a moment on a row, and dragging it must move exactly it.
// Running a drag through the scene machinery would have relaid the whole
// film every time someone nudged the music two seconds later, which is the
// one thing the phase's own test says must not happen: "with the scenes
// untouched".
//
// So a clip op writes attributes on one element and nothing else.
//
// ── The trap that makes this more than SetAttribute ───────────────────────
// A timed container CLIPS its children: "The container is itself a timed
// element, so words timed past its window are never drawn." Media placed
// from the panel is appended INSIDE a scene, so dragging that photograph
// from the first scene to the third does not move it — it makes it
// invisible, in a film that still renders, with nothing anywhere saying
// why. Which is the worst failure mode this studio has: everything says the
// edit worked.
//
// So when a clip's new window escapes the scene holding it, the element is
// MOVED OUT to the root composition, whose window is the whole film. It
// then plays where it was dropped, which is what dragging it meant.

import (
	"fmt"
	"strings"

	"github.com/idivarts/backend-sls/pkg/hfedit"
)

// Clip operations, as the app names them. Prefixed so no caller can confuse
// a clip retime (one element moves) with a scene retime (the film relays).
const (
	clipOpRetime = "clip-retime"
	clipOpTrack  = "clip-track"
	clipOpDelete = "clip-delete"
)

// isClipOp reports whether this request is about one clip rather than the
// film's shape.
func isClipOp(op string) bool {
	switch op {
	case clipOpRetime, clipOpTrack, clipOpDelete:
		return true
	}
	return false
}

// minClipMs is the shortest a clip may be made by dragging.
//
// Shorter than a scene's half-second floor on purpose: a sound effect of a
// tenth of a second is a real thing a film wants, where a scene that short
// is a scene nobody can read. Zero is still refused — a clip with no length
// is a row on the timeline the renderer draws nothing of.
const minClipMs = 100

// maxTrackIndex bounds the rows. Five are named in the authoring contract;
// a few spare let a film separate two music beds without letting a bad
// request put a clip on row 900, which would draw 900 empty rows above it.
const maxTrackIndex = 11

// applyClipEdit returns the document with one clip changed, and the film's
// length — which a clip op never alters, so it comes back as the root says
// it is.
func applyClipEdit(source string, req structureRequest) (string, int, error) {
	// The film's own length, unchanged by anything here. Read before the
	// edit so a failure cannot report a length from a half-applied change.
	filmMs := rootDurationOf(source)

	key := strings.TrimSpace(req.ClipID)
	if key == "" {
		key = strings.TrimSpace(req.SceneID)
	}
	if key == "" {
		return "", 0, fmt.Errorf("say which clip to change")
	}
	el, ok := hfedit.FindByID(source, key)
	if !ok {
		return "", 0, fmt.Errorf("that is no longer in this film")
	}
	// ── Two things a clip op must never touch ────────────────────────────
	// A scene, because scenes ripple and this does not — a scene dragged
	// through here would overlap the one after it and the film would play
	// two at once. And the root, because the root IS the film: setting its
	// start would move every frame of it at once.
	if _, isScene := el.Attrs["data-scene"]; isScene {
		return "", 0, fmt.Errorf("that is a scene — scenes are moved in the scene panel, where the film relays around them")
	}
	if _, isRoot := el.Attrs["data-composition-id"]; isRoot {
		return "", 0, fmt.Errorf("that is the film itself, not something on it")
	}

	if req.Op == clipOpDelete {
		return deleteClip(source, el, filmMs)
	}

	// Where it is now, so a request that says only one of the two keeps the
	// other rather than resetting it to zero.
	startMs := msAttrOr(el, "data-start", 0)
	durationMs := clipDurationOf(el)

	var edits []hfedit.Edit

	switch req.Op {
	case clipOpTrack:
		if req.TrackIndex == nil {
			return "", 0, fmt.Errorf("say which row it should go on")
		}
		if *req.TrackIndex < 0 || *req.TrackIndex > maxTrackIndex {
			return "", 0, fmt.Errorf("there is no row %d on this timeline", *req.TrackIndex)
		}
		e, err := hfedit.SetAttribute(el, source, "data-track-index", fmt.Sprintf("%d", *req.TrackIndex))
		if err != nil {
			return "", 0, err
		}
		// A row change moves nothing in time, so it cannot escape its
		// scene — written and done.
		out, aerr := hfedit.Apply(source, []hfedit.Edit{e})
		return out, filmMs, aerr

	case clipOpRetime:
		if req.StartMs == nil && req.DurationMs == nil {
			return "", 0, fmt.Errorf("say where it should start, or how long it should last")
		}
		if req.StartMs != nil {
			startMs = *req.StartMs
		}
		if req.DurationMs != nil {
			durationMs = *req.DurationMs
		}
		if durationMs < minClipMs {
			return "", 0, fmt.Errorf("a clip has to last at least %.1f seconds", float64(minClipMs)/1000)
		}
		// ── Clamped rather than refused ───────────────────────────────────
		// Dragging is imprecise by nature and a drag that ends slightly
		// past the end of the film is a drag to the end of the film, not a
		// mistake worth an error message. Negative likewise: it means the
		// start.
		if startMs < 0 {
			startMs = 0
		}
		if filmMs > 0 && startMs > filmMs-minClipMs {
			startMs = filmMs - minClipMs
			if startMs < 0 {
				startMs = 0
			}
		}
		for _, set := range [][2]string{
			{"data-start", secondsAttr(startMs)},
			{"data-duration", secondsAttr(durationMs)},
		} {
			e, err := hfedit.SetAttribute(el, source, set[0], set[1])
			if err != nil {
				return "", 0, err
			}
			edits = append(edits, e)
		}
		// ⚠️ data-end is DERIVED, and a stale one wins over the duration we
		// just wrote in some readers. Cleared to the new end rather than
		// left to contradict it.
		if _, has := el.Attrs["data-end"]; has {
			e, err := hfedit.SetAttribute(el, source, "data-end", secondsAttr(startMs+durationMs))
			if err != nil {
				return "", 0, err
			}
			edits = append(edits, e)
		}

	default:
		return "", 0, fmt.Errorf("%q is not something that can be done to a clip", req.Op)
	}

	// Does the new window still fit inside whatever holds it?
	holder, held := timedHolderOf(source, el)
	if held && !windowFits(startMs, durationMs, holder) {
		out, err := liftToRoot(source, el, edits)
		return out, filmMs, err
	}

	out, aerr := hfedit.Apply(source, edits)
	return out, filmMs, aerr
}

// deleteClip removes one element and nothing else.
func deleteClip(source string, el hfedit.Element, filmMs int) (string, int, error) {
	out, err := hfedit.Apply(source, []hfedit.Edit{{Start: el.Start, End: el.End, Text: ""}})
	return out, filmMs, err
}

// timedHolderOf is the innermost TIMED element containing this one.
//
// Timed, not merely containing: an untimed <div> wrapping a photograph
// clips nothing, so lifting out of it would be markup churn for no reason.
// Returns false when nothing times it — the element already answers to the
// film's own window.
func timedHolderOf(source string, el hfedit.Element) (hfedit.Element, bool) {
	var best hfedit.Element
	found := false
	for _, c := range hfedit.FindAll(source, func(e hfedit.Element) bool { return true }) {
		if c.Start >= el.Start || c.End <= el.End {
			continue // not a strict container
		}
		if _, isRoot := c.Attrs["data-composition-id"]; isRoot {
			continue // the root's window IS the film; it clips nothing off
		}
		if _, timed := c.Attrs["data-start"]; !timed {
			if _, d := c.Attrs["data-duration"]; !d {
				continue
			}
		}
		// Innermost wins: the nearest window is the one that clips first.
		if !found || c.Start > best.Start {
			best, found = c, true
		}
	}
	return best, found
}

// windowFits reports whether a clip's window is inside its holder's.
func windowFits(startMs, durationMs int, holder hfedit.Element) bool {
	hs := msAttrOr(holder, "data-start", 0)
	hd := clipDurationOf(holder)
	if hd <= 0 {
		return true // a holder with no length clips nothing we can reason about
	}
	return startMs >= hs && startMs+durationMs <= hs+hd
}

// liftToRoot moves an element out of whatever was clipping it, to the end
// of the root composition, carrying the attribute edits with it.
//
// ── Why the markup moves and not just the clock ───────────────────────────
// Leaving it where it is and writing the new time would produce a film that
// renders perfectly and does not contain the thing the user just dragged.
// An error message was the other option; it would be honest and it would
// also mean the timeline refuses the most ordinary drag there is — a
// photograph from the first scene to the third.
func liftToRoot(source string, el hfedit.Element, edits []hfedit.Edit) (string, error) {
	root, ok := rootOf(source)
	if !ok {
		// No root to lift into. The edit in place is still better than an
		// error: the clip keeps its old window and the user sees it did
		// not move, rather than seeing nothing happen at all.
		return hfedit.Apply(source, edits)
	}
	if el.Start <= root.TagEnd && el.End >= root.End {
		return hfedit.Apply(source, edits) // the root itself; already guarded, belt and braces
	}

	// The element's own markup, with this request's attribute changes
	// applied to it in isolation. Done on the substring because the two
	// halves of a move — a deletion here, an insertion there — must not
	// overlap any other edit when Apply lays them down.
	fragment := source[el.Start:el.End]
	if _, found := hfedit.FindByID(fragment, el.Attrs["id"]); found {
		shifted := make([]hfedit.Edit, 0, len(edits))
		for _, e := range edits {
			if e.Start < el.Start || e.End > el.End {
				continue
			}
			shifted = append(shifted, hfedit.Edit{Start: e.Start - el.Start, End: e.End - el.Start, Text: e.Text})
		}
		moved, err := hfedit.Apply(fragment, shifted)
		if err != nil {
			return "", err
		}
		fragment = moved
	}

	at := root.End - len("</"+root.TagName+">")
	if at < el.End {
		// The root closes before this element ends, which cannot happen in
		// a well-formed document. Rather than write an insertion that
		// overlaps the deletion, leave it where it is.
		return hfedit.Apply(source, edits)
	}
	return hfedit.Apply(source, []hfedit.Edit{
		{Start: el.Start, End: el.End, Text: ""},
		{Start: at, End: at, Text: "\n  " + strings.TrimSpace(fragment) + "\n"},
	})
}

// rootOf is the root composition element.
func rootOf(source string) (hfedit.Element, bool) {
	for _, e := range hfedit.FindAll(source, func(e hfedit.Element) bool {
		_, is := e.Attrs["data-composition-id"]
		return is
	}) {
		return e, true
	}
	return hfedit.Element{}, false
}

// rootDurationOf is the film's length in milliseconds, or 0 when the
// document does not say.
func rootDurationOf(source string) int {
	root, ok := rootOf(source)
	if !ok {
		return 0
	}
	return clipDurationOf(root)
}

// clipDurationOf is how long an element lasts: data-duration, or data-end
// minus data-start. Both are in the composition contract, and hfproject
// reads both — see hfproject.Clips.
func clipDurationOf(el hfedit.Element) int {
	if d := msAttrOr(el, "data-duration", 0); d > 0 {
		return d
	}
	if end := msAttrOr(el, "data-end", 0); end > 0 {
		if d := end - msAttrOr(el, "data-start", 0); d > 0 {
			return d
		}
	}
	return 0
}

// msAttrOr reads a seconds attribute as milliseconds.
func msAttrOr(el hfedit.Element, name string, fallback int) int {
	if v, ok := msAttr(el, name); ok {
		return v
	}
	return fallback
}

// clipNote is the line the revisions list shows.
func clipNote(req structureRequest) string {
	what := strings.TrimSpace(req.ClipID)
	if what == "" {
		what = strings.TrimSpace(req.SceneID)
	}
	switch req.Op {
	case clipOpRetime:
		if req.StartMs != nil && req.DurationMs != nil {
			return "Moved and trimmed " + what
		}
		if req.DurationMs != nil {
			return "Trimmed " + what
		}
		return "Moved " + what
	case clipOpTrack:
		return "Moved " + what + " to another row"
	case clipOpDelete:
		return "Removed " + what
	}
	return "Changed " + what
}
