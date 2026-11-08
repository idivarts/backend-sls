package studio

// The small decisions the handlers lean on: canvas sizes, defaults, what counts
// as valid, and the one response shape every mutation returns.

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
)

// canvasFor is the pixel size a format is authored at.
//
// 1080 on the SHORT side, matching what every design the AI writes already uses.
// It is here rather than inlined because a studio authored from scratch has to
// agree with the designs it may later be mixed with: two scenes at different
// sizes in one timeline means one of them is scaled, and which one is a coin
// flip.
func canvasFor(format string) (int, int) {
	switch format {
	case "reel", "story":
		return 1080, 1920
	case "video":
		return 1920, 1080
	case "carousel", "post":
		return 1080, 1350
	default:
		return 1080, 1920
	}
}

func knownFormat(f string) bool {
	switch f {
	case "reel", "video", "post", "carousel", "story":
		return true
	}
	return false
}

// defaultPreset: snappy for short vertical formats, soft for long video.
//
// The preset matters more than the individual transition kinds — a timeline
// that picks a different one per boundary looks like a slideshow template, and
// one preset applied throughout looks like an edit — so it has a default from
// the first scene rather than waiting for someone to choose.
func defaultPreset(format string) string {
	switch format {
	case "reel", "story":
		return trendlymodels.PresetSnappy
	case "video":
		return trendlymodels.PresetSoft
	default:
		return trendlymodels.PresetNone
	}
}

func knownPreset(p string) bool {
	switch p {
	case trendlymodels.PresetNone, trendlymodels.PresetSnappy, trendlymodels.PresetSoft,
		trendlymodels.PresetGraphic, trendlymodels.PresetEnergetic:
		return true
	}
	return false
}

func knownTransition(k string) bool {
	switch k {
	case trendlymodels.TransitionCut, trendlymodels.TransitionDissolve, trendlymodels.TransitionDip,
		trendlymodels.TransitionSlide, trendlymodels.TransitionWipe,
		trendlymodels.TransitionBlurDissolve, trendlymodels.TransitionWhip:
		return true
	}
	return false
}

func knownSlot(s string) bool {
	switch s {
	case trendlymodels.SlotFull, trendlymodels.SlotLeft, trendlymodels.SlotRight,
		trendlymodels.SlotTop, trendlymodels.SlotBottom,
		trendlymodels.SlotQ1, trendlymodels.SlotQ2, trendlymodels.SlotQ3, trendlymodels.SlotQ4,
		trendlymodels.SlotPipTL, trendlymodels.SlotPipTR, trendlymodels.SlotPipBL,
		trendlymodels.SlotPipBR, trendlymodels.SlotLowerThird, trendlymodels.SlotCustom:
		return true
	}
	return false
}

// defaultSceneMs is how long a hand-added scene is.
//
// A carousel scene has no clock at all, so it is zero — the number only exists
// so nothing divides by it.
func defaultSceneMs(format string) int {
	switch format {
	case "reel", "story":
		return 3000
	case "video":
		return 5000
	default:
		return 0
	}
}

// validateLayers refuses a stack that cannot render.
//
// Checked here rather than at render time because a scene saved in a state the
// renderer will reject is a scene the user finds out about minutes later, after
// paying for the render.
func validateLayers(layers []trendlymodels.StudioLayer) string {
	if len(layers) > 6 {
		// Not a technical ceiling. Six layers in one moment is already more
		// than a viewer can read, and every extra one is another compositing
		// pass at render time.
		return "A scene can hold at most six layers."
	}
	seen := map[string]bool{}
	for i, l := range layers {
		if l.ID == "" {
			return "Every layer needs an id."
		}
		if seen[l.ID] {
			return "Two layers share the same id."
		}
		seen[l.ID] = true

		if !knownSlot(l.Slot) {
			return "Unknown slot on layer " + l.ID + "."
		}
		switch l.Kind {
		case "media":
			if l.MediaID == "" {
				return "A media layer needs a mediaId."
			}
		case "html":
			if strings.TrimSpace(l.HTML) == "" {
				return "An HTML layer needs a document."
			}
		default:
			return "A layer is either media or html."
		}
		if l.Slot == trendlymodels.SlotCustom && l.Rect == nil {
			return "A custom slot needs a rectangle."
		}
		if l.Rect != nil {
			r := *l.Rect
			if r.W <= 0 || r.H <= 0 || r.X < 0 || r.Y < 0 || r.X+r.W > 1.0001 || r.Y+r.H > 1.0001 {
				// Fractions of the canvas, so a layer survives the canvas
				// changing size. A rectangle outside 0..1 is either pixels by
				// mistake or a layer placed off screen.
				return "A layer's rectangle is in fractions of the canvas, inside 0 to 1."
			}
		}
		if l.TrimOutMs > 0 && l.TrimOutMs <= l.TrimInMs {
			return "A trim has to end after it starts."
		}
		_ = i
	}
	return ""
}

// validateNarration refuses a line with no stated provenance.
//
// ⚠️ This is the script-provenance rule, enforced at the edge as well as in the
// planner. Trendly's first brand rule is that a claim has to be one we can
// stand behind, so a narrated line must say where it came from — and an
// invented scenario must say that it is one.
func validateNarration(n *trendlymodels.SceneNarration) string {
	if n == nil {
		return ""
	}
	if strings.TrimSpace(n.Line) == "" {
		// Clearing narration is done by sending an empty object, which is
		// legitimate; the source is only required once there are words.
		return ""
	}
	switch n.Source {
	case trendlymodels.NarrationFromScript, trendlymodels.NarrationFromCaption,
		trendlymodels.NarrationParaphrase, trendlymodels.NarrationIllustrative:
	case trendlymodels.NarrationFromBrand:
		if n.BrandFactID == "" {
			return "A line attributed to the brand has to name which fact."
		}
	default:
		return "Say where this line came from: the script, the caption, a brand fact, " +
			"a paraphrase of one of those, or an invented illustration."
	}
	return ""
}

// primaryKindFor derives what a scene IS from its stack.
//
// layers[0] decides. A media layer needs the library row to know whether it is
// video or an image, which is why this takes a context — and why a scene whose
// media has gone missing reads as empty rather than guessing.
func primaryKindFor(ctx context.Context, brandID string, layers []trendlymodels.StudioLayer) string {
	if len(layers) == 0 {
		return trendlymodels.SceneEmpty
	}
	first := layers[0]
	if first.Kind == "html" {
		// The caller that wrote the HTML knows whether it animates. Motion is
		// the safe default: rendering an animation as a still loses the
		// movement, where capturing frames of a static document is merely
		// wasteful.
		return trendlymodels.SceneMotion
	}
	if first.MediaID == "" {
		return trendlymodels.SceneEmpty
	}
	a, err := trendlymodels.GetAsset(ctx, brandID, first.MediaID)
	if err != nil || a == nil {
		return trendlymodels.SceneImage
	}
	if a.Kind == trendlymodels.AssetKindVideo {
		return trendlymodels.SceneVideo
	}
	return trendlymodels.SceneImage
}

func indexOf(ids []string, id string) int {
	for i, s := range ids {
		if s == id {
			return i
		}
	}
	return len(ids) - 1
}

func nowMs() int64 { return time.Now().UnixMilli() }

// respondWithStudio is the ONE response shape.
//
// Every mutation returns the whole studio plus its measured length, rather than
// just an ok. The client then has no reason to maintain its own idea of the
// order or the duration — which is the habit that produced two disagreeing
// timelines in the first place.
func respondWithStudio(c *gin.Context, brandID, contentID string, s *trendlymodels.Studio) {
	respondWithStudioStatus(c, http.StatusOK, brandID, contentID, s)
}

// respondWithStudioStatus is the same answer under a chosen status code.
//
// 202 exists for the paths that have ACCEPTED work rather than done it — a
// queued scene design, for instance. The body is identical either way, because
// the client wants the studio as it now reads regardless; only the code says
// whether anything is still coming.
func respondWithStudioStatus(c *gin.Context, status int, brandID, contentID string, s *trendlymodels.Studio) {
	ctx := c.Request.Context()
	scenes, err := trendlymodels.ListStudioScenes(ctx, brandID, contentID, s.ID)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not read the scenes"})
		return
	}
	m := trendlymodels.MeasureStudio(s, scenes)

	// Keep the cached pointer honest on every read. It is what the calendar
	// card draws, and a stale "6 scenes" on a studio with two is the kind of
	// wrongness people stop trusting the whole screen over.
	_ = trendlymodels.SetContentStudioRef(ctx, brandID, contentID, &trendlymodels.StudioRef{
		StudioID:   s.ID,
		SceneCount: len(m.Scenes),
		DurationMs: m.TotalMs,
		RenderURL:  s.RenderURL,
	})

	c.JSON(status, gin.H{
		"studio": s,
		"scenes": m.Scenes,
		"timeline": gin.H{
			"totalMs": m.TotalMs,
			"starts":  m.Starts,
		},
	})
}

func reloadAndRespond(c *gin.Context, brandID, contentID, studioID string) {
	reloadAndRespondWith(c, http.StatusOK, brandID, contentID, studioID)
}

// reloadAndRespondWith re-reads the studio and answers under a chosen code.
func reloadAndRespondWith(c *gin.Context, status int, brandID, contentID, studioID string) {
	s, err := trendlymodels.GetStudio(c.Request.Context(), brandID, contentID, studioID)
	if err != nil || s == nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not re-read the studio"})
		return
	}
	respondWithStudioStatus(c, status, brandID, contentID, s)
}
