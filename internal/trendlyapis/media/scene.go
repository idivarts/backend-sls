package media

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/scenegraph"
)

// Deterministic scene endpoints — text edits, seeding, and revert do NOT need
// the AI (per the ticket: text is directly editable, deterministic). The AI-
// driven comment→ops path lives in the chat engine (apply_scene_edits).

type generateSceneReq struct {
	DocType  string `json:"docType"` // image|video
	Format   string `json:"format"`  // post|reel|story|video|carousel
	Headline string `json:"headline"`
	Subhead  string `json:"subhead"`
	Cta      string `json:"cta"`
}

// GenerateSceneHTTP seeds a brand-kitted scene for a content and points it at it.
func GenerateSceneHTTP(c *gin.Context) {
	brandID := c.Param("brandId")
	contentID := c.Param("contentId")
	var req generateSceneReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	size := scenegraph.SizePost
	switch req.Format {
	case "reel", "story":
		size = scenegraph.SizeReel
	case "video":
		size = scenegraph.SizeVideo
	}
	bk := scenegraph.BrandKit{}
	var brand trendlymodels.Brand
	if err := brand.Get(brandID); err == nil && brand.Image != nil {
		bk.LogoURL = *brand.Image
	}
	var doc *scenegraph.Document
	if req.DocType == "video" {
		doc = scenegraph.SeedVideoScene(size, req.Headline, req.Cta, bk)
	} else {
		doc = scenegraph.SeedImageScene(size, req.Headline, req.Subhead, bk)
	}
	revID, err := persistScene(brandID, contentID, doc, "generate", "")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"revisionId": revID})
}

type applyOpsReq struct {
	Ops []scenegraph.Op `json:"ops" binding:"required"`
}

// ApplySceneOpsHTTP applies deterministic ops (setText, moveToZone, restyle…)
// to the current scene and stores a new revision.
func ApplySceneOpsHTTP(c *gin.Context) {
	brandID := c.Param("brandId")
	contentID := c.Param("contentId")
	var req applyOpsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	content, err := trendlymodels.GetContent(brandID, contentID)
	if err != nil || content.SceneRef == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no scene to edit"})
		return
	}
	cur, err := trendlymodels.GetSceneRevision(brandID, contentID, content.SceneRef.RevisionID)
	if err != nil || cur == nil || cur.Graph == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "current scene not found"})
		return
	}
	if problems := scenegraph.ValidateOps(cur.Graph, req.Ops); len(problems) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid ops", "problems": problems})
		return
	}
	applied, err := scenegraph.Apply(cur.Graph, req.Ops)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	revID, err := persistScene(brandID, contentID, applied.Document, "edit", cur.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"revisionId": revID})
}

type revertReq struct {
	RevisionID string `json:"revisionId" binding:"required"`
}

// RevertSceneHTTP re-applies a prior revision's graph as a new current revision.
func RevertSceneHTTP(c *gin.Context) {
	brandID := c.Param("brandId")
	contentID := c.Param("contentId")
	var req revertReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	target, err := trendlymodels.GetSceneRevision(brandID, contentID, req.RevisionID)
	if err != nil || target == nil || target.Graph == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "revision not found"})
		return
	}
	revID, err := persistScene(brandID, contentID, target.Graph, "revert", req.RevisionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"revisionId": revID})
}

// persistScene writes an immutable revision, computes the hit-box layout, and
// points the content at it. (Mirrors the AI engine's persistScene; render
// enqueue is handled by the AI path / render worker.)
func persistScene(brandID, contentID string, doc *scenegraph.Document, origin, parentRev string) (string, error) {
	if err := scenegraph.ValidateDocument(doc); err != nil {
		return "", err
	}
	revID, err := trendlymodels.CreateSceneRevision(brandID, contentID, &trendlymodels.ContentSceneRevision{
		Graph:            doc,
		ParentRevisionID: parentRev,
		Origin:           origin,
	})
	if err != nil {
		return "", err
	}
	layout := scenegraph.ComputeLayout(doc)
	if err := trendlymodels.SetContentSceneRef(brandID, contentID, &trendlymodels.ContentSceneRef{
		RevisionID: revID,
		DocType:    string(doc.Type),
		Boxes:      layout.Boxes,
	}); err != nil {
		return "", err
	}
	return revID, nil
}
