package scenegraph

import (
	"encoding/json"
	"fmt"
)

// OpName is the fixed vocabulary the AI may emit. Anything outside this set is
// rejected — the AI never emits geometry or freehand JSON rewrites.
type OpName string

const (
	// Image / shared ops (v1).
	OpSetText       OpName = "setText"
	OpMoveToZone    OpName = "moveToZone"
	OpSetImage      OpName = "setImage"
	OpRestyle       OpName = "restyle"
	OpDelete        OpName = "delete"
	OpDuplicate     OpName = "duplicate"
	OpSetBackground OpName = "setBackground"
	OpAddElement    OpName = "addElement"
	// regenerateElement is scoped generative work — deferred (see ticket Risks).
	// It is accepted by the vocabulary but the apply engine records it as a
	// pending request rather than mutating pixels.
	OpRegenerateElement OpName = "regenerateElement"

	// Video / temporal ops (extend v1).
	OpSetDuration    OpName = "setDuration"
	OpSetTransition  OpName = "setTransition"
	OpSetAnimation   OpName = "setAnimation"
	OpReorderScene   OpName = "reorderScene"
	OpSetAudio       OpName = "setAudio"
	OpEnableCaptions OpName = "enableCaptions"
	OpTrim           OpName = "trim"
)

// knownOps is the allow-list used for validation.
var knownOps = map[OpName]bool{
	OpSetText: true, OpMoveToZone: true, OpSetImage: true, OpRestyle: true,
	OpDelete: true, OpDuplicate: true, OpSetBackground: true, OpAddElement: true,
	OpRegenerateElement: true, OpSetDuration: true, OpSetTransition: true,
	OpSetAnimation: true, OpReorderScene: true, OpSetAudio: true,
	OpEnableCaptions: true, OpTrim: true,
}

// Op is one structured edit. Only the fields relevant to Op are populated; the
// apply engine validates that the required fields for each op are present.
type Op struct {
	Op OpName `json:"op"`

	// Target element / scene.
	ID      string `json:"id,omitempty"`
	SceneID string `json:"sceneId,omitempty"`

	// Payloads by op.
	Text       string      `json:"text,omitempty"`
	Zone       Zone        `json:"zone,omitempty"`
	Src        *Src        `json:"src,omitempty"`
	Style      *Style      `json:"style,omitempty"`
	Background *Background `json:"background,omitempty"`
	Element    *Element    `json:"element,omitempty"`

	// Video params.
	Seconds float64 `json:"seconds,omitempty"`
	Index   int     `json:"index,omitempty"`
	A       string  `json:"a,omitempty"`
	B       string  `json:"b,omitempty"`
	Type    string  `json:"type,omitempty"`  // transition type / audio type / animation preset
	Brief   string  `json:"brief,omitempty"` // free-text brief for audio generation
	Source  string  `json:"source,omitempty"`
	In      float64 `json:"in,omitempty"`
	Out     float64 `json:"out,omitempty"`

	// Instruction carries the natural-language rationale for regenerateElement
	// and is echoed back for the pending-work record.
	Instruction string `json:"instruction,omitempty"`
}

// ParseOps decodes the AI's structured output (a JSON array of ops) into typed
// ops. It tolerates a top-level {"ops": [...]} wrapper as well as a bare array.
func ParseOps(raw []byte) ([]Op, error) {
	trimmed := trimSpace(raw)
	if len(trimmed) == 0 {
		return nil, nil
	}
	if trimmed[0] == '{' {
		var wrap struct {
			Ops []Op `json:"ops"`
		}
		if err := json.Unmarshal(trimmed, &wrap); err != nil {
			return nil, fmt.Errorf("scenegraph: parse ops object: %w", err)
		}
		return wrap.Ops, nil
	}
	var ops []Op
	if err := json.Unmarshal(trimmed, &ops); err != nil {
		return nil, fmt.Errorf("scenegraph: parse ops array: %w", err)
	}
	return ops, nil
}

func trimSpace(b []byte) []byte {
	i, j := 0, len(b)
	for i < j && isSpace(b[i]) {
		i++
	}
	for j > i && isSpace(b[j-1]) {
		j--
	}
	return b[i:j]
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

// Describe renders a single op as a short human-readable line for the
// preview/diff shown before applying.
func (o Op) Describe() string {
	switch o.Op {
	case OpSetText:
		return fmt.Sprintf("Set text of %s to %q", o.ID, o.Text)
	case OpMoveToZone:
		return fmt.Sprintf("Move %s to %s", o.ID, o.Zone)
	case OpSetImage:
		ref := ""
		if o.Src != nil {
			ref = o.Src.Kind + ":" + o.Src.Ref
		}
		return fmt.Sprintf("Set image of %s to %s", o.ID, ref)
	case OpRestyle:
		return fmt.Sprintf("Restyle %s", o.ID)
	case OpDelete:
		return fmt.Sprintf("Remove %s", o.ID)
	case OpDuplicate:
		return fmt.Sprintf("Duplicate %s", o.ID)
	case OpSetBackground:
		if o.Background != nil {
			return fmt.Sprintf("Change background to %s", o.Background.Kind)
		}
		return "Change background"
	case OpAddElement:
		if o.Element != nil {
			return fmt.Sprintf("Add %s element in %s", o.Element.Type, o.Element.Zone)
		}
		return "Add element"
	case OpRegenerateElement:
		return fmt.Sprintf("Regenerate %s: %s", o.ID, o.Instruction)
	case OpSetDuration:
		return fmt.Sprintf("Set %s duration to %.1fs", o.SceneID, o.Seconds)
	case OpSetTransition:
		return fmt.Sprintf("Set transition %s→%s to %s", o.A, o.B, o.Type)
	case OpSetAnimation:
		return fmt.Sprintf("Animate %s with %s", o.ID, o.Type)
	case OpReorderScene:
		return fmt.Sprintf("Move scene %s to index %d", o.SceneID, o.Index)
	case OpSetAudio:
		return fmt.Sprintf("Set %s audio: %s", o.Type, o.Brief)
	case OpEnableCaptions:
		return fmt.Sprintf("Enable captions from %s", o.Source)
	case OpTrim:
		return fmt.Sprintf("Trim %s to %.1f–%.1fs", o.ID, o.In, o.Out)
	default:
		return string(o.Op)
	}
}
