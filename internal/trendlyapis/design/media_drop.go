package design

// Putting media AT A MOMENT rather than into a scene.
//
// ── Why this is not the same code path ────────────────────────────────────
// Placing media in a scene appends it inside that scene, so it inherits the
// scene's window and moves with it — right for a photograph that belongs to
// a shot. Dropping a file on a row at 0:06, or asking for a sound at 0:06,
// means something else: it belongs to the FILM at that moment, and a scene
// would clip anything timed outside its own window.
//
// That is the quietest failure this studio has. The film still renders,
// nothing errors, and the thing a person just dropped is not in it. So a
// drop goes at the root, whose window is the whole film.

import (
	"fmt"
	"path"
	"strings"

	"github.com/idivarts/backend-sls/pkg/hfedit"
	"github.com/idivarts/backend-sls/pkg/hfproject"
)

// placeMediaAtMoment adds one media element to the root of the film at the
// time it was dropped, and returns the document and the line the revisions
// list shows.
func placeMediaAtMoment(source string, req placeMediaRequest, kind, assetPath, intent string) (string, string, error) {
	root, ok := rootOf(source)
	if !ok {
		return "", "", fmt.Errorf("this film has no root composition to put it in")
	}
	filmMs := clipDurationOf(root)

	startMs := *req.StartMs
	if startMs < 0 {
		startMs = 0
	}
	// Clamped, not refused: a drop at the very end of the row is a drop at
	// the end of the film, and an error there would be pedantry about a
	// gesture that was perfectly clear.
	if filmMs > 0 && startMs > filmMs-minClipMs {
		startMs = filmMs - minClipMs
		if startMs < 0 {
			startMs = 0
		}
	}

	durationMs := defaultDropMs
	if req.DurationMs != nil {
		durationMs = *req.DurationMs
	}
	if durationMs < minClipMs {
		return "", "", fmt.Errorf("a clip has to last at least %.1f seconds", float64(minClipMs)/1000)
	}
	// Never past the end of the film, which the renderer would cut anyway —
	// but a clip that LOOKS longer than the film draws a bar wider than its
	// row and pushes the whole timeline out of alignment.
	if filmMs > 0 && startMs+durationMs > filmMs {
		durationMs = filmMs - startMs
	}

	track := hfproject.DefaultTrackFor(kind)
	if req.TrackIndex != nil {
		if *req.TrackIndex < 0 || *req.TrackIndex > maxTrackIndex {
			return "", "", fmt.Errorf("there is no row %d on this timeline", *req.TrackIndex)
		}
		track = *req.TrackIndex
	}

	id := freeMediaID(source, kind)
	tag := mediaTag[kind]

	var b strings.Builder
	fmt.Fprintf(&b, "\n  <%s id=%q", tag, id)
	fmt.Fprintf(&b, ` data-start=%q data-duration=%q`, secondsAttr(startMs), secondsAttr(durationMs))
	fmt.Fprintf(&b, ` data-track-index="%d"`, track)
	if tag == "img" {
		// Sized to the frame rather than left at its natural size — the
		// same reasoning as placing one in a scene, where a 4000px stock
		// photograph became a film with one corner of a photograph in it.
		// Positioned too, because at the root there is no scene's own
		// layout to sit inside.
		b.WriteString(` class="clip" style="position:absolute;inset:0;width:100%;height:100%;object-fit:cover"`)
	} else {
		b.WriteString(` class="clip"`)
	}
	if assetPath != "" {
		// data-media as well as the file, so the document says what this
		// is — without it an uploaded voiceover reads back as music.
		fmt.Fprintf(&b, ` data-media=%q src=%q data-media-resolved=%q`, kind, assetPath, assetPath)
	} else {
		fmt.Fprintf(&b, ` data-media=%q data-media-intent=%q`, kind, intent)
	}
	if tag == "img" {
		b.WriteString(" />")
	} else {
		fmt.Fprintf(&b, "></%s>", tag)
	}

	at := root.End - len("</"+root.TagName+">")
	out, aerr := hfedit.Apply(source, []hfedit.Edit{{Start: at, End: at, Text: b.String() + "\n"}})
	if aerr != nil {
		return "", "", aerr
	}

	what := intent
	if assetPath != "" {
		what = path.Base(assetPath)
	}
	// Named by WHEN rather than by which scene, because that is what the
	// person chose — they dropped it at a moment on a row.
	return out, fmt.Sprintf("%s · at %s", shortened(what, 40), clockOf(startMs)), nil
}

// clockOf is "0:06", for the revisions list.
func clockOf(ms int) string {
	if ms < 0 {
		ms = 0
	}
	total := ms / 1000
	return fmt.Sprintf("%d:%02d", total/60, total%60)
}
