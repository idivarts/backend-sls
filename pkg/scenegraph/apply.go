package scenegraph

import (
	"encoding/json"
	"fmt"
)

// OpResult records the outcome of applying a single op, for the confirm/diff UI
// and for surfacing deferred work (regenerateElement).
type OpResult struct {
	Op       Op     `json:"op"`
	Applied  bool   `json:"applied"`
	Pending  bool   `json:"pending,omitempty"` // e.g. regenerateElement queued, not yet done
	Describe string `json:"describe"`
	Warning  string `json:"warning,omitempty"`
}

// ApplyResult is the full result of an apply batch.
type ApplyResult struct {
	Document *Document  `json:"document"`
	Results  []OpResult `json:"results"`
}

// Apply deterministically applies a batch of ops to a copy of the document. It
// never mutates the input. Invalid or unresolvable ops are skipped with a
// warning rather than aborting the whole batch, so a single bad op does not
// discard the user's other edits. Callers should run ValidateOps first if they
// want to reject the whole turn.
func Apply(d *Document, ops []Op) (*ApplyResult, error) {
	next, err := clone(d)
	if err != nil {
		return nil, err
	}
	res := &ApplyResult{Document: next}
	for _, op := range ops {
		r := OpResult{Op: op, Describe: op.Describe()}
		if !knownOps[op.Op] {
			r.Warning = "unknown op"
			res.Results = append(res.Results, r)
			continue
		}
		applied, pending, warn := applyOne(next, op)
		r.Applied = applied
		r.Pending = pending
		r.Warning = warn
		res.Results = append(res.Results, r)
	}
	return res, nil
}

func applyOne(d *Document, op Op) (applied, pending bool, warning string) {
	switch op.Op {
	case OpSetText:
		el := findElementAnywhere(d, op.ID)
		if el == nil {
			return false, false, "element not found"
		}
		if el.Type != ElText {
			return false, false, "target is not a text element"
		}
		el.Text = op.Text
		return true, false, ""

	case OpMoveToZone:
		el := findElementAnywhere(d, op.ID)
		if el == nil {
			return false, false, "element not found"
		}
		if el.Locked {
			return false, false, "element is locked"
		}
		el.Zone = op.Zone
		return true, false, ""

	case OpSetImage:
		el := findElementAnywhere(d, op.ID)
		if el == nil {
			return false, false, "element not found"
		}
		if el.Type != ElImage && el.Type != ElLogo {
			return false, false, "target does not carry an image"
		}
		el.Src = op.Src
		if el.Type == ElLogo && op.Src.Kind == "brand" {
			el.Slot = op.Src.Ref
		}
		return true, false, ""

	case OpRestyle:
		el := findElementAnywhere(d, op.ID)
		if el == nil {
			return false, false, "element not found"
		}
		el.Style = mergeStyle(el.Style, op.Style)
		return true, false, ""

	case OpDelete:
		if removeElement(d, op.ID) {
			return true, false, ""
		}
		return false, false, "element not found or locked"

	case OpDuplicate:
		if dupErr := duplicateElement(d, op.ID); dupErr == "" {
			return true, false, ""
		} else {
			return false, false, dupErr
		}

	case OpSetBackground:
		if op.Background == nil {
			return false, false, "missing background"
		}
		d.Background = *op.Background
		return true, false, ""

	case OpAddElement:
		if op.Element == nil {
			return false, false, "missing element"
		}
		el := *op.Element
		if el.ID == "" {
			el.ID = nextElementID(d)
		}
		d.Elements = append(d.Elements, el)
		return true, false, ""

	case OpRegenerateElement:
		// Scoped generative work is deferred: we record it as pending so the
		// caller can queue an image job; the scene shape is unchanged.
		if findElementAnywhere(d, op.ID) == nil {
			return false, false, "element not found"
		}
		return false, true, "regeneration queued (deferred generative op)"

	// ---- video ops ----
	case OpSetDuration:
		if sc := findScene(d, op.SceneID); sc != nil {
			sc.Duration = op.Seconds
			return true, false, ""
		}
		return false, false, "scene not found"

	case OpSetTransition:
		if d.Timeline == nil {
			return false, false, "not a video document"
		}
		for i := range d.Timeline.Transitions {
			if d.Timeline.Transitions[i].A == op.A && d.Timeline.Transitions[i].B == op.B {
				d.Timeline.Transitions[i].Type = op.Type
				return true, false, ""
			}
		}
		d.Timeline.Transitions = append(d.Timeline.Transitions, Transition{A: op.A, B: op.B, Type: op.Type})
		return true, false, ""

	case OpSetAnimation:
		el := findElementAnywhere(d, op.ID)
		if el == nil {
			return false, false, "element not found"
		}
		el.Animation = op.Type
		return true, false, ""

	case OpReorderScene:
		if reorderScene(d, op.SceneID, op.Index) {
			return true, false, ""
		}
		return false, false, "scene not found"

	case OpSetAudio:
		if d.Timeline == nil {
			return false, false, "not a video document"
		}
		// Audio generation is async; record the brief as a pending track that
		// the audio pipeline fills with a concrete Src.
		d.Timeline.Audio = upsertAudio(d.Timeline.Audio, AudioTrack{Type: op.Type, Duck: op.Type == "music"})
		return false, true, "audio generation queued: " + op.Brief

	case OpEnableCaptions:
		if d.Timeline == nil {
			return false, false, "not a video document"
		}
		d.Timeline.Captions = &Captions{Enabled: true, Source: op.Source}
		return true, false, ""

	case OpTrim:
		// Trim is stored on the scene's duration for single-scene clips.
		if sc := findScene(d, op.ID); sc != nil {
			sc.Duration = op.Out - op.In
			return true, false, ""
		}
		return false, false, "clip not found"
	}
	return false, false, "unhandled op"
}

func mergeStyle(base, patch *Style) *Style {
	if patch == nil {
		return base
	}
	if base == nil {
		base = &Style{}
	}
	out := *base
	if patch.Font != "" {
		out.Font = patch.Font
	}
	if patch.Color != "" {
		out.Color = patch.Color
	}
	if patch.Scale != 0 {
		out.Scale = patch.Scale
	}
	if patch.Align != "" {
		out.Align = patch.Align
	}
	if patch.Weight != "" {
		out.Weight = patch.Weight
	}
	out.Scrim = patch.Scrim || base.Scrim
	return &out
}

func removeElement(d *Document, id string) bool {
	for i := range d.Elements {
		if d.Elements[i].ID == id {
			if d.Elements[i].Locked {
				return false
			}
			d.Elements = append(d.Elements[:i], d.Elements[i+1:]...)
			return true
		}
	}
	if d.Timeline != nil {
		for s := range d.Timeline.Scenes {
			sc := d.Timeline.Scenes[s].Scene
			if sc == nil {
				continue
			}
			for i := range sc.Elements {
				if sc.Elements[i].ID == id && !sc.Elements[i].Locked {
					sc.Elements = append(sc.Elements[:i], sc.Elements[i+1:]...)
					return true
				}
			}
		}
	}
	return false
}

func duplicateElement(d *Document, id string) string {
	el := d.FindElement(id)
	if el == nil {
		return "element not found"
	}
	dup := *el
	dup.ID = nextElementID(d)
	d.Elements = append(d.Elements, dup)
	return ""
}

func nextElementID(d *Document) string {
	i := len(d.Elements) + 1
	for {
		candidate := fmt.Sprintf("el_%d", i)
		if d.FindElement(candidate) == nil {
			return candidate
		}
		i++
	}
}

func findScene(d *Document, sceneID string) *Scene {
	if d.Timeline == nil {
		return nil
	}
	for i := range d.Timeline.Scenes {
		if d.Timeline.Scenes[i].ID == sceneID {
			return &d.Timeline.Scenes[i]
		}
	}
	return nil
}

func reorderScene(d *Document, sceneID string, index int) bool {
	if d.Timeline == nil {
		return false
	}
	scenes := d.Timeline.Scenes
	from := -1
	for i := range scenes {
		if scenes[i].ID == sceneID {
			from = i
			break
		}
	}
	if from == -1 {
		return false
	}
	if index < 0 {
		index = 0
	}
	if index >= len(scenes) {
		index = len(scenes) - 1
	}
	s := scenes[from]
	scenes = append(scenes[:from], scenes[from+1:]...)
	rest := append([]Scene{}, scenes[index:]...)
	scenes = append(scenes[:index], s)
	scenes = append(scenes, rest...)
	d.Timeline.Scenes = scenes
	return true
}

func upsertAudio(tracks []AudioTrack, t AudioTrack) []AudioTrack {
	for i := range tracks {
		if tracks[i].Type == t.Type {
			tracks[i].Duck = t.Duck
			return tracks
		}
	}
	return append(tracks, t)
}

// clone deep-copies a document via JSON round-trip. Documents are small
// (kilobytes) so this is cheap and guarantees no shared pointers.
func clone(d *Document) (*Document, error) {
	if d == nil {
		return nil, fmt.Errorf("scenegraph: cannot apply to nil document")
	}
	b, err := json.Marshal(d)
	if err != nil {
		return nil, fmt.Errorf("scenegraph: clone marshal: %w", err)
	}
	var out Document
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("scenegraph: clone unmarshal: %w", err)
	}
	return &out, nil
}
