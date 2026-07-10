package scenegraph

import (
	"fmt"
	"strings"
)

// ValidationError aggregates schema problems found in a document.
type ValidationError struct {
	Issues []string
}

func (e *ValidationError) Error() string {
	return "scenegraph: invalid document: " + strings.Join(e.Issues, "; ")
}

func (e *ValidationError) add(format string, a ...interface{}) {
	e.Issues = append(e.Issues, fmt.Sprintf(format, a...))
}

var validHAlign = map[string]bool{"": true, "left": true, "center": true, "right": true}
var validBackgroundKind = map[string]bool{"color": true, "gradient": true, "image": true}
var validSrcKind = map[string]bool{"unsplash": true, "upload": true, "brand": true, "generated": true}

// ValidateDocument checks that a document is well-formed and internally
// consistent (unique ids, valid zones, type-appropriate payloads). It returns a
// *ValidationError with every issue, or nil.
func ValidateDocument(d *Document) error {
	ve := &ValidationError{}
	if d == nil {
		ve.add("document is nil")
		return ve
	}
	if d.Type != DocImage && d.Type != DocVideo {
		ve.add("type %q must be image or video", d.Type)
	}
	if d.Size.W <= 0 || d.Size.H <= 0 {
		ve.add("size must be positive (got %dx%d)", d.Size.W, d.Size.H)
	}
	if !validBackgroundKind[d.Background.Kind] {
		ve.add("background.kind %q invalid", d.Background.Kind)
	}

	zoneSet := map[Zone]bool{}
	for _, z := range d.zones() {
		zoneSet[z] = true
	}

	validateElements(ve, d.Elements, zoneSet, "")

	if d.Type == DocVideo {
		validateTimeline(ve, d.Timeline, zoneSet)
	}

	if len(ve.Issues) > 0 {
		return ve
	}
	return nil
}

// zones returns the document's zone list, falling back to DefaultZones (+full).
func (d *Document) zones() []Zone {
	if len(d.Zones) == 0 {
		return append(append([]Zone{}, DefaultZones...), ZoneFull)
	}
	return d.Zones
}

func validateElements(ve *ValidationError, els []Element, zoneSet map[Zone]bool, prefix string) {
	seen := map[string]bool{}
	for i, el := range els {
		where := fmt.Sprintf("%selements[%d]", prefix, i)
		if el.ID == "" {
			ve.add("%s: missing id", where)
		} else if seen[el.ID] {
			ve.add("%s: duplicate id %q", where, el.ID)
		}
		seen[el.ID] = true

		if el.Zone != "" && !zoneSet[el.Zone] {
			ve.add("%s (%s): zone %q not declared", where, el.ID, el.Zone)
		}
		if !validHAlign[el.HAlign] {
			ve.add("%s (%s): hAlign %q invalid", where, el.ID, el.HAlign)
		}

		switch el.Type {
		case ElText:
			if strings.TrimSpace(el.Text) == "" {
				ve.add("%s (%s): text element has empty text", where, el.ID)
			}
		case ElImage:
			if el.Src == nil {
				ve.add("%s (%s): image element missing src", where, el.ID)
			} else if !validSrcKind[el.Src.Kind] {
				ve.add("%s (%s): src.kind %q invalid", where, el.ID, el.Src.Kind)
			}
		case ElLogo:
			if el.Slot == "" && el.Src == nil {
				ve.add("%s (%s): logo element needs a slot or src", where, el.ID)
			}
		case ElShape:
			// shapes are style-only; nothing extra required.
		default:
			ve.add("%s (%s): unknown element type %q", where, el.ID, el.Type)
		}
	}
}

func validateTimeline(ve *ValidationError, t *Timeline, zoneSet map[Zone]bool) {
	if t == nil {
		ve.add("video document missing timeline")
		return
	}
	if t.FPS <= 0 {
		ve.add("timeline.fps must be positive")
	}
	if len(t.Scenes) == 0 {
		ve.add("timeline has no scenes")
	}
	sceneIDs := map[string]bool{}
	for i, s := range t.Scenes {
		if s.ID == "" {
			ve.add("timeline.scenes[%d]: missing id", i)
		} else if sceneIDs[s.ID] {
			ve.add("timeline.scenes[%d]: duplicate id %q", i, s.ID)
		}
		sceneIDs[s.ID] = true
		if s.Duration <= 0 {
			ve.add("timeline.scenes[%d] (%s): duration must be positive", i, s.ID)
		}
		if s.Scene == nil {
			ve.add("timeline.scenes[%d] (%s): missing scene graph", i, s.ID)
			continue
		}
		validateElements(ve, s.Scene.Elements, zoneSet, fmt.Sprintf("scenes[%s].", s.ID))
	}
	for i, tr := range t.Transitions {
		if !sceneIDs[tr.A] || !sceneIDs[tr.B] {
			ve.add("timeline.transitions[%d]: references unknown scene", i)
		}
	}
}

// ValidateOps checks a batch of ops against a document without applying them —
// used to reject a bad AI turn before showing a preview. It returns the list of
// invalid-op descriptions (empty if all valid).
func ValidateOps(d *Document, ops []Op) []string {
	var problems []string
	for i, op := range ops {
		if !knownOps[op.Op] {
			problems = append(problems, fmt.Sprintf("op[%d]: unknown op %q", i, op.Op))
			continue
		}
		if err := checkOpTarget(d, op); err != nil {
			problems = append(problems, fmt.Sprintf("op[%d] (%s): %v", i, op.Op, err))
		}
	}
	return problems
}

// checkOpTarget validates that an op's required fields and targets exist.
func checkOpTarget(d *Document, op Op) error {
	needsElement := map[OpName]bool{
		OpSetText: true, OpMoveToZone: true, OpSetImage: true, OpRestyle: true,
		OpDelete: true, OpDuplicate: true, OpRegenerateElement: true,
		OpSetAnimation: true, OpTrim: true,
	}
	if needsElement[op.Op] {
		if op.ID == "" {
			return fmt.Errorf("missing target id")
		}
		if findElementAnywhere(d, op.ID) == nil {
			return fmt.Errorf("element %q not found", op.ID)
		}
	}
	switch op.Op {
	case OpMoveToZone:
		if op.Zone == "" {
			return fmt.Errorf("missing zone")
		}
	case OpSetImage:
		if op.Src == nil {
			return fmt.Errorf("missing src")
		}
	case OpSetBackground:
		if op.Background == nil || !validBackgroundKind[op.Background.Kind] {
			return fmt.Errorf("invalid background")
		}
	case OpAddElement:
		if op.Element == nil {
			return fmt.Errorf("missing element")
		}
	case OpSetDuration, OpReorderScene:
		if op.SceneID == "" {
			return fmt.Errorf("missing sceneId")
		}
	}
	return nil
}

// findElementAnywhere looks in the top-level elements and every video scene.
func findElementAnywhere(d *Document, id string) *Element {
	if el := d.FindElement(id); el != nil {
		return el
	}
	if d.Timeline != nil {
		for i := range d.Timeline.Scenes {
			if d.Timeline.Scenes[i].Scene != nil {
				if el := d.Timeline.Scenes[i].Scene.FindElement(id); el != nil {
					return el
				}
			}
		}
	}
	return nil
}
