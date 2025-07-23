package matchmaking

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
)

func GetCollaborationIDs(c *gin.Context) {
	// userId, b := middlewares.GetUserId(c)
	// if !b {
	// 	c.JSON(http.StatusBadRequest, gin.H{"error": "User ID not found"})
	// 	return
	// }

	collabs, err := trendlymodels.GetCollabIDs(nil, 100)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Collabs not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": "Collabs not found", "collabs": collabs})
}
