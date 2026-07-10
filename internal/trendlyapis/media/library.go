package media

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
)

// ListMusicLibrary returns the curated, commercially-licensed music catalog for
// the soundtrack browser. Filterable by ?mood= and free-text ?q=. Browsing +
// selecting a catalog track is free (no wallet metering) — only generation costs.
func ListMusicLibrary(c *gin.Context) {
	tracks, err := trendlymodels.ListMusicLibrary(c.Query("mood"), c.Query("q"), 100)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"tracks": tracks})
}
