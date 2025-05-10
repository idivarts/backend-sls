package trendlyCollabs

import (
	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
)

// End | request to end Contract
func EndContract(c *gin.Context) {
	userType := middlewares.GetUserType(c)
	if userType == "user" {
		requestToEndContract(c)
		return
	}
}
func requestToEndContract(c *gin.Context) {

}

func GiveContractFeedback(c *gin.Context) {

}
