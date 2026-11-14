package design

// The endpoint behind the scene panel and the timeline.

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/mysentry"
)

// EditDesignStructure retimes, reorders, duplicates, adds or deletes a
// scene.
// POST /api/design/brands/:brandId/designs/:designId/structure
//
// Charges nothing: no model is involved. It writes a revision like every
// other edit, so a reorder someone regrets is one click back.
//
// ⚠️ A structural change moves the film's LENGTH, which is what a render
// is priced from. The call we are making: the film is priced AT RENDER
// TIME, from the duration as it then stands — so lengthening a film after
// it was rendered costs the new price on the next render and nothing
// retroactively. No difference to charge, no refund to issue, and the
// number the user is quoted is the number for the film they are about to
// make. The alternative — charging the difference as they drag — would
// bill for a shape they have not decided on yet.
func EditDesignStructure(c *gin.Context) {
	brandID, designID, ok := guard(c, true)
	if !ok {
		return
	}
	var req structureRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()
	d, err := trendlymodels.GetBrandDesign(ctx, brandID, designID)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not read the design"})
		return
	}
	if d == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no such design"})
		return
	}
	// A design nobody has written yet is a fact about the design, not a
	// failure of ours — the render and preview endpoints answer 400 with
	// the same sentence, and a 502 would have the user retrying something
	// that cannot work.
	if d.ProjectPrefix == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "This design has nothing in it yet."})
		return
	}
	source, rerr := readCurrentDocument(ctx, d)
	if rerr != nil {
		mysentry.Capture(rerr, map[string]string{"op": "design.structure.read", "designId": designID})
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not read this film"})
		return
	}

	edited, _, serr := applyStructureEdit(source, req)
	if serr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": serr.Error()})
		return
	}

	out, message, theirs, cerr := commitEditedDocument(ctx, brandID, designID, d, edited,
		trendlymodels.DesignRevisionInspector, structureNote(req))
	if cerr != nil || out == nil {
		status := http.StatusBadGateway
		body := gin.H{"error": message}
		if theirs {
			status = http.StatusBadRequest
			body = gin.H{"error": "that change would break the film", "message": message}
		}
		c.JSON(status, body)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"revision":   out.Revision,
		"previewUrl": out.PreviewURL,
		"durationMs": out.DurationMs,
	})
}

// structureNote is the line the revisions list shows.
func structureNote(req structureRequest) string {
	if isClipOp(req.Op) {
		return clipNote(req)
	}
	switch req.Op {
	case sceneOpRetime:
		return "Retimed " + req.SceneID
	case sceneOpMove:
		return "Moved " + req.SceneID
	case sceneOpDuplicate:
		return "Duplicated " + req.SceneID
	case sceneOpDelete:
		return "Deleted " + req.SceneID
	case sceneOpAdd:
		return "Added a scene"
	}
	return "Changed the scenes"
}
