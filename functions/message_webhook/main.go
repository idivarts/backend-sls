package main

import (
	apihandler "github.com/TrendsHub/th-backend/pkg/api_handler"
	"github.com/gin-gonic/gin"
)

func main() {
	apihandler.GinEngine.POST("/instagram/webhook", func(ctx *gin.Context) {
		ctx.JSON(200, gin.H{
			"message": "Success",
		})
	})

	apihandler.StartLambda()
}
