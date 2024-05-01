package messagewebhook

import "github.com/gin-gonic/gin"

func Receive(ctx *gin.Context) {
	ctx.JSON(200, gin.H{
		"message": "Success",
	})
}
