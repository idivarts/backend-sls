package main

import (
	apihandler "github.com/TrendsHub/th-backend/pkg/api_handler"
	"github.com/gin-gonic/gin"
)

func main() {
	apihandler.GinEngine.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "Okay so your other function also executed successfully!",
			"gin":     true,
		})
	})

	apihandler.StartLambda()
}
