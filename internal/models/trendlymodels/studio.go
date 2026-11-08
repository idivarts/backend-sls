package trendlymodels

// The Design Studio's documents.
//
//	brands/{brandId}/contents/{contentId}/studios/{studioId}
//	  .../scenes/{sceneId}
//	  .../renders/{renderId}
//	  .../runs/{runId}
//
// Mirrors shared-libs/firestore/trendly-pro/models/studio.ts. Field names are
// the json tags there. Change one, change both.
//
// ── Why this replaces the old studio ────────────────────────────────────────
// The old one kept a whole design as ONE HTML document whose slides were
// `[data-slide]` children, and a separate composition timeline of clips. Two
// structures both claimed to say what "scene 3" was, and a scene was only a DOM
// node — so it could not be regenerated on its own, could not carry its own
// duration or transition, and could not be anything but HTML.
//
// Here A SCENE IS A DOCUMENT, and ORDER LIVES IN ONE PLACE: `sceneOrder` on the
// studio. A scene's `index` is a cache for sorting a query result. When the two
// disagree, sceneOrder wins — two places that both claim to know the order is
// the exact bug this rebuild exists to remove.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Scene sources. What fills a scene.
const (
	// SceneMotion is an HTML/CSS animation that the scene owns outright,
	// which is what lets one scene be regenerated without touching its
	// neighbours.
	SceneMotion = "motion"
	// SceneComposite is also an HTML document, but one placing images and
	// elements to be captured ONCE as a still. A separate kind rather than a
	// flag on motion because the renderer treats them differently and the AI
	// plans them differently.
	SceneComposite = "composite"
	SceneVideo     = "video"
	SceneImage     = "image"
	// SceneEmpty is a deliberate placeholder — a gap someone left on purpose.
	// Not a broken scene, and nothing should nag about it.
	SceneEmpty = "empty"
)

// Layer slots. Named positions rather than coordinates, which is a deliberate
// restriction on what the AI may ask for: a planner given x/y/w/h produces
// layouts that are subtly wrong at another aspect ratio, where one given "left
// half" produces a split screen that works at every size.
const (
	SlotFull       = "full"
	SlotLeft       = "left"
	SlotRight      = "right"
	SlotTop        = "top"
	SlotBottom     = "bottom"
	SlotQ1         = "q1"
	SlotQ2         = "q2"
	SlotQ3         = "q3"
	SlotQ4         = "q4"
	SlotPipTL      = "pip-tl"
	SlotPipTR      = "pip-tr"
	SlotPipBL      = "pip-bl"
	SlotPipBR      = "pip-br"
	SlotLowerThird = "lower-third"
	// SlotCustom is for a human dragging a layer. The planner may not pick it.
	SlotCustom = "custom"
)

// Transition kinds.
const (
	TransitionCut          = "cut"
	TransitionDissolve     = "dissolve"
	TransitionDip          = "dip"
	TransitionSlide        = "slide"
	TransitionWipe         = "wipe"
	TransitionBlurDissolve = "blur-dissolve"
	TransitionWhip         = "whip"
)

// Transition presets. A document-level choice, which matters more than the
// individual kinds: a plan picking a different transition per boundary looks
// like a slideshow template, and one preset applied throughout looks like an
// edit.
const (
	PresetNone      = "none"
	PresetSnappy    = "snappy"
	PresetSoft      = "soft"
	PresetGraphic   = "graphic"
	PresetEnergetic = "energetic"
)

// Narration sources. ENFORCEMENT, not metadata — see ISceneNarration in the
// TypeScript mirror and the validator in internal/trendlyapis/ai/director.
const (
	NarrationFromScript   = "script"
	NarrationFromCaption  = "caption"
	NarrationFromBrand    = "brand"
	NarrationParaphrase   = "paraphrase"
	NarrationIllustrative = "illustrative"
)

// LayerRect is a rectangle in FRACTIONS of the canvas (0..1), for custom slots.
//
// Fractions rather than pixels so a layer survives the canvas changing size: a
// 9:16 reel re-cut as a 1:1 post keeps its composition instead of placing a
// caption off screen.
type LayerRect struct {
	X float64 `json:"x" firestore:"x"`
	Y float64 `json:"y" firestore:"y"`
	W float64 `json:"w" firestore:"w"`
	H float64 `json:"h" firestore:"h"`
}

// KenBurns is a slow push or pan across a still, so it does not read as a dead
// frame.
type KenBurns struct {
	FromScale float64 `json:"fromScale" firestore:"fromScale"`
	ToScale   float64 `json:"toScale" firestore:"toScale"`
	OriginX   float64 `json:"originX,omitempty" firestore:"originX,omitempty"`
	OriginY   float64 `json:"originY,omitempty" firestore:"originY,omitempty"`
}

// StudioLayer is one layer of a scene.
//
// Most scenes have exactly one, which is the point of modelling a stack at all:
// "two clips side by side with type over them" is three layers of ONE scene,
// not three scenes. A scene is a moment; the stack is what is visible during it.
type StudioLayer struct {
	ID   string `json:"id" firestore:"id"`
	Kind string `json:"kind" firestore:"kind"` // media | html
	Slot string `json:"slot" firestore:"slot"`

	Rect *LayerRect `json:"rect,omitempty" firestore:"rect,omitempty"`

	MediaID string `json:"mediaId,omitempty" firestore:"mediaId,omitempty"`
	// HTML lives on the layer rather than in a subcollection: a scene is
	// already a document, so nesting one more level buys nothing and costs a
	// read on every scene the rail draws.
	HTML string `json:"html,omitempty" firestore:"html,omitempty"`

	TrimInMs  int `json:"trimInMs,omitempty" firestore:"trimInMs,omitempty"`
	TrimOutMs int `json:"trimOutMs,omitempty" firestore:"trimOutMs,omitempty"`

	Volume  float64   `json:"volume,omitempty" firestore:"volume,omitempty"`
	Speed   float64   `json:"speed,omitempty" firestore:"speed,omitempty"`
	Fit     string    `json:"fit,omitempty" firestore:"fit,omitempty"`
	Opacity float64   `json:"opacity,omitempty" firestore:"opacity,omitempty"`
	KenBurn *KenBurns `json:"kenBurns,omitempty" firestore:"kenBurns,omitempty"`

	// FreezeOnEnd holds a video layer's last frame when it is shorter than the
	// scene, rather than cutting to black. Black mid-scene reads as a bug; a
	// freeze reads as a choice.
	FreezeOnEnd bool `json:"freezeOnEnd,omitempty" firestore:"freezeOnEnd,omitempty"`
}

// Transition is how one scene becomes the next. It lives on the INCOMING scene.
type Transition struct {
	Kind string `json:"kind" firestore:"kind"`
	// DurationMs is an OVERLAP, not a gap. A 400ms dissolve takes 200ms off
	// the end of the outgoing scene and 200ms off the start of the incoming
	// one, so a timeline gets SHORTER as transitions are added. Clamped to
	// half the shorter neighbour by TransitionOverlapMs.
	DurationMs int    `json:"durationMs" firestore:"durationMs"`
	Direction  string `json:"direction,omitempty" firestore:"direction,omitempty"`
}

// SceneNarration is the spoken line over a scene, and where it came from.
type SceneNarration struct {
	Line   string `json:"line" firestore:"line"`
	Source string `json:"source" firestore:"source"`
	// BrandFactID identifies which documented fact, for Source == brand.
	BrandFactID string `json:"brandFactId,omitempty" firestore:"brandFactId,omitempty"`
	// FitToNarration stretches the scene to hold the line. Without it, a line
	// that cannot be said inside the scene is REFUSED rather than allowed to
	// overrun the picture — the commonest way a render comes out wrong.
	FitToNarration bool   `json:"fitToNarration,omitempty" firestore:"fitToNarration,omitempty"`
	MediaID        string `json:"mediaId,omitempty" firestore:"mediaId,omitempty"`
	OffsetMs       int    `json:"offsetMs,omitempty" firestore:"offsetMs,omitempty"`
}

// SceneGeneration records how a scene's media was generated.
type SceneGeneration struct {
	Model  string                 `json:"model" firestore:"model"`
	Prompt string                 `json:"prompt" firestore:"prompt"`
	Params map[string]interface{} `json:"params,omitempty" firestore:"params,omitempty"`
	JobID  string                 `json:"jobId,omitempty" firestore:"jobId,omitempty"`
	Status string                 `json:"status,omitempty" firestore:"status,omitempty"`
	Error  string                 `json:"error,omitempty" firestore:"error,omitempty"`

	// ChainedFromSceneID means this scene continues the previous one as one
	// unbroken motion, generated from its last frame.
	//
	// ⚠️ A CHAIN IS NOT A WAY TO HOLD A FACE across separate beats. It only
	// makes sense when two scenes are genuinely the same instant carrying on;
	// otherwise the second clip inherits the first's exact framing and the cut
	// reads as a glitch. For a person in two different moments, use a cast
	// anchor.
	ChainedFromSceneID string   `json:"chainedFromSceneId,omitempty" firestore:"chainedFromSceneId,omitempty"`
	StartFrameMediaID  string   `json:"startFrameMediaId,omitempty" firestore:"startFrameMediaId,omitempty"`
	ReferenceMediaIDs  []string `json:"referenceMediaIds,omitempty" firestore:"referenceMediaIds,omitempty"`

	CastIDs    []string `json:"castIds,omitempty" firestore:"castIds,omitempty"`
	LocationID string   `json:"locationId,omitempty" firestore:"locationId,omitempty"`
}

// CastMember is a person who appears in more than one generated clip.
//
// The whole mechanism for keeping a face the same, and three things make it
// work: ONE approved still every clip references, a `look` string appended
// VERBATIM to every prompt (paraphrasing the wardrobe between clips is how a
// navy jumper becomes a blue shirt), and a description for the human reading
// the plan rather than for the model.
type CastMember struct {
	ID            string `json:"id" firestore:"id"`
	Description   string `json:"description" firestore:"description"`
	Look          string `json:"look" firestore:"look"`
	AnchorMediaID string `json:"anchorMediaId,omitempty" firestore:"anchorMediaId,omitempty"`
	VoiceID       string `json:"voiceId,omitempty" firestore:"voiceId,omitempty"`
}

// StudioLocation is a place appearing in more than one generated clip. Same
// rule as cast: without an anchor, each clip invents its own room.
type StudioLocation struct {
	ID            string `json:"id" firestore:"id"`
	Description   string `json:"description" firestore:"description"`
	Look          string `json:"look" firestore:"look"`
	AnchorMediaID string `json:"anchorMediaId,omitempty" firestore:"anchorMediaId,omitempty"`
}

// AudioLane is one piece of audio on the document.
//
// ⚠️ A VOICEOVER IS ANCHORED TO A SCENE, not to an absolute millisecond.
// {sceneId, offsetMs} survives reorder, split and retime; an absolute start does
// not — and reordering is the most common edit in the rail, so absolute times
// would desynchronise voice from picture on the first drag. Music is the
// exception: a bed belongs to the whole document and starts at zero.
type AudioLane struct {
	ID      string `json:"id" firestore:"id"`
	Kind    string `json:"kind" firestore:"kind"` // music | voiceover | sfx
	MediaID string `json:"mediaId" firestore:"mediaId"`

	FromStart bool   `json:"fromStart,omitempty" firestore:"fromStart,omitempty"`
	SceneID   string `json:"sceneId,omitempty" firestore:"sceneId,omitempty"`
	OffsetMs  int    `json:"offsetMs,omitempty" firestore:"offsetMs,omitempty"`

	Volume float64 `json:"volume,omitempty" firestore:"volume,omitempty"`
	// DuckMusic lowers the bed under this lane. On by default for voiceover:
	// a bed at full level under a voice is the commonest reason a render
	// sounds amateur.
	DuckMusic bool `json:"duckMusic,omitempty" firestore:"duckMusic,omitempty"`
	FadeInMs  int  `json:"fadeInMs,omitempty" firestore:"fadeInMs,omitempty"`
	FadeOutMs int  `json:"fadeOutMs,omitempty" firestore:"fadeOutMs,omitempty"`
	TrimInMs  int  `json:"trimInMs,omitempty" firestore:"trimInMs,omitempty"`
	TrimOutMs int  `json:"trimOutMs,omitempty" firestore:"trimOutMs,omitempty"`
}

// MusicRequest is a bed the director planned that nobody has applied yet.
//
// Approval must not spend money or overwrite sound on its own: generating is a
// metered call the user has not made, and a studio being approved onto may
// already have a bed the user chose. So the plan's intent is parked here and
// the sound panel offers it in one tap.
//
// It is not dropped, which is what used to happen, because the plan's music is
// the part hardest to re-describe — "32 seconds, builds at 0:18" is a
// judgement about the edit, not something a user retypes from memory.
type MusicRequest struct {
	// Approach is "generate" (costs money when applied) or "reuse" (a row the
	// brand already owns). Reuse waits too: the reason to wait is the bed that
	// may already be there, not the price.
	Approach string `json:"approach" firestore:"approach"`
	ModelID  string `json:"modelId,omitempty" firestore:"modelId,omitempty"`
	Prompt   string `json:"prompt,omitempty" firestore:"prompt,omitempty"`
	MediaID  string `json:"mediaId,omitempty" firestore:"mediaId,omitempty"`
	// Dismissed records a decline, so it stops being offered without the
	// judgement behind it being lost.
	Dismissed bool `json:"dismissed,omitempty" firestore:"dismissed,omitempty"`
}

// StudioAudio holds the lanes and the document's one voice.
type StudioAudio struct {
	Lanes []AudioLane `json:"lanes,omitempty" firestore:"lanes,omitempty"`
	// VoiceID is ONE voice for the whole document. Not per scene: two voices
	// across a 30-second reel reads as two videos spliced together.
	VoiceID      string        `json:"voiceId,omitempty" firestore:"voiceId,omitempty"`
	MusicRequest *MusicRequest `json:"musicRequest,omitempty" firestore:"musicRequest,omitempty"`
}

// LaneMediaIDs is the distinct media an audio object references.
//
// Separate from SceneMediaIDs because the two are counted against different
// documents: a scene's media hangs off the scene, a lane's off the studio. The
// sets overlap in practice — a clip used as footage and as its own sound — and
// a row referenced twice should be counted twice, which is why these are never
// merged into one list.
func LaneMediaIDs(a *StudioAudio) []string {
	if a == nil {
		return []string{}
	}
	seen := map[string]bool{}
	out := []string{}
	for _, l := range a.Lanes {
		if l.MediaID == "" || seen[l.MediaID] {
			continue
		}
		seen[l.MediaID] = true
		out = append(out, l.MediaID)
	}
	return out
}

// StudioScene is one moment on the timeline.
type StudioScene struct {
	ID string `json:"id" firestore:"-"`

	// PrimaryKind is what this scene is, derived from layers[0]. Stored so the
	// rail can draw a scene without resolving its media, and so a query can
	// filter on it. layers[0] is the authority; this is a cache.
	PrimaryKind string `json:"primaryKind" firestore:"primaryKind"`

	// DurationMs is the time on screen BEFORE transitions take their overlap
	// and before narration stretches it. The effective length is computed.
	DurationMs int `json:"durationMs" firestore:"durationMs"`

	TransitionIn *Transition `json:"transitionIn,omitempty" firestore:"transitionIn,omitempty"`

	Layers []StudioLayer `json:"layers" firestore:"layers"`

	// MediaIDs is everything any layer references, duplicated out of the
	// layers so a reference count can move without walking the stack and a
	// scene delete knows what to decrement in one read.
	MediaIDs []string `json:"mediaIds,omitempty" firestore:"mediaIds,omitempty"`

	Narration *SceneNarration  `json:"narration,omitempty" firestore:"narration,omitempty"`
	Gen       *SceneGeneration `json:"gen,omitempty" firestore:"gen,omitempty"`

	// Index is a cache of position. sceneOrder is the authority.
	Index int `json:"index,omitempty" firestore:"index,omitempty"`

	Title string `json:"title,omitempty" firestore:"title,omitempty"`
	// Pending is set while a generation is in flight, so the rail shows a
	// placeholder rather than an empty scene the user might try to fill.
	Pending bool `json:"pending,omitempty" firestore:"pending,omitempty"`

	CreatedAt int64 `json:"createdAt" firestore:"createdAt"`
	UpdatedAt int64 `json:"updatedAt,omitempty" firestore:"updatedAt,omitempty"`
}

// Studio is the document a content's media is authored in.
type Studio struct {
	ID string `json:"id" firestore:"-"`

	Format string `json:"format" firestore:"format"`
	Width  int    `json:"width" firestore:"width"`
	Height int    `json:"height" firestore:"height"`
	FPS    int    `json:"fps" firestore:"fps"`

	// SceneOrder is THE authority on order.
	SceneOrder []string `json:"sceneOrder" firestore:"sceneOrder"`

	Audio            *StudioAudio `json:"audio,omitempty" firestore:"audio,omitempty"`
	TransitionPreset string       `json:"transitionPreset,omitempty" firestore:"transitionPreset,omitempty"`

	Cast      []CastMember     `json:"cast,omitempty" firestore:"cast,omitempty"`
	Locations []StudioLocation `json:"locations,omitempty" firestore:"locations,omitempty"`

	RenderID     string `json:"renderId,omitempty" firestore:"renderId,omitempty"`
	RenderURL    string `json:"renderUrl,omitempty" firestore:"renderUrl,omitempty"`
	RenderStatus string `json:"renderStatus,omitempty" firestore:"renderStatus,omitempty"`
	// RenderError is the sentence shown when a render failed. Kept on the
	// document rather than only logged: the app is subscribed here, and "it
	// didn't work" with no reason is the least actionable thing a screen can
	// say.
	RenderError string `json:"renderError,omitempty" firestore:"renderError,omitempty"`
	// RenderChargedTokens is what the wallet paid, so a retry knows not to
	// charge twice and a failure can be refunded exactly once.
	RenderChargedTokens int64 `json:"renderChargedTokens,omitempty" firestore:"renderChargedTokens,omitempty"`

	CreatedAt int64  `json:"createdAt" firestore:"createdAt"`
	UpdatedAt int64  `json:"updatedAt,omitempty" firestore:"updatedAt,omitempty"`
	CreatedBy string `json:"createdBy,omitempty" firestore:"createdBy,omitempty"`
}

// StudioRef is the pointer cached on the content document.
//
// ⚠️ Cached, therefore not authoritative: anything that CHARGES or PUBLISHES
// recomputes from the scenes, because this document is client-writable.
type StudioRef struct {
	StudioID   string `json:"studioId" firestore:"studioId"`
	SceneCount int    `json:"sceneCount" firestore:"sceneCount"`
	DurationMs int    `json:"durationMs" firestore:"durationMs"`
	RenderURL  string `json:"renderUrl,omitempty" firestore:"renderUrl,omitempty"`
	UpdatedAt  int64  `json:"updatedAt" firestore:"updatedAt"`
}

// ── Collection handles ──────────────────────────────────────────────────────

func studiosCollection(brandID, contentID string) *firestore.CollectionRef {
	return firestoredb.Client.Collection(
		fmt.Sprintf("brands/%s/contents/%s/studios", brandID, contentID))
}

func studioDoc(brandID, contentID, studioID string) *firestore.DocumentRef {
	return studiosCollection(brandID, contentID).Doc(studioID)
}

func scenesCollection(brandID, contentID, studioID string) *firestore.CollectionRef {
	return studioDoc(brandID, contentID, studioID).Collection("scenes")
}

// ── Studio reads and writes ─────────────────────────────────────────────────

// CreateStudio makes an EMPTY studio: no scenes at all.
//
// Deliberate, and the one decision the whole surface is built around. A studio
// that opens with a scene already in it has made a choice on the user's behalf
// about what they are making, and the first thing they do is delete it.
func CreateStudio(ctx context.Context, brandID, contentID string, s *Studio) (string, error) {
	now := time.Now().UnixMilli()
	if s.CreatedAt == 0 {
		s.CreatedAt = now
	}
	s.UpdatedAt = now
	if s.SceneOrder == nil {
		s.SceneOrder = []string{}
	}
	if s.FPS == 0 {
		s.FPS = 30
	}
	ref, _, err := studiosCollection(brandID, contentID).Add(ctx, s)
	if err != nil {
		return "", fmt.Errorf("CreateStudio: %w", err)
	}
	return ref.ID, nil
}

// GetStudio reads one studio, or (nil, nil) when it is absent.
func GetStudio(ctx context.Context, brandID, contentID, studioID string) (*Studio, error) {
	doc, err := studioDoc(brandID, contentID, studioID).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, err
	}
	var s Studio
	if err := doc.DataTo(&s); err != nil {
		return nil, err
	}
	s.ID = doc.Ref.ID
	return &s, nil
}

// UpdateStudio applies partial updates and stamps updatedAt.
// Refuses the audio object wholesale, for the same reason UpdateStudioScene
// refuses a layer list: lanes carry media references, and a lane list written
// without moving the counts with it is a library that can no longer be tidied.
// `audio.voiceId` and `audio.musicRequest` are fine — neither names media.
func UpdateStudio(ctx context.Context, brandID, contentID, studioID string, updates []firestore.Update) error {
	for _, u := range updates {
		// Both spellings. `firestore.Update` takes either a dotted Path or a
		// FieldPath, and a guard that knows only one is a guard the next
		// handler walks past without noticing.
		if p := updatePath(u); p == "audio" || p == "audio.lanes" {
			return fmt.Errorf("UpdateStudio: %q must go through SetStudioAudio", p)
		}
	}
	updates = append(updates, firestore.Update{Path: "updatedAt", Value: time.Now().UnixMilli()})
	_, err := studioDoc(brandID, contentID, studioID).Update(ctx, updates)
	return err
}

// updatePath is an update's target as one dotted string, whichever of the two
// ways the caller spelled it.
//
// Firestore accepts a dotted `Path` or a `FieldPath` slice and treats them
// identically, so the refcount guards have to as well — otherwise the one
// spelling nobody thought to check becomes the way the rule gets broken.
func updatePath(u firestore.Update) string {
	if u.Path != "" {
		return u.Path
	}
	return strings.Join(u.FieldPath, ".")
}

// SetStudioAudio replaces the audio object and moves the media reference counts
// the lanes imply.
//
// ⚠️ Lane media was never counted at all before this, which meant a soundtrack
// could sit on a studio with refCount 0 and the archive sweep was free to
// archive the bytes out from under a render. The bug is invisible until a
// render comes back silent.
//
// The DIFFERENCE is applied, not a clear-and-re-add: replacing one lane of two
// would otherwise drop the untouched lane's media to zero for an instant, and a
// sweep landing in that instant deletes something in use.
func SetStudioAudio(ctx context.Context, brandID, contentID, studioID string, audio *StudioAudio) error {
	before, err := GetStudio(ctx, brandID, contentID, studioID)
	if err != nil {
		return err
	}
	if before == nil {
		return fmt.Errorf("SetStudioAudio: no studio %s", studioID)
	}
	if audio == nil {
		audio = &StudioAudio{}
	}
	if audio.Lanes == nil {
		audio.Lanes = []AudioLane{}
	}

	if _, err := studioDoc(brandID, contentID, studioID).Update(ctx, []firestore.Update{
		{Path: "audio", Value: audio},
		{Path: "updatedAt", Value: time.Now().UnixMilli()},
	}); err != nil {
		return err
	}

	added, removed := diffStrings(LaneMediaIDs(before.Audio), LaneMediaIDs(audio))
	if len(added) > 0 {
		_ = AdjustAssetRefCounts(ctx, brandID, added, 1)
	}
	if len(removed) > 0 {
		_ = AdjustAssetRefCounts(ctx, brandID, removed, -1)
	}
	return nil
}

// SetSceneOrder replaces the order.
//
// A whole-array write rather than a splice, because the order is one fact and a
// partial update to it is how two clients end up each believing a different
// sequence. The caller sends what it wants the order to be.
func SetSceneOrder(ctx context.Context, brandID, contentID, studioID string, order []string) error {
	if order == nil {
		order = []string{}
	}
	return UpdateStudio(ctx, brandID, contentID, studioID, []firestore.Update{
		{Path: "sceneOrder", Value: order},
	})
}

// DeleteStudio removes a studio and every scene under it, releasing the media
// references as it goes.
//
// The reference release is the part that matters: deleting the documents without
// decrementing would leave every piece of media the studio used permanently
// un-archivable, which is a library that can never be tidied.
func DeleteStudio(ctx context.Context, brandID, contentID, studioID string) error {
	// Read the studio before anything is deleted, because its audio lanes hold
	// media references too and the document is about to go. A bed left counted
	// after its studio is gone is permanently un-archivable.
	studio, err := GetStudio(ctx, brandID, contentID, studioID)
	if err != nil {
		return err
	}

	scenes, err := ListStudioScenes(ctx, brandID, contentID, studioID)
	if err != nil {
		return err
	}
	for _, sc := range scenes {
		// Delete FIRST, release the media SECOND — the order DeleteStudioScene
		// already uses, and the only safe one.
		//
		// Reversed, a scene delete that fails after the decrement leaves the
		// count too LOW while the scene still references the media, and a
		// sweep is then free to archive something a live scene is showing.
		// This way the failure leaves the count too HIGH, which only makes
		// the media un-archivable until someone fixes it by hand.
		//
		// Un-archivable is an inconvenience; archived-while-in-use loses the
		// user's work. When a counter cannot be updated atomically with the
		// thing it counts, it should fail toward the recoverable side.
		if _, derr := scenesCollection(brandID, contentID, studioID).Doc(sc.ID).Delete(ctx); derr != nil {
			return derr
		}
		if len(sc.MediaIDs) > 0 {
			_ = AdjustAssetRefCounts(ctx, brandID, sc.MediaIDs, -1)
		}
	}
	// The runs go too. Firestore does not delete subcollections with their
	// parent, so without this a deleted studio leaves every plan ever made for
	// it behind — invisible, billable, and still holding the brief and script
	// in planJson.
	//
	// Before the studio doc, so a failure here leaves a studio that can be
	// deleted again rather than an unreachable pile of runs.
	if rerr := DeleteStudioRuns(ctx, brandID, contentID, studioID); rerr != nil {
		return rerr
	}

	if _, err := studioDoc(brandID, contentID, studioID).Delete(ctx); err != nil {
		return err
	}

	// The lanes' media, released last and only once the document is actually
	// gone — the same ordering the scenes above use, and for the same reason.
	if studio != nil {
		if ids := LaneMediaIDs(studio.Audio); len(ids) > 0 {
			_ = AdjustAssetRefCounts(ctx, brandID, ids, -1)
		}
	}
	return nil
}

// ListStudios returns a content's studios, newest first.
//
// Normally one. The plural exists because per-platform variations may one day
// want their own visuals, and a single-studio assumption baked into the path
// would be the expensive thing to undo.
func ListStudios(ctx context.Context, brandID, contentID string, limit int) ([]Studio, error) {
	if limit <= 0 {
		limit = 10
	}
	iter := studiosCollection(brandID, contentID).
		OrderBy("createdAt", firestore.Desc).Limit(limit).Documents(ctx)
	defer iter.Stop()

	out := []Studio{}
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return out, err
		}
		var s Studio
		if err := doc.DataTo(&s); err != nil {
			continue
		}
		s.ID = doc.Ref.ID
		out = append(out, s)
	}
	return out, nil
}

// SetContentStudioRef caches the pointer on the content document.
func SetContentStudioRef(ctx context.Context, brandID, contentID string, ref *StudioRef) error {
	if ref != nil && ref.UpdatedAt == 0 {
		ref.UpdatedAt = time.Now().UnixMilli()
	}
	_, err := firestoredb.Client.
		Collection(fmt.Sprintf("brands/%s/contents", brandID)).Doc(contentID).
		Set(ctx, map[string]interface{}{
			"studioRef": ref,
			"updatedAt": time.Now().UnixMilli(),
		}, firestore.MergeAll)
	return err
}
