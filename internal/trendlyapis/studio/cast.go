package studio

// Cast and locations — the mechanism that keeps one face and one room across
// separate generated clips.
//
// ── Why they are documents, not prompt text ─────────────────────────────────
// "The founder, mid-thirties, navy jumper" repeated in six prompts produces six
// different people, because each generation reads the description freshly. A
// cast member is a row so that:
//
//   - its `look` is appended VERBATIM to every prompt it appears in, rather
//     than re-typed and quietly paraphrased;
//   - one approved still can be referenced by every clip, which is the only
//     mechanism that actually holds a face;
//   - the system can COUNT appearances, and refuse a third generated clip
//     without an anchor instead of letting the drift happen and then being
//     asked why.
//
// ── Why an anchor is picked rather than described ───────────────────────────
// The anchor is media: a frame cut out of a clip that came out well, an
// uploaded photo, or a generated still. It has to be a real asset in the
// library, because a reference the provider can fetch is the whole point — and
// because an anchor nobody looked at is not approved, it is just the first
// thing the model happened to make.

import (
	"net/http"
	"strings"

	"cloud.google.com/go/firestore"
	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
)

type setCastRequest struct {
	Cast []trendlymodels.CastMember `json:"cast"`
}

// SetCast replaces the studio's cast.
// PUT …/studios/:studioId/cast
//
// The whole list, because the cast is one fact about the document and a
// per-member endpoint invites two clients to disagree about who is in it.
func SetCast(c *gin.Context) {
	brandID, contentID, studioID, ok := guard(c, true)
	if !ok {
		return
	}
	var req setCastRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()
	seen := map[string]bool{}
	for i := range req.Cast {
		m := &req.Cast[i]
		m.ID = strings.TrimSpace(m.ID)
		if m.ID == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "cast_needs_id",
				"message": "Every cast member needs an id.",
			})
			return
		}
		if seen[m.ID] {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "duplicate_cast",
				"message": "Two cast members share the same id.",
			})
			return
		}
		seen[m.ID] = true

		m.Description = strings.TrimSpace(m.Description)
		m.Look = strings.TrimSpace(m.Look)
		if m.Description == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "cast_needs_description",
				"message": "Say who this is, so the plan is readable by a person.",
			})
			return
		}
		if m.Look == "" {
			// Refused rather than defaulted. The `look` is what is appended to
			// every prompt; an empty one means the clips have nothing holding
			// them together except the description, which is the failure this
			// whole mechanism exists to prevent.
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "cast_needs_look",
				"message": "Describe how they look — hair, clothing, age — in a phrase. " +
					"It is appended to every prompt word for word, which is what keeps " +
					"them the same person across clips.",
			})
			return
		}

		// An anchor has to be real, a still, and usable as a model input.
		if m.AnchorMediaID != "" {
			a, err := trendlymodels.GetAsset(ctx, brandID, m.AnchorMediaID)
			if err != nil || a == nil {
				c.JSON(http.StatusBadRequest, gin.H{
					"error":   "anchor_not_found",
					"message": "That anchor isn't in your media.",
				})
				return
			}
			if a.Kind != trendlymodels.AssetKindImage {
				// A clip cannot anchor a face: the provider takes a still, and
				// "the whole clip" has no single frame to be the reference.
				c.JSON(http.StatusBadRequest, gin.H{
					"error": "anchor_not_a_still",
					"message": "An anchor has to be a still. Take a frame out of the clip " +
						"and use that.",
				})
				return
			}
			if a.NoAIInput {
				c.JSON(http.StatusBadRequest, gin.H{
					"error": "anchor_not_usable",
					"message": "That image's licence forbids using it as an AI reference. " +
						"Pick another.",
				})
				return
			}
		}
	}

	if err := trendlymodels.UpdateStudio(ctx, brandID, contentID, studioID, []firestore.Update{
		{Path: "cast", Value: req.Cast},
	}); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not save the cast"})
		return
	}
	reloadAndRespond(c, brandID, contentID, studioID)
}

type setLocationsRequest struct {
	Locations []trendlymodels.StudioLocation `json:"locations"`
}

// SetLocations replaces the studio's locations.
// PUT …/studios/:studioId/locations
//
// Same rule as cast, for the same reason: a place described freshly in each
// prompt is a different room each time, and a viewer notices a kitchen that
// changes layout between two shots of the same scene.
func SetLocations(c *gin.Context) {
	brandID, contentID, studioID, ok := guard(c, true)
	if !ok {
		return
	}
	var req setLocationsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()
	seen := map[string]bool{}
	for i := range req.Locations {
		l := &req.Locations[i]
		l.ID = strings.TrimSpace(l.ID)
		l.Description = strings.TrimSpace(l.Description)
		l.Look = strings.TrimSpace(l.Look)
		if l.ID == "" || l.Description == "" || l.Look == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "location_incomplete",
				"message": "A location needs an id, a description and a look — the look is " +
					"appended to every prompt word for word.",
			})
			return
		}
		if seen[l.ID] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "duplicate_location"})
			return
		}
		seen[l.ID] = true

		if l.AnchorMediaID != "" {
			a, err := trendlymodels.GetAsset(ctx, brandID, l.AnchorMediaID)
			if err != nil || a == nil || a.Kind != trendlymodels.AssetKindImage || a.NoAIInput {
				c.JSON(http.StatusBadRequest, gin.H{
					"error":   "anchor_not_usable",
					"message": "A location anchor has to be a still in your media that may be used as an AI reference.",
				})
				return
			}
		}
	}

	if err := trendlymodels.UpdateStudio(ctx, brandID, contentID, studioID, []firestore.Update{
		{Path: "locations", Value: req.Locations},
	}); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not save the locations"})
		return
	}
	reloadAndRespond(c, brandID, contentID, studioID)
}
