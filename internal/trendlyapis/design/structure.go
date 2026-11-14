package design

// Changing the film's SHAPE rather than its contents.
//
// ── The rule the plan said to state, and show ─────────────────────────────
// Phase 15: "A stated rule for what happens to the scenes after the one you
// moved — ripple, or leave a gap. Whichever we choose, it is written down
// here and the UI shows it, because guessing differently from the user is
// how an editor feels broken."
//
//	SCENES PLAY ONE AFTER ANOTHER, IN THE ORDER THEY APPEAR.
//
// So every structural change RIPPLES: the scenes are laid back-to-back in
// document order and the film's length is their total. Lengthen the first
// scene and everything after it moves later; delete one and everything
// after it moves earlier; reorder them and the clock follows the new order.
//
// Chosen over "leave a gap" because a gap in one of these films is dead
// air — a few seconds of background with nothing on it — and nobody drags a
// scene in order to get that. The cost is that a film which deliberately
// overlaps two scenes cannot be built this way; nothing authors one today,
// and when something does, this is the rule to revisit.
//
// ── Why a scene's children move with it ───────────────────────────────────
// Timing in these films is ABSOLUTE: data-start is seconds from the start
// of the film, not from the start of the scene. So moving a scene from 3s
// to 6s and leaving its headline at 3.2s would leave the headline behind,
// on screen during the scene before it. Every timed descendant shifts by
// the same delta as its scene.

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/idivarts/backend-sls/pkg/hfedit"
)

// Structural operations, as the app names them.
const (
	sceneOpRetime    = "retime"
	sceneOpMove      = "move"
	sceneOpDuplicate = "duplicate"
	sceneOpDelete    = "delete"
	sceneOpAdd       = "add"
)

type structureRequest struct {
	// Op is one of the sceneOp* constants.
	Op string `json:"op" binding:"required"`
	// SceneID is the element id of the scene acted on. Not needed by "add".
	SceneID string `json:"sceneId"`
	// DurationMs is the new length, for "retime".
	DurationMs *int `json:"durationMs"`
	// ToIndex is where the scene goes, for "move": 0 is first.
	ToIndex *int `json:"toIndex"`
}

// minSceneMs is the shortest a scene may be. Below about half a second
// nothing on it can be read, and a zero-length scene is a scene the
// renderer draws no frames of — which looks like a deleted one with a row
// still on the timeline.
const minSceneMs = 500

// maxScenes bounds what one film may hold. A reel with thirty scenes is a
// mistake, and duplicate is one click.
const maxScenes = 24

// scene is one top-level scene found in the document.
type scene struct {
	el hfedit.Element
	// id is the element's id attribute — what an edit addresses.
	id string
	// name is its data-scene value, which is what the app calls it.
	name       string
	startMs    int
	durationMs int
}

// matches reports whether this is the scene the caller means.
//
// ── Two names for one thing, and the app only knows one ──────────────────
// A scene is written `<section id="scene-coldbrew" data-scene="coldbrew">`,
// and hfproject keys it by the data-scene value — so the scene panel, the
// timeline and every click in the app say "coldbrew" while the document
// says "scene-coldbrew". Matching only the element id meant every button
// in that panel answered "that scene is no longer in this film", which is
// exactly how it read on dev: duplicate, and nothing happens but an error
// about a scene plainly on screen.
func (s scene) matches(key string) bool {
	return key != "" && (key == s.id || key == s.name)
}

// sceneIndex finds the scene a caller means, or -1.
//
// An element id wins over a data-scene name, because an id is unique and
// a name need not be: duplicating a scene gives the copy its own name
// (see duplicateScene) precisely so this stays unambiguous, but a film
// may have been written with two scenes called the same thing, and then
// the id is the only thing that can tell them apart.
func sceneIndex(scenes []scene, key string) int {
	if key == "" {
		return -1
	}
	for i, s := range scenes {
		if s.id == key {
			return i
		}
	}
	for i, s := range scenes {
		if s.name == key {
			return i
		}
	}
	return -1
}

// applyStructureEdit returns the document with the change made, and the
// film's new length.
//
// Separated from the handler so the rules can be tested against real
// markup without a bucket, a session or a design — which is where the
// mistakes in this kind of code live.
func applyStructureEdit(source string, req structureRequest) (string, int, error) {
	scenes, err := readScenes(source)
	if err != nil {
		return "", 0, err
	}

	// The op is applied to the ORDER and the LENGTHS only. Where each scene
	// then sits is recomputed from scratch afterwards, which is what makes
	// the ripple rule a single piece of code rather than four.
	order := make([]int, len(scenes))
	for i := range scenes {
		order[i] = i
	}
	durations := make([]int, len(scenes))
	for i, s := range scenes {
		durations[i] = s.durationMs
	}

	at := sceneIndex(scenes, req.SceneID)

	var extra []hfedit.Edit // insertions and removals of whole markup
	switch req.Op {
	case sceneOpRetime:
		if at < 0 {
			return "", 0, fmt.Errorf("that scene is no longer in this film")
		}
		if req.DurationMs == nil {
			return "", 0, fmt.Errorf("say how long it should be")
		}
		if *req.DurationMs < minSceneMs {
			return "", 0, fmt.Errorf("a scene has to last at least %.1f seconds", float64(minSceneMs)/1000)
		}
		durations[at] = *req.DurationMs

	case sceneOpMove:
		if at < 0 {
			return "", 0, fmt.Errorf("that scene is no longer in this film")
		}
		if req.ToIndex == nil {
			return "", 0, fmt.Errorf("say where it should go")
		}
		to := *req.ToIndex
		if to < 0 || to >= len(scenes) {
			return "", 0, fmt.Errorf("there is no position %d in this film", to)
		}
		if to == at {
			return "", 0, fmt.Errorf("that scene is already there")
		}
		// ── The markup moves, not just the clock ─────────────────────────
		// Rewriting only data-start would leave the DOCUMENT in the old
		// order while the film played in the new one. Everything that
		// reads the project — the scene panel, the timeline, the next
		// edit, a person opening index.html — would then disagree with
		// the film, which is precisely the "editor feels broken" this
		// phase is supposed to avoid.
		markup := source[scenes[at].el.Start:scenes[at].el.End]
		anchor := scenes[to]
		insertAt := anchor.el.Start
		if to > at {
			insertAt = anchor.el.End
		}
		extra = []hfedit.Edit{
			{Start: scenes[at].el.Start, End: scenes[at].el.End, Text: ""},
			{Start: insertAt, End: insertAt, Text: markup},
		}
		if insertAt > scenes[at].el.Start && insertAt < scenes[at].el.End {
			return "", 0, fmt.Errorf("a scene cannot be moved inside itself")
		}

	case sceneOpDelete:
		if at < 0 {
			return "", 0, fmt.Errorf("that scene is no longer in this film")
		}
		if len(scenes) <= 1 {
			return "", 0, fmt.Errorf("a film needs at least one scene")
		}
		extra = []hfedit.Edit{{Start: scenes[at].el.Start, End: scenes[at].el.End, Text: ""}}

	case sceneOpDuplicate:
		if at < 0 {
			return "", 0, fmt.Errorf("that scene is no longer in this film")
		}
		if len(scenes) >= maxScenes {
			return "", 0, fmt.Errorf("a film can hold %d scenes", maxScenes)
		}
		copyMarkup, copyID, derr := duplicateScene(source, scenes[at], scenes)
		if derr != nil {
			return "", 0, derr
		}
		// Inserted right after the original, which is where a duplicate
		// belongs and what every editor does.
		extra = []hfedit.Edit{{Start: scenes[at].el.End, End: scenes[at].el.End, Text: "\n" + copyMarkup}}
		_ = copyID // named for the error messages; the layout pass finds it by reading the document again

	case sceneOpAdd:
		if len(scenes) >= maxScenes {
			return "", 0, fmt.Errorf("a film can hold %d scenes", maxScenes)
		}
		markup, aerr := emptyScene(source, scenes)
		if aerr != nil {
			return "", 0, aerr
		}
		last := scenes[len(scenes)-1]
		extra = []hfedit.Edit{{Start: last.el.End, End: last.el.End, Text: "\n" + markup}}

	default:
		return "", 0, fmt.Errorf("%q is not something that can be done to a scene", req.Op)
	}

	// ── Structure first, clock second ────────────────────────────────────
	// Adding, duplicating and deleting change what scenes EXIST, and the
	// offsets of everything after them. Doing that in one pass with the
	// retiming would mean splicing against offsets that have already moved,
	// so the markup change is applied on its own and the document is read
	// again before the clock is written.
	if len(extra) > 0 {
		moved, aerr := hfedit.Apply(source, extra)
		if aerr != nil {
			return "", 0, aerr
		}
		source = moved
		scenes, err = readScenes(source)
		if err != nil {
			return "", 0, err
		}
		order = make([]int, len(scenes))
		for i := range scenes {
			order[i] = i
		}
		durations = make([]int, len(scenes))
		for i, s := range scenes {
			durations[i] = s.durationMs
		}
		if req.Op == sceneOpRetime && at >= 0 && at < len(durations) {
			durations[at] = *req.DurationMs
		}
	}

	return layOutScenes(source, scenes, order, durations)
}

// layOutScenes writes the clock: every scene back-to-back in document
// order, every timed descendant shifted with its scene, and the root's
// duration set to the total.
//
// `order` is the sequence to lay out in, which after a markup change is
// simply document order — the two passes are separate because splicing
// markup and retiming it together would mean offsets from two different
// versions of the document.
func layOutScenes(source string, scenes []scene, order, durations []int) (string, int, error) {
	starts := make([]int, len(scenes))
	clock := 0
	for _, idx := range order {
		starts[idx] = clock
		clock += durations[idx]
	}

	var edits []hfedit.Edit
	for i, s := range scenes {
		delta := starts[i] - s.startMs
		if delta != 0 {
			e, err := hfedit.SetAttribute(s.el, source, "data-start", secondsAttr(starts[i]))
			if err != nil {
				return "", 0, err
			}
			edits = append(edits, e)
		}
		if durations[i] != s.durationMs {
			e, err := hfedit.SetAttribute(s.el, source, "data-duration", secondsAttr(durations[i]))
			if err != nil {
				return "", 0, err
			}
			edits = append(edits, e)
		}
		if delta == 0 {
			continue
		}
		// Everything timed INSIDE the scene moves with it, because these
		// films time from the start of the FILM and not of the scene.
		for _, child := range hfedit.FindAll(source, func(e hfedit.Element) bool {
			if e.Start <= s.el.Start || e.Start >= s.el.End {
				return false
			}
			_, has := e.Attrs["data-start"]
			return has
		}) {
			was, ok := msAttr(child, "data-start")
			if !ok {
				continue
			}
			e, err := hfedit.SetAttribute(child, source, "data-start", secondsAttr(maxInt(0, was+delta)))
			if err != nil {
				return "", 0, err
			}
			edits = append(edits, e)
		}
	}

	// The root's duration is what the renderer cuts to and what the wallet
	// is priced from, so it is not allowed to drift from the scenes.
	roots := hfedit.FindAll(source, func(e hfedit.Element) bool {
		_, has := e.Attrs["data-composition-id"]
		return has
	})
	if len(roots) == 0 {
		return "", 0, fmt.Errorf("this film has no root composition")
	}
	e, err := hfedit.SetAttribute(roots[0], source, "data-duration", secondsAttr(clock))
	if err != nil {
		return "", 0, err
	}
	edits = append(edits, e)

	out, aerr := hfedit.Apply(source, edits)
	if aerr != nil {
		return "", 0, aerr
	}
	return out, clock, nil
}

// readScenes finds the film's top-level scenes, in document order.
//
// Top-level only: a [data-scene] nested inside another is part of its
// parent, not a scene of the film, and treating it as one would put a row
// on the timeline for something that cannot be moved independently.
func readScenes(source string) ([]scene, error) {
	found := hfedit.FindAll(source, func(e hfedit.Element) bool {
		_, has := e.Attrs["data-scene"]
		return has
	})
	var out []scene
	for _, el := range found {
		nested := false
		for _, other := range found {
			if other.Start < el.Start && other.End >= el.End {
				nested = true
				break
			}
		}
		if nested {
			continue
		}
		id := el.Attrs["id"]
		if id == "" {
			return nil, fmt.Errorf("a scene in this film has no id, so it cannot be moved")
		}
		start, _ := msAttr(el, "data-start")
		dur, ok := msAttr(el, "data-duration")
		if !ok || dur <= 0 {
			return nil, fmt.Errorf("scene %q does not say how long it lasts", id)
		}
		out = append(out, scene{
			el: el, id: id, name: strings.TrimSpace(el.Attrs["data-scene"]),
			startMs: start, durationMs: dur,
		})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("this film has no scenes to change")
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].el.Start < out[j].el.Start })
	return out, nil
}

// duplicateScene copies a scene's markup, giving every id inside it a new
// one.
//
// Every id, not just the scene's: two elements sharing an id make the
// selection bridge, the inspector and every later edit ambiguous, and the
// renderer warns about it. The suffix is the lowest number that collides
// with nothing in the document.
func duplicateScene(source string, s scene, all []scene) (string, string, error) {
	markup := source[s.el.Start:s.el.End]
	ids := hfedit.FindAll(markup, func(e hfedit.Element) bool { return e.Attrs["id"] != "" })

	suffix, serr := freeSuffix(source, ids)
	if serr != nil {
		return "", "", serr
	}

	var edits []hfedit.Edit
	for _, el := range ids {
		e, err := hfedit.SetAttribute(el, markup, "id", el.Attrs["id"]+suffix)
		if err != nil {
			return "", "", err
		}
		edits = append(edits, e)
	}
	// ── And its own NAME ─────────────────────────────────────────────────
	// The app addresses a scene by its data-scene value, so a copy that
	// kept the original's name would give the panel two rows called
	// "coldbrew" and no way to say which one a button meant — pressing
	// Delete on the second would delete the first. The copy is named
	// after itself.
	if name := strings.TrimSpace(s.el.Attrs["data-scene"]); name != "" {
		outer := hfedit.FindAll(markup, func(e hfedit.Element) bool {
			_, has := e.Attrs["data-scene"]
			return has
		})
		if len(outer) > 0 {
			e, err := hfedit.SetAttribute(outer[0], markup, "data-scene", name+suffix)
			if err != nil {
				return "", "", err
			}
			edits = append(edits, e)
		}
	}
	out, err := hfedit.Apply(markup, edits)
	if err != nil {
		return "", "", err
	}
	return out, s.id + suffix, nil
}

// freeSuffix finds a suffix that leaves EVERY id in the copy unique.
//
// Every id, not just the scene's. Checking only the scene meant that
// duplicating `hook` into a film that already held `hook-title-2` — which
// is what a previous duplicate leaves behind, or what an author may simply
// have written — produced a copy whose headline collided with it. From
// then on the selection bridge, FindByID and every later edit resolve to
// whichever comes first, so the inspector edits one element while the user
// is looking at another.
func freeSuffix(source string, ids []hfedit.Element) (string, error) {
	for n := 2; n < 100; n++ {
		suffix := fmt.Sprintf("-%d", n)
		free := true
		for _, el := range ids {
			if _, taken := hfedit.FindByID(source, el.Attrs["id"]+suffix); taken {
				free = false
				break
			}
		}
		if free {
			return suffix, nil
		}
	}
	return "", fmt.Errorf("this scene has been duplicated too many times to name another copy")
}

// emptyScene is the markup for a new, blank scene.
//
// Deliberately minimal and deliberately NOT empty: a scene with nothing in
// it renders as a few seconds of background, which reads as a bug. A line
// of placeholder text says what it is and is the first thing the user will
// edit.
func emptyScene(source string, scenes []scene) (string, error) {
	id := "scene"
	for n := len(scenes) + 1; n < len(scenes)+100; n++ {
		candidate := fmt.Sprintf("scene-%d", n)
		if _, taken := hfedit.FindByID(source, candidate); !taken {
			id = candidate
			break
		}
	}
	return fmt.Sprintf(
		`<section id=%q class="clip" data-scene=%q data-start="0" data-duration="3">`+
			`<h1 id=%q class="clip" data-start="0" data-duration="3">New scene</h1></section>`,
		id, id, id+"-title"), nil
}

// msAttr reads a seconds attribute as milliseconds.
func msAttr(el hfedit.Element, name string) (int, bool) {
	raw, has := el.Attrs[name]
	if !has || strings.TrimSpace(raw) == "" {
		return 0, false
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return 0, false
	}
	return int(f*1000 + 0.5), true
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
