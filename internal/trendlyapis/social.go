package trendlyapis

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
)

func FetchInsights(c *gin.Context) {
	userId, b := middlewares.GetUserId(c)
	if !b {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid User"})
		return
	}

	user := &trendlymodels.User{}
	err := user.Get(userId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid User", "error": err.Error()})
		return
	}

	pSocial := user.PrimarySocial
	if pSocial == nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Primary Social not found"})
		return
	}

	social := &trendlymodels.SocialsPrivate{}
	social.Get(userId, *pSocial)

	if social.GraphType == 0 {
		// facebook

	} else if social.GraphType == 1 {
		// instagram

	} else {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid Social"})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{"message": "FetchReach"})
}
