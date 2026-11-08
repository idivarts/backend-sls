package trendlymodels

// Scenes: reads, writes, and the measurement everything else trusts.
//
// Two rules are enforced here rather than left to callers, because both were
// bugs in the old studio:
//
//  1. A SCENE WRITE MOVES MEDIA REFERENCE COUNTS. Placing media in a scene
//     increments it, clearing it decrements. Left to the caller, half the
//     paths forget, and the library then either refuses to archive anything or
//     archives something still on screen.
//  2. TRANSITIONS SHORTEN THE TIMELINE. Each one overlaps its two neighbours,
//     so the total is the sum of the scenes MINUS every overlap. Getting it
//     backwards gives a timeline that drifts longer with every dissolve and a
//     render whose audio no longer matches its picture.

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// SpeechCharsPerSecond is how fast speech runs.
//
// A measured-enough average for English at a natural pace. It is used to REFUSE
// a line that cannot be said inside its scene, so erring fast would let
// overruns through — which is why it is not higher.
const SpeechCharsPerSecond = 14

// NarrationTailMs is a short pause so a line does not end exactly on the cut.
const NarrationTailMs = 300

// CreateStudioScene writes a scene and appends it to the studio's order.
//
// The append is part of creating a scene, not a second call: a scene that
// exists but is not in sceneOrder is a document nobody can reach, and making
// the caller do it in two steps guarantees that state exists eventually.
func CreateStudioScene(ctx context.Context, brandID, contentID, studioID string, sc *StudioScene) (string, error) {
	now := time.Now().UnixMilli()
	if sc.CreatedAt == 0 {
		sc.CreatedAt = now
	}
	sc.UpdatedAt = now
	if sc.Layers == nil {
		sc.Layers = []StudioLayer{}
	}
	sc.MediaIDs = SceneMediaIDs(sc)
	if sc.PrimaryKind == "" {
		sc.PrimaryKind = SceneEmpty
	}

	ref, _, err := scenesCollection(brandID, contentID, studioID).Add(ctx, sc)
	if err != nil {
		return "", fmt.Errorf("CreateStudioScene: %w", err)
	}

	studio, err := GetStudio(ctx, brandID, contentID, studioID)
	if err == nil && studio != nil {
		order := append(studio.SceneOrder, ref.ID)
		if oerr := SetSceneOrder(ctx, brandID, contentID, studioID, order); oerr != nil {
			return ref.ID, oerr
		}
	}

	if len(sc.MediaIDs) > 0 {
		_ = AdjustAssetRefCounts(ctx, brandID, sc.MediaIDs, 1)
	}
	return ref.ID, nil
}

// InsertStudioScene writes a scene at a POSITION in the order.
//
// "Add an empty scene in the middle" is an explicit requirement, and appending
// then reordering would briefly show the scene at the end — which, on a rail
// the user is watching, reads as the wrong thing happening and then correcting
// itself.
func InsertStudioScene(ctx context.Context, brandID, contentID, studioID string, at int, sc *StudioScene) (string, error) {
	studio, err := GetStudio(ctx, brandID, contentID, studioID)
	if err != nil {
		return "", err
	}
	if studio == nil {
		return "", fmt.Errorf("InsertStudioScene: no studio %s", studioID)
	}

	now := time.Now().UnixMilli()
	if sc.CreatedAt == 0 {
		sc.CreatedAt = now
	}
	sc.UpdatedAt = now
	if sc.Layers == nil {
		sc.Layers = []StudioLayer{}
	}
	sc.MediaIDs = SceneMediaIDs(sc)
	if sc.PrimaryKind == "" {
		sc.PrimaryKind = SceneEmpty
	}

	ref, _, err := scenesCollection(brandID, contentID, studioID).Add(ctx, sc)
	if err != nil {
		return "", err
	}

	order := studio.SceneOrder
	if at < 0 || at > len(order) {
		at = len(order)
	}
	next := make([]string, 0, len(order)+1)
	next = append(next, order[:at]...)
	next = append(next, ref.ID)
	next = append(next, order[at:]...)
	if oerr := SetSceneOrder(ctx, brandID, contentID, studioID, next); oerr != nil {
		return ref.ID, oerr
	}

	if len(sc.MediaIDs) > 0 {
		_ = AdjustAssetRefCounts(ctx, brandID, sc.MediaIDs, 1)
	}
	return ref.ID, nil
}

// GetStudioScene reads one scene, or (nil, nil) when absent.
func GetStudioScene(ctx context.Context, brandID, contentID, studioID, sceneID string) (*StudioScene, error) {
	doc, err := scenesCollection(brandID, contentID, studioID).Doc(sceneID).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, err
	}
	var sc StudioScene
	if err := doc.DataTo(&sc); err != nil {
		return nil, err
	}
	sc.ID = doc.Ref.ID
	return &sc, nil
}

// ListStudioScenes returns every scene of a studio, UNORDERED.
//
// Unordered on purpose: the order is on the studio, and a read that sorted by
// `index` here would quietly make the cache authoritative. Callers pass the
// result through OrderedScenes.
func ListStudioScenes(ctx context.Context, brandID, contentID, studioID string) ([]StudioScene, error) {
	iter := scenesCollection(brandID, contentID, studioID).Documents(ctx)
	defer iter.Stop()

	out := []StudioScene{}
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return out, err
		}
		var sc StudioScene
		if err := doc.DataTo(&sc); err != nil {
			continue
		}
		sc.ID = doc.Ref.ID
		out = append(out, sc)
	}
	return out, nil
}

// SetSceneLayers replaces a scene's stack and moves the reference counts by the
// DIFFERENCE.
//
// The difference, not a clear-and-re-add: swapping one layer of a two-layer
// scene would otherwise drop the untouched layer's media to zero for an instant,
// and an archive sweep landing in that instant deletes something on screen.
func SetSceneLayers(ctx context.Context, brandID, contentID, studioID, sceneID string,
	layers []StudioLayer, primaryKind string) error {

	before, err := GetStudioScene(ctx, brandID, contentID, studioID, sceneID)
	if err != nil {
		return err
	}
	if before == nil {
		return fmt.Errorf("SetSceneLayers: no scene %s", sceneID)
	}

	if layers == nil {
		layers = []StudioLayer{}
	}
	// The narration comes along even though this call does not touch it:
	// SceneMediaIDs counts the spoken line's audio, and a `next` built from
	// layers alone describes a scene with no voice — so the diff below would
	// release the voiceover of any scene whose pictures were edited.
	next := &StudioScene{Layers: layers, Narration: before.Narration}
	nextIDs := SceneMediaIDs(next)

	updates := []firestore.Update{
		{Path: "layers", Value: layers},
		{Path: "mediaIds", Value: nextIDs},
		{Path: "updatedAt", Value: time.Now().UnixMilli()},
	}
	if primaryKind != "" {
		updates = append(updates, firestore.Update{Path: "primaryKind", Value: primaryKind})
	}
	if _, err := scenesCollection(brandID, contentID, studioID).Doc(sceneID).Update(ctx, updates); err != nil {
		return err
	}

	added, removed := diffStrings(before.MediaIDs, nextIDs)
	if len(added) > 0 {
		_ = AdjustAssetRefCounts(ctx, brandID, added, 1)
	}
	if len(removed) > 0 {
		_ = AdjustAssetRefCounts(ctx, brandID, removed, -1)
	}
	return nil
}

// UpdateStudioScene applies partial updates to a scene.
//
// Refuses the two fields that have to move reference counts with them, for the
// same reason UpdateAsset refuses the lifecycle fields: a layer list written
// without its counts is a library that can no longer be tidied.
func UpdateStudioScene(ctx context.Context, brandID, contentID, studioID, sceneID string,
	updates []firestore.Update) error {

	for _, u := range updates {
		// Both spellings — see updatePath. This guard predates that helper and
		// had the same hole.
		switch p := updatePath(u); p {
		case "layers", "mediaIds":
			return fmt.Errorf("UpdateStudioScene: %q moves media reference counts; use SetSceneLayers", p)
		case "narration":
			return fmt.Errorf("UpdateStudioScene: %q moves media reference counts; use SetSceneNarration", p)
		}
	}
	updates = append(updates, firestore.Update{Path: "updatedAt", Value: time.Now().UnixMilli()})
	_, err := scenesCollection(brandID, contentID, studioID).Doc(sceneID).Update(ctx, updates)
	return err
}

// DeleteStudioScene removes a scene, takes it out of the order, and releases
// its media.
func DeleteStudioScene(ctx context.Context, brandID, contentID, studioID, sceneID string) error {
	sc, err := GetStudioScene(ctx, brandID, contentID, studioID, sceneID)
	if err != nil {
		return err
	}

	studio, serr := GetStudio(ctx, brandID, contentID, studioID)
	if serr == nil && studio != nil {
		next := make([]string, 0, len(studio.SceneOrder))
		for _, id := range studio.SceneOrder {
			if id != sceneID {
				next = append(next, id)
			}
		}
		if oerr := SetSceneOrder(ctx, brandID, contentID, studioID, next); oerr != nil {
			return oerr
		}
	}

	if _, err := scenesCollection(brandID, contentID, studioID).Doc(sceneID).Delete(ctx); err != nil {
		return err
	}
	if sc != nil && len(sc.MediaIDs) > 0 {
		_ = AdjustAssetRefCounts(ctx, brandID, sc.MediaIDs, -1)
	}
	return nil
}

// SetSceneNarration replaces a scene's spoken line and moves the reference
// count of the audio it points at.
//
// Narration goes through its own function for the same reason layers do: the
// line carries a mediaId, so writing it changes which assets the scene holds a
// reference to, and the only correct way to move a count is by the DIFFERENCE
// between what was there and what is there now. Clearing and re-adding would
// drop an unchanged voiceover to zero for an instant, and an archive sweep
// landing in that instant deletes something a scene is speaking.
//
// A nil narration clears the line, which releases its audio.
func SetSceneNarration(ctx context.Context, brandID, contentID, studioID, sceneID string,
	narration *SceneNarration) error {

	before, err := GetStudioScene(ctx, brandID, contentID, studioID, sceneID)
	if err != nil {
		return err
	}
	if before == nil {
		return fmt.Errorf("SetSceneNarration: no scene %s", sceneID)
	}

	next := &StudioScene{Layers: before.Layers, Narration: narration}
	nextIDs := SceneMediaIDs(next)

	if _, err := scenesCollection(brandID, contentID, studioID).Doc(sceneID).Update(ctx, []firestore.Update{
		{Path: "narration", Value: narration},
		{Path: "mediaIds", Value: nextIDs},
		{Path: "updatedAt", Value: time.Now().UnixMilli()},
	}); err != nil {
		return err
	}

	added, removed := diffStrings(before.MediaIDs, nextIDs)
	if len(added) > 0 {
		_ = AdjustAssetRefCounts(ctx, brandID, added, 1)
	}
	if len(removed) > 0 {
		_ = AdjustAssetRefCounts(ctx, brandID, removed, -1)
	}
	return nil
}

// ── Pure helpers: measurement and derivation ────────────────────────────────

// SceneMediaIDs is every piece of media a scene references, deduped.
//
// Deduped because the same media placed twice in one scene must count once, or
// clearing the scene leaves the count stuck above zero and the media can never
// be archived.
//
// ⚠️ THE NARRATION'S AUDIO COUNTS. It used to not, and the consequence was that
// a generated voiceover — the one thing in the studio the brand has actually
// paid cash for — had a reference count of zero while a scene was speaking it,
// so an archive sweep was free to take it. The export now reads that audio and
// REFUSES to render a scene whose line has gone, which turns a silent loss into
// a blocked publish; making it un-archivable is the other half of that.
//
// Every caller therefore has to hand over a scene with its narration attached.
// A caller that passes layers alone computes a set that is missing the voice,
// and the diff against it releases the voice — see SetSceneLayers.
func SceneMediaIDs(sc *StudioScene) []string {
	if sc == nil {
		return nil
	}
	ids := make([]string, 0, len(sc.Layers)+1)
	for _, l := range sc.Layers {
		if l.MediaID != "" {
			ids = append(ids, l.MediaID)
		}
	}
	if sc.Narration != nil && sc.Narration.MediaID != "" {
		ids = append(ids, sc.Narration.MediaID)
	}
	return dedupeStrings(ids)
}

// OrderedScenes puts scenes in the studio's order.
//
// A scene that exists but is NOT in sceneOrder is appended rather than hidden:
// that state means a write half landed, and the user can see and delete a
// stray scene where a hidden one is a document nobody can reach.
func OrderedScenes(studio *Studio, scenes []StudioScene) []StudioScene {
	if studio == nil {
		return scenes
	}
	byID := make(map[string]StudioScene, len(scenes))
	for _, sc := range scenes {
		byID[sc.ID] = sc
	}
	out := make([]StudioScene, 0, len(scenes))
	placed := make(map[string]bool, len(scenes))
	for _, id := range studio.SceneOrder {
		if sc, ok := byID[id]; ok {
			out = append(out, sc)
			placed[id] = true
		}
	}
	for _, sc := range scenes {
		if !placed[sc.ID] {
			out = append(out, sc)
		}
	}
	return out
}

// NarrationMs is how long a line takes to say.
func NarrationMs(line string) int {
	chars := len([]rune(trimSpace(line)))
	if chars == 0 {
		return 0
	}
	return (chars*1000)/SpeechCharsPerSecond + NarrationTailMs
}

// EffectiveSceneMs is a scene's length once narration has had its say.
//
// FitToNarration stretches the scene to hold the line. Without it the stored
// duration stands, and an over-long line is a validation failure rather than a
// silent overrun of the picture.
func EffectiveSceneMs(sc StudioScene) int {
	base := sc.DurationMs
	if base < 0 {
		base = 0
	}
	n := sc.Narration
	if n == nil || n.Line == "" || !n.FitToNarration {
		return base
	}
	need := NarrationMs(n.Line) + n.OffsetMs
	if need > base {
		return need
	}
	return base
}

// TransitionOverlapMs is how much time a transition takes off a boundary.
//
// Clamped to half the SHORTER neighbour: a 1s dissolve between two 600ms scenes
// would consume more than exists, and the clamp is what stops a generated
// timeline collapsing when the planner asks for a cinematic cross-fade between
// two quick beats.
func TransitionOverlapMs(previous, incoming StudioScene) int {
	t := incoming.TransitionIn
	if t == nil || t.Kind == TransitionCut || t.DurationMs <= 0 {
		return 0
	}
	shorter := EffectiveSceneMs(previous)
	if o := EffectiveSceneMs(incoming); o < shorter {
		shorter = o
	}
	half := shorter / 2
	if t.DurationMs < half {
		return t.DurationMs
	}
	return half
}

// StudioMeasurement is the timeline as it will actually render.
type StudioMeasurement struct {
	TotalMs int
	// Starts is each scene's position on the timeline, after overlaps.
	Starts map[string]int
	Scenes []StudioScene
}

// MeasureStudio is the one definition of how long a studio is.
//
// Everything that charges, publishes or renders asks this rather than reading
// the cached `studioRef.durationMs`, because that one is client-writable and
// this number is what gets billed.
func MeasureStudio(studio *Studio, scenes []StudioScene) StudioMeasurement {
	ordered := OrderedScenes(studio, scenes)
	out := StudioMeasurement{Starts: make(map[string]int, len(ordered)), Scenes: ordered}

	cursor := 0
	for i, sc := range ordered {
		if i > 0 {
			cursor -= TransitionOverlapMs(ordered[i-1], sc)
			if cursor < 0 {
				cursor = 0
			}
		}
		out.Starts[sc.ID] = cursor
		cursor += EffectiveSceneMs(sc)
	}
	out.TotalMs = cursor
	return out
}

// diffStrings returns what is in next and not before, and vice versa.
func diffStrings(before, next []string) (added, removed []string) {
	inBefore := make(map[string]bool, len(before))
	for _, s := range before {
		inBefore[s] = true
	}
	inNext := make(map[string]bool, len(next))
	for _, s := range next {
		inNext[s] = true
	}
	for s := range inNext {
		if !inBefore[s] {
			added = append(added, s)
		}
	}
	for s := range inBefore {
		if !inNext[s] {
			removed = append(removed, s)
		}
	}
	return added, removed
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\n' || s[start] == '\t' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\n' || s[end-1] == '\t' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}

// SetSceneGeneratedMedia puts a finished generation into its scene.
//
// It REPLACES the stack rather than adding to it: a generated clip is a TAKE,
// and appending would leave the previous attempt underneath, visible wherever
// the new one does not cover it.
//
// The scene's own duration is left alone. The clip was generated to the length
// the scene asked for, and overwriting the scene with the clip's actual length
// would move every later scene on the timeline because a provider rounded four
// seconds to 4.04.
func SetSceneGeneratedMedia(ctx context.Context, brandID, contentID, studioID, sceneID, mediaID string) (bool, error) {
	sc, err := GetStudioScene(ctx, brandID, contentID, studioID, sceneID)
	if err != nil {
		return false, err
	}
	if sc == nil {
		// By the time a generation finishes the user may well have deleted the
		// scene. An ordinary outcome, not an error — and the media is in the
		// library either way, so nothing they paid for is lost.
		return false, nil
	}

	layers := []StudioLayer{{
		ID:      fmt.Sprintf("l_gen_%d", time.Now().UnixMilli()),
		Kind:    "media",
		Slot:    SlotFull,
		MediaID: mediaID,
		Fit:     "cover",
	}}
	if err := SetSceneLayers(ctx, brandID, contentID, studioID, sceneID, layers, SceneVideo); err != nil {
		return false, err
	}
	if err := UpdateStudioScene(ctx, brandID, contentID, studioID, sceneID, []firestore.Update{
		{Path: "pending", Value: false},
		{Path: "gen.status", Value: "done"},
	}); err != nil {
		return true, err
	}
	// The clip landed, so this may have been the run's last outstanding scene.
	_ = SettleStudioRunIfComplete(ctx, brandID, contentID, studioID, sc.RunID)
	return true, nil
}

// MarkSceneGenerationFailed records that a scene's generation did not produce a
// clip, so the rail can show it as failed rather than as forever pending.
func MarkSceneGenerationFailed(ctx context.Context, brandID, contentID, studioID, sceneID, reason string) error {
	if err := UpdateStudioScene(ctx, brandID, contentID, studioID, sceneID, []firestore.Update{
		{Path: "pending", Value: false},
		{Path: "gen.status", Value: "failed"},
		{Path: "gen.error", Value: reason},
	}); err != nil {
		return err
	}
	// A run whose last scene FAILED is just as finished as one whose last
	// scene worked. Settling only on success is how a run full of failures
	// stays `running` forever.
	sc, err := GetStudioScene(ctx, brandID, contentID, studioID, sceneID)
	if err != nil || sc == nil {
		return nil
	}
	_ = SettleStudioRunIfComplete(ctx, brandID, contentID, studioID, sc.RunID)
	return nil
}
