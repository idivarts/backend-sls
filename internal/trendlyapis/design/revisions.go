package design

// Going back.
//
// The list itself needs no endpoint: revisions live on the design document,
// which the app already subscribes to, so they arrive with everything else.
// What needs an endpoint is the one thing a client must not do for itself —
// moving the pointer. A client able to write projectPrefix could point a
// design at another brand's files, which is why the document is
// backend-written throughout (see init.go).

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/mediastore"
	"github.com/idivarts/backend-sls/pkg/mysentry"
)

// RestoreDesignRevision points a design back at one of its own revisions.
// POST /api/design/brands/:brandId/designs/:designId/revisions/:n/restore
//
// Writes no files and charges nothing: the revision's bytes are still where
// they were, and that is the entire point of having kept them. What moves is
// the pointer, the player page and the timeline — together, in one
// transaction, because a design playing one revision while the renderer
// renders another is the failure this is meant to prevent.
//
// The restore is itself recorded as a new revision, so going back is
// undoable and the list reads as a history rather than quietly rewriting
// itself.
func RestoreDesignRevision(c *gin.Context) {
	brandID, designID, ok := guard(c, true)
	if !ok {
		return
	}
	n, err := strconv.Atoi(c.Param("n"))
	if err != nil || n <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "that is not a revision number"})
		return
	}

	ctx := c.Request.Context()
	restored, pruned, rerr := trendlymodels.RestoreDesignRevision(ctx, brandID, designID, n)
	if rerr != nil {
		// A revision that is not there is the user's mistake, not ours:
		// they may be acting on a list that has since been pruned.
		c.JSON(http.StatusNotFound, gin.H{"error": rerr.Error()})
		return
	}

	// Cleanup is best-effort and after the fact, for the same reason it is
	// everywhere else: bytes left behind cost a few hundred kilobytes, and
	// a failed delete must never undo a restore that has already happened.
	for _, p := range pruned {
		if _, derr := mediastore.DeletePrefix(ctx, mediastore.TargetAttachment, p); derr != nil {
			mysentry.Capture(derr, map[string]string{"op": "design.revision.cleanup", "prefix": p})
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"restored":   restored.N,
		"prefix":     restored.Prefix,
		"previewUrl": restored.PreviewURL,
	})
}
