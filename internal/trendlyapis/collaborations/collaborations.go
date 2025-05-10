package trendlyCollabs

import (
	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
)

// Starting a collab | Request to start
func StartContract(c *gin.Context) {
	userType := middlewares.GetUserType(c)
	if userType == "user" {
		requestToStart(c)
		return
	}

}

func requestToStart(c *gin.Context) {

}
