package trendlyapis

import (
	"fmt"
	"os"

	"github.com/gin-gonic/gin"
)

func InstagramRedirect(ctx *gin.Context) {
	clientId := os.Getenv("INSTAGRAM_CLIENT_ID")
	if clientId == "" {
		ctx.JSON(400, gin.H{"error": "Instagram client id not found"})
	}
	ctx.Redirect(302, fmt.Sprintf("https://www.instagram.com/oauth/authorize?enable_fb_login=1&force_authentication=0&client_id=%s&redirect_uri=https://be.trendly.pro/instagram/auth&response_type=code&scope=instagram_business_basic", clientId))
}

func InstagramAuth(ctx *gin.Context) {

}
