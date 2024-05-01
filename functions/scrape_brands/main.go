package main

import (
	apihandler "github.com/TrendsHub/th-backend/pkg/api_handler"
	"github.com/gin-gonic/gin"
)

func main() {
	apihandler.GinEngine.POST("/scrape/brands", func(ctx *gin.Context) {
		ctx.JSON(200, gin.H{
			"message": "Success",
		})
	})
	apihandler.GinEngine.GET("/scrape/brands", func(ctx *gin.Context) {
		ctx.JSON(200, gin.H{
			"message": "Success",
		})
	})

	apihandler.StartLambda()

}
