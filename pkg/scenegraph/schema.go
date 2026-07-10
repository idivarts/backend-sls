// Package scenegraph defines Trendly's owned, AI-readable/AI-mutable scene
// document for image and video content, plus the deterministic engines that
// operate on it: validation, a fixed edit-op vocabulary, an op-apply engine, a
// zone→pixel layout engine (which doubles as the frontend hit-box map) and an
// SVG serializer used for rendering.
//
// The document is deliberately semantic, not a freeform canvas:
//   - Elements live in named zones (top/center/bottom, left/right), never raw
//     x/y. The layout engine computes pixels; the AI never emits geometry.
//   - Edits are a fixed vocabulary of structured ops (see ops.go), applied
//     deterministically — the AI never rewrites the whole JSON.
//
// It has no external dependencies so it compiles and unit-tests standalone.
package scenegraph

// SchemaVersion is bumped when the document shape changes in a
// non-backwards-compatible way. Stored on every document.
const SchemaVersion = 1

// DocType discriminates image vs video documents.
type DocType string

const (
	DocImage DocType = "image"
	DocVideo DocType = "video"
)

// Zone is a semantic position slot. Vertical zones stack top→bottom; the
// horizontal hint (HAlign on an element) positions within a zone.
type Zone string

const (
	ZoneTop    Zone = "top"
	ZoneCenter Zone = "center"
	ZoneBottom Zone = "bottom"
	// Full covers the whole frame — used for backgrounds / hero images.
	ZoneFull Zone = "full"
)

// DefaultZones is the canonical vertical zone order used when a document does
// not specify its own.
var DefaultZones = []Zone{ZoneTop, ZoneCenter, ZoneBottom}

// ElementType discriminates the element payload.
type ElementType string

const (
	ElText  ElementType = "text"
	ElImage ElementType = "image"
	ElLogo  ElementType = "logo"
	ElShape ElementType = "shape"
)

// Size is the frame size in pixels (the export resolution).
type Size struct {
	W int `json:"w" firestore:"w"`
	H int `json:"h" firestore:"h"`
}

// Background of the frame. Kind is one of: color | gradient | image.
type Background struct {
	Kind string `json:"kind" firestore:"kind"`
	// Value is a hex color (color), the first gradient stop (gradient) or a
	// media ref/URL (image).
	Value string `json:"value,omitempty" firestore:"value,omitempty"`
	// Value2 + Angle are only used for gradient backgrounds.
	Value2 string `json:"value2,omitempty" firestore:"value2,omitempty"`
	Angle  int    `json:"angle,omitempty" firestore:"angle,omitempty"`
}

// Src references media for image/logo elements. Kind is one of:
// unsplash | upload | brand | generated. Ref is the provider id / storage URL /
// brand-kit slot the media resolves from.
type Src struct {
	Kind string `json:"kind" firestore:"kind"`
	Ref  string `json:"ref" firestore:"ref"`
}

// Transforms are deterministic, non-generative image adjustments.
type Transforms struct {
	RemoveBg bool   `json:"removeBg,omitempty" firestore:"removeBg,omitempty"`
	Filter   string `json:"filter,omitempty" firestore:"filter,omitempty"` // duotone|mono|none
	Fit      string `json:"fit,omitempty" firestore:"fit,omitempty"`       // cover|contain
}

// Style controls text/shape appearance. Font/Color use brand-kit tokens
// ("brand.heading", "brand.navy") or literal values (a hex color, a font name).
type Style struct {
	Font   string  `json:"font,omitempty" firestore:"font,omitempty"`
	Color  string  `json:"color,omitempty" firestore:"color,omitempty"`
	Scale  float64 `json:"scale,omitempty" firestore:"scale,omitempty"` // 1.0 = base
	Align  string  `json:"align,omitempty" firestore:"align,omitempty"` // left|center|right
	Weight string  `json:"weight,omitempty" firestore:"weight,omitempty"`
	// Scrim draws a contrast wash behind text so it stays legible over busy
	// backgrounds (enforced accessibility, see critique).
	Scrim bool `json:"scrim,omitempty" firestore:"scrim,omitempty"`
}

// Element is a single semantic object in a scene.
type Element struct {
	ID   string      `json:"id" firestore:"id"`
	Type ElementType `json:"type" firestore:"type"`
	Zone Zone        `json:"zone" firestore:"zone"`
	// HAlign positions the element horizontally within its zone.
	HAlign string `json:"hAlign,omitempty" firestore:"hAlign,omitempty"` // left|center|right

	// Text elements.
	Text string `json:"text,omitempty" firestore:"text,omitempty"`

	// Image elements.
	Src        *Src        `json:"src,omitempty" firestore:"src,omitempty"`
	Transforms *Transforms `json:"transforms,omitempty" firestore:"transforms,omitempty"`

	// Logo elements resolve their media from a brand-kit slot ("brand.logo").
	Slot string `json:"slot,omitempty" firestore:"slot,omitempty"`

	Style *Style `json:"style,omitempty" firestore:"style,omitempty"`

	// Animation preset for video scenes (fadeIn|popIn|kenBurns|slideUp…).
	Animation string `json:"animation,omitempty" firestore:"animation,omitempty"`

	// Locked elements cannot be deleted/moved by AI ops (e.g. a mandatory logo).
	Locked bool `json:"locked,omitempty" firestore:"locked,omitempty"`
}

// Document is the whole scene (image) or the base of a video document.
type Document struct {
	Version    int        `json:"version" firestore:"version"`
	Type       DocType    `json:"type" firestore:"type"`
	Size       Size       `json:"size" firestore:"size"`
	Background Background `json:"background" firestore:"background"`
	Zones      []Zone     `json:"zones" firestore:"zones"`
	Elements   []Element  `json:"elements" firestore:"elements"`

	// Timeline is present only when Type == DocVideo.
	Timeline *Timeline `json:"timeline,omitempty" firestore:"timeline,omitempty"`
}

// ---- Video timeline (extends the image model with time) ----

// Timeline is a sequence of image scenes + motion + audio.
type Timeline struct {
	FPS         int          `json:"fps" firestore:"fps"`
	Scenes      []Scene      `json:"scenes" firestore:"scenes"`
	Transitions []Transition `json:"transitions,omitempty" firestore:"transitions,omitempty"`
	Audio       []AudioTrack `json:"audio,omitempty" firestore:"audio,omitempty"`
	Captions    *Captions    `json:"captions,omitempty" firestore:"captions,omitempty"`
}

// Scene is one clip: an image scene graph + duration + motion preset.
type Scene struct {
	ID       string    `json:"id" firestore:"id"`
	Duration float64   `json:"duration" firestore:"duration"` // seconds
	Template string    `json:"template,omitempty" firestore:"template,omitempty"`
	Scene    *Document `json:"scene" firestore:"scene"` // an image Document
	Enter    string    `json:"enter,omitempty" firestore:"enter,omitempty"`
	Exit     string    `json:"exit,omitempty" firestore:"exit,omitempty"`
}

// Transition connects two scenes.
type Transition struct {
	A    string `json:"a" firestore:"a"`
	B    string `json:"b" firestore:"b"`
	Type string `json:"type" firestore:"type"` // crossfade|cut|slide
}

// AudioTrack is a music/voiceover/sfx layer muxed at render time. Src resolves
// from generatedAudio (ElevenLabs) or a preset.
type AudioTrack struct {
	Type   string  `json:"type" firestore:"type"` // music|voiceover|sfx
	Src    string  `json:"src" firestore:"src"`   // storage URL / generatedAudio id
	In     float64 `json:"in" firestore:"in"`
	Out    float64 `json:"out" firestore:"out"`
	Volume float64 `json:"volume,omitempty" firestore:"volume,omitempty"`
	// Duck lowers this track under a voiceover (music beds set Duck=true).
	Duck bool `json:"duck,omitempty" firestore:"duck,omitempty"`
}

// Captions configures burned-in captions.
type Captions struct {
	Enabled bool   `json:"enabled" firestore:"enabled"`
	Source  string `json:"source" firestore:"source"` // voiceover|script
}

// TotalDuration returns the sum of scene durations (0 for images).
func (d *Document) TotalDuration() float64 {
	if d.Timeline == nil {
		return 0
	}
	var t float64
	for _, s := range d.Timeline.Scenes {
		t += s.Duration
	}
	return t
}

// FindElement returns a pointer to the element with the given id, or nil.
func (d *Document) FindElement(id string) *Element {
	for i := range d.Elements {
		if d.Elements[i].ID == id {
			return &d.Elements[i]
		}
	}
	return nil
}
