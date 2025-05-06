package trendlyCollabs

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
)

func SendInvitation(c *gin.Context) {
	userType := middlewares.GetUserType(c)
	if userType != "manager" {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "Only Managers can call this endpoint"})
	}
	// collabId := c.Param("collabId")
	// userId := c.Param("collabId")

}
