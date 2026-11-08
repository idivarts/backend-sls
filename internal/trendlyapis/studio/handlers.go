package studio

import (
	"fmt"
	"net/http"

	"cloud.google.com/go/firestore"
	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
)

// guard checks the caller may edit this brand's content and returns the ids.
//
// One helper rather than four lines in every handler, because the thing being
// checked is the same every time: authoring media is an edit of the post, so
// the calendar EDITOR privilege is the bar — not the publish privilege, which
// stays where it is.
func guard(c *gin.Context, editor bool) (brandID, contentID, studioID string, ok bool) {
	brandID = c.Param("brandId")
	priv := trendlymodels.PrivCalendarView
	if editor {
		priv = trendlymodels.PrivCalendarEditor
	}
	if _, allowed := middlewares.RequireFeaturePrivilege(c, brandID,
		trendlymodels.FeatureContentCalendar, priv); !allowed {
		return "", "", "", false
	}
	return brandID, c.Param("contentId"), c.Param("studioId"), true
}

type openStudioRequest struct {
	// Format decides the canvas and the transport: a carousel is paged with
	// arrows, a reel is scrubbed with a playhead. Defaults to the content's
	// own type, which is almost always what is wanted.
	Format string `json:"format"`
}

// OpenStudio returns the content's studio, creating an EMPTY one if it has none.
// POST /api/studio/brands/:brandId/contents/:contentId/open
//
// Idempotent: opening twice returns the same studio. The app calls this on
// every navigation to /design-studio/<contentId>, so it has to be.
func OpenStudio(c *gin.Context) {
	brandID, contentID, _, ok := guard(c, true)
	if !ok {
		return
	}
	var req openStudioRequest
	_ = c.ShouldBindJSON(&req)

	ctx := c.Request.Context()
	content, err := trendlymodels.GetContent(brandID, contentID)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not read the content"})
		return
	}
	if content == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no such content"})
		return
	}

	// An existing studio wins. Re-creating on every open would orphan the
	// scenes the user has already made, which is the worst possible failure
	// for a document people return to.
	if content.StudioRef != nil && content.StudioRef.StudioID != "" {
		if existing, gerr := trendlymodels.GetStudio(ctx, brandID, contentID, content.StudioRef.StudioID); gerr == nil && existing != nil {
			respondWithStudio(c, brandID, contentID, existing)
			return
		}
	}
	// Fall back to a listing before creating: a studio whose ref failed to
	// write is still a studio, and making a second one would hide the first.
	if list, lerr := trendlymodels.ListStudios(ctx, brandID, contentID, 1); lerr == nil && len(list) > 0 {
		respondWithStudio(c, brandID, contentID, &list[0])
		return
	}

	format := req.Format
	if format == "" {
		format = string(content.ContentFormat)
	}
	w, h := canvasFor(format)
	s := &trendlymodels.Studio{
		Format: format,
		Width:  w,
		Height: h,
		FPS:    30,
		// Empty. See the note on CreateStudio.
		SceneOrder: []string{},
		// Snappy for short vertical formats, soft for long video. The preset
		// is what makes a generated timeline look deliberate rather than like
		// seven unrelated decisions, so it has a default from the start.
		TransitionPreset: defaultPreset(format),
		CreatedBy:        c.GetString("userId"),
	}
	id, err := trendlymodels.CreateStudio(ctx, brandID, contentID, s)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not open the studio"})
		return
	}
	s.ID = id
	_ = trendlymodels.SetContentStudioRef(ctx, brandID, contentID, &trendlymodels.StudioRef{
		StudioID: id, SceneCount: 0, DurationMs: 0,
	})
	respondWithStudio(c, brandID, contentID, s)
}

// GetStudio returns a studio with its scenes, in order, and its real length.
// GET …/studios/:studioId
func GetStudio(c *gin.Context) {
	brandID, contentID, studioID, ok := guard(c, false)
	if !ok {
		return
	}
	s, err := trendlymodels.GetStudio(c.Request.Context(), brandID, contentID, studioID)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not read the studio"})
		return
	}
	if s == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no such studio"})
		return
	}
	respondWithStudio(c, brandID, contentID, s)
}

type patchStudioRequest struct {
	Format           *string                         `json:"format"`
	FPS              *int                            `json:"fps"`
	TransitionPreset *string                         `json:"transitionPreset"`
	Audio            *trendlymodels.StudioAudio      `json:"audio"`
	Cast             *[]trendlymodels.CastMember     `json:"cast"`
	Locations        *[]trendlymodels.StudioLocation `json:"locations"`
}

// PatchStudio changes the document-level settings.
// PATCH …/studios/:studioId
func PatchStudio(c *gin.Context) {
	brandID, contentID, studioID, ok := guard(c, true)
	if !ok {
		return
	}
	var req patchStudioRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updates := []firestore.Update{}
	if req.Format != nil {
		if !knownFormat(*req.Format) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unknown format"})
			return
		}
		w, h := canvasFor(*req.Format)
		updates = append(updates,
			firestore.Update{Path: "format", Value: *req.Format},
			// The canvas follows the format. Leaving the old size behind is
			// how a reel re-targeted to a square post renders letterboxed with
			// no indication of why.
			firestore.Update{Path: "width", Value: w},
			firestore.Update{Path: "height", Value: h},
		)
	}
	if req.FPS != nil && *req.FPS > 0 {
		updates = append(updates, firestore.Update{Path: "fps", Value: *req.FPS})
	}
	if req.TransitionPreset != nil {
		if !knownPreset(*req.TransitionPreset) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unknown transition preset"})
			return
		}
		updates = append(updates, firestore.Update{Path: "transitionPreset", Value: *req.TransitionPreset})
	}
	// Audio is written on its own, through the model function that moves the
	// lanes' media reference counts. It cannot ride in `updates` — UpdateStudio
	// refuses the field precisely so this path cannot be bypassed.
	audio := req.Audio
	if req.Cast != nil {
		updates = append(updates, firestore.Update{Path: "cast", Value: *req.Cast})
	}
	if req.Locations != nil {
		updates = append(updates, firestore.Update{Path: "locations", Value: *req.Locations})
	}
	if len(updates) == 0 && audio == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "nothing to change"})
		return
	}

	if len(updates) > 0 {
		if err := trendlymodels.UpdateStudio(c.Request.Context(), brandID, contentID, studioID, updates); err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "could not update the studio"})
			return
		}
	}
	if audio != nil {
		if err := trendlymodels.SetStudioAudio(c.Request.Context(), brandID, contentID, studioID, audio); err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "could not update the studio's sound"})
			return
		}
	}
	reloadAndRespond(c, brandID, contentID, studioID)
}

// DeleteStudio removes a studio, its scenes, and its media references.
// DELETE …/studios/:studioId
func DeleteStudio(c *gin.Context) {
	brandID, contentID, studioID, ok := guard(c, true)
	if !ok {
		return
	}
	if err := trendlymodels.DeleteStudio(c.Request.Context(), brandID, contentID, studioID); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not delete the studio"})
		return
	}
	_ = trendlymodels.SetContentStudioRef(c.Request.Context(), brandID, contentID, nil)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type addSceneRequest struct {
	// At is the position to insert at. Omitted appends. "Add an empty scene in
	// the middle" is an explicit requirement, and appending-then-reordering
	// would briefly show the scene at the end — which on a rail the user is
	// watching reads as the wrong thing happening and then correcting itself.
	At *int `json:"at"`
	// DurationMs defaults to the format's own default.
	DurationMs *int   `json:"durationMs"`
	Title      string `json:"title"`
	// Layers may be supplied so "add a scene from this media" is one call
	// rather than two — two would briefly show an empty scene.
	Layers []trendlymodels.StudioLayer `json:"layers"`
}

// AddScene inserts a scene. With no layers it is a deliberate empty placeholder.
// POST …/studios/:studioId/scenes
func AddScene(c *gin.Context) {
	brandID, contentID, studioID, ok := guard(c, true)
	if !ok {
		return
	}
	var req addSceneRequest
	_ = c.ShouldBindJSON(&req)

	ctx := c.Request.Context()
	s, err := trendlymodels.GetStudio(ctx, brandID, contentID, studioID)
	if err != nil || s == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no such studio"})
		return
	}

	if problem := validateLayers(req.Layers); problem != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_layers", "message": problem})
		return
	}

	duration := defaultSceneMs(s.Format)
	if req.DurationMs != nil && *req.DurationMs >= 0 {
		duration = *req.DurationMs
	}
	sc := &trendlymodels.StudioScene{
		DurationMs:  duration,
		Title:       req.Title,
		Layers:      req.Layers,
		PrimaryKind: primaryKindFor(ctx, brandID, req.Layers),
	}

	var id string
	if req.At != nil {
		id, err = trendlymodels.InsertStudioScene(ctx, brandID, contentID, studioID, *req.At, sc)
	} else {
		id, err = trendlymodels.CreateStudioScene(ctx, brandID, contentID, studioID, sc)
	}
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not add the scene"})
		return
	}

	c.Header("X-Scene-Id", id)
	reloadAndRespond(c, brandID, contentID, studioID)
}

type reorderRequest struct {
	Order []string `json:"order" binding:"required"`
}

// ReorderScenes replaces the order.
// POST …/studios/:studioId/scenes/order
//
// The whole array, never a move instruction. Order is ONE fact; a partial
// update to it is how two clients end up believing different sequences — which
// is the class of bug that made the old studio's two timelines disagree.
func ReorderScenes(c *gin.Context) {
	brandID, contentID, studioID, ok := guard(c, true)
	if !ok {
		return
	}
	var req reorderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()
	scenes, err := trendlymodels.ListStudioScenes(ctx, brandID, contentID, studioID)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not read the scenes"})
		return
	}

	// The submitted order has to be exactly the scenes that exist. A reorder
	// that silently dropped an id would delete a scene by omission, and one
	// that accepted an unknown id would put a hole in the timeline.
	exists := make(map[string]bool, len(scenes))
	for _, sc := range scenes {
		exists[sc.ID] = true
	}
	seen := make(map[string]bool, len(req.Order))
	for _, id := range req.Order {
		if !exists[id] {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "unknown_scene",
				"message": fmt.Sprintf("%s is not a scene of this studio", id),
			})
			return
		}
		if seen[id] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "duplicate_scene"})
			return
		}
		seen[id] = true
	}
	if len(req.Order) != len(scenes) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "incomplete_order",
			"message": fmt.Sprintf("the studio has %d scenes and the order lists %d", len(scenes), len(req.Order)),
		})
		return
	}

	if err := trendlymodels.SetSceneOrder(ctx, brandID, contentID, studioID, req.Order); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not reorder"})
		return
	}
	reloadAndRespond(c, brandID, contentID, studioID)
}

type patchSceneRequest struct {
	DurationMs   *int                          `json:"durationMs"`
	Title        *string                       `json:"title"`
	TransitionIn *trendlymodels.Transition     `json:"transitionIn"`
	Narration    *trendlymodels.SceneNarration `json:"narration"`
}

// PatchScene changes a scene's own settings.
// PATCH …/studios/:studioId/scenes/:sceneId
func PatchScene(c *gin.Context) {
	brandID, contentID, studioID, ok := guard(c, true)
	if !ok {
		return
	}
	var req patchSceneRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updates := []firestore.Update{}
	if req.DurationMs != nil {
		if *req.DurationMs < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "a scene cannot be shorter than nothing"})
			return
		}
		updates = append(updates, firestore.Update{Path: "durationMs", Value: *req.DurationMs})
	}
	if req.Title != nil {
		updates = append(updates, firestore.Update{Path: "title", Value: *req.Title})
	}
	if req.TransitionIn != nil {
		if !knownTransition(req.TransitionIn.Kind) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unknown transition"})
			return
		}
		updates = append(updates, firestore.Update{Path: "transitionIn", Value: req.TransitionIn})
	}
	// Narration is lifted OUT of the generic update: its mediaId holds a
	// reference to a piece of audio, so it has to go through the model function
	// that moves the count. UpdateStudioScene refuses the path outright.
	narration := req.Narration
	if narration != nil {
		if problem := validateNarration(narration); problem != "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_narration", "message": problem})
			return
		}
	}
	if len(updates) == 0 && narration == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "nothing to change"})
		return
	}

	sceneID := c.Param("sceneId")
	if len(updates) > 0 {
		if err := trendlymodels.UpdateStudioScene(c.Request.Context(), brandID, contentID, studioID,
			sceneID, updates); err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
	}
	if narration != nil {
		if err := trendlymodels.SetSceneNarration(c.Request.Context(), brandID, contentID, studioID,
			sceneID, narration); err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
	}
	reloadAndRespond(c, brandID, contentID, studioID)
}

type setLayersRequest struct {
	Layers []trendlymodels.StudioLayer `json:"layers"`
}

// SetSceneLayers replaces a scene's stack.
// PUT …/studios/:studioId/scenes/:sceneId/layers
//
// The whole stack, because the model moves reference counts by the DIFFERENCE
// and needs both sides to compute it. A per-layer endpoint would make the
// difference a client's job to describe, which is how a count goes wrong.
func SetSceneLayers(c *gin.Context) {
	brandID, contentID, studioID, ok := guard(c, true)
	if !ok {
		return
	}
	var req setLayersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if problem := validateLayers(req.Layers); problem != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_layers", "message": problem})
		return
	}

	ctx := c.Request.Context()
	kind := primaryKindFor(ctx, brandID, req.Layers)
	if err := trendlymodels.SetSceneLayers(ctx, brandID, contentID, studioID,
		c.Param("sceneId"), req.Layers, kind); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	reloadAndRespond(c, brandID, contentID, studioID)
}

// DeleteScene removes a scene and releases its media.
// DELETE …/studios/:studioId/scenes/:sceneId
func DeleteScene(c *gin.Context) {
	brandID, contentID, studioID, ok := guard(c, true)
	if !ok {
		return
	}
	if err := trendlymodels.DeleteStudioScene(c.Request.Context(), brandID, contentID, studioID,
		c.Param("sceneId")); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not delete the scene"})
		return
	}
	reloadAndRespond(c, brandID, contentID, studioID)
}

// DuplicateScene copies a scene in place, directly after the original.
// POST …/studios/:studioId/scenes/:sceneId/duplicate
//
// Also the answer to "split a motion scene", which is refused: splitting an
// animation produces two documents whose animations start mid-flight. Duplicate
// and shorten both gives the user the same outcome and nothing broken.
func DuplicateScene(c *gin.Context) {
	brandID, contentID, studioID, ok := guard(c, true)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	sceneID := c.Param("sceneId")

	src, err := trendlymodels.GetStudioScene(ctx, brandID, contentID, studioID, sceneID)
	if err != nil || src == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no such scene"})
		return
	}
	s, err := trendlymodels.GetStudio(ctx, brandID, contentID, studioID)
	if err != nil || s == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no such studio"})
		return
	}

	at := indexOf(s.SceneOrder, sceneID) + 1
	// Not named `copy`: that is a builtin, and shadowing it in a function that
	// also copies slices is how a later edit silently breaks.
	dup := *src
	dup.ID = ""
	dup.CreatedAt = 0
	dup.UpdatedAt = 0
	// A duplicate is a NEW take, not the same generation. Carrying the job id
	// over would make two scenes report the same in-flight generation, and a
	// failure on one would mark both failed.
	if dup.Gen != nil {
		g := *dup.Gen
		g.JobID = ""
		g.Status = ""
		g.Error = ""
		dup.Gen = &g
	}
	// Relayer with fresh ids so the two scenes' layers are not the same rows.
	layers := make([]trendlymodels.StudioLayer, 0, len(src.Layers))
	for i, l := range src.Layers {
		l.ID = fmt.Sprintf("l%d_%d", i, nowMs())
		layers = append(layers, l)
	}
	dup.Layers = layers

	if _, err := trendlymodels.InsertStudioScene(ctx, brandID, contentID, studioID, at, &dup); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not duplicate the scene"})
		return
	}
	reloadAndRespond(c, brandID, contentID, studioID)
}

type splitSceneRequest struct {
	// AtMs is the point inside the scene to cut, in scene time.
	AtMs int `json:"atMs" binding:"required"`
}

// SplitScene cuts one scene into two at a point inside it.
// POST …/studios/:studioId/scenes/:sceneId/split
//
// ⚠️ REFUSED for motion and composite scenes. Both are HTML documents with
// their own animation, and splitting one produces a second scene whose
// animation starts mid-flight — an effect nobody asked for and cannot see
// coming. Duplicate-and-shorten is offered instead, which gets the same result
// with nothing broken.
func SplitScene(c *gin.Context) {
	brandID, contentID, studioID, ok := guard(c, true)
	if !ok {
		return
	}
	var req splitSceneRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()
	sceneID := c.Param("sceneId")
	src, err := trendlymodels.GetStudioScene(ctx, brandID, contentID, studioID, sceneID)
	if err != nil || src == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no such scene"})
		return
	}

	if src.PrimaryKind == trendlymodels.SceneMotion || src.PrimaryKind == trendlymodels.SceneComposite {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "cannot_split_animation",
			"message": "This scene is an animation, so splitting it would leave the second half " +
				"starting mid-movement. Duplicate it and shorten both instead.",
		})
		return
	}
	if req.AtMs <= 0 || req.AtMs >= src.DurationMs {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "split_outside_scene",
			"message": "Pick a point inside the scene to cut at.",
		})
		return
	}

	s, err := trendlymodels.GetStudio(ctx, brandID, contentID, studioID)
	if err != nil || s == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no such studio"})
		return
	}

	// The second half inherits the trim, advanced by the cut. A split that
	// gave both halves the same trim would play the same footage twice, which
	// is the obvious bug and the easy one to ship.
	second := *src
	second.ID = ""
	second.CreatedAt = 0
	second.UpdatedAt = 0
	second.DurationMs = src.DurationMs - req.AtMs
	// The incoming transition belongs to the FIRST half; the halves of one
	// shot meet on a cut, because anything else dissolves a shot into itself.
	second.TransitionIn = &trendlymodels.Transition{Kind: trendlymodels.TransitionCut}
	secondLayers := make([]trendlymodels.StudioLayer, 0, len(src.Layers))
	for i, l := range src.Layers {
		l.ID = fmt.Sprintf("l%d_%d", i, nowMs())
		l.TrimInMs = l.TrimInMs + req.AtMs
		secondLayers = append(secondLayers, l)
	}
	second.Layers = secondLayers
	// Narration goes with the FIRST half: the line was written against the
	// start of the shot, and halving a sentence is worse than letting it run
	// over a cut that is now in the middle of it.
	second.Narration = nil

	at := indexOf(s.SceneOrder, sceneID) + 1
	if _, err := trendlymodels.InsertStudioScene(ctx, brandID, contentID, studioID, at, &second); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not split the scene"})
		return
	}

	// Shorten the original LAST, so a failure above leaves the timeline as it
	// was rather than with a truncated scene and no second half.
	firstLayers := make([]trendlymodels.StudioLayer, 0, len(src.Layers))
	for _, l := range src.Layers {
		l.TrimOutMs = l.TrimInMs + req.AtMs
		firstLayers = append(firstLayers, l)
	}
	if err := trendlymodels.SetSceneLayers(ctx, brandID, contentID, studioID, sceneID,
		firstLayers, src.PrimaryKind); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not trim the first half"})
		return
	}
	if err := trendlymodels.UpdateStudioScene(ctx, brandID, contentID, studioID, sceneID,
		[]firestore.Update{{Path: "durationMs", Value: req.AtMs}}); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not shorten the first half"})
		return
	}

	reloadAndRespond(c, brandID, contentID, studioID)
}
