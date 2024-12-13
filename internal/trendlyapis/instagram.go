package trendlyapis

import (
	"fmt"
	"log"
	"net/url"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/pkg/instagram"
)

func InstagramRedirect(ctx *gin.Context) {
	redirect_uri := ctx.Query("redirect_uri")
	if redirect_uri == "" {
		ctx.JSON(400, gin.H{"error": "Redirect URI not found"})
		return
	}

	clientId := os.Getenv("INSTA_CLIENT_ID")
	if clientId == "" {
		ctx.JSON(400, gin.H{"error": "Instagram client id not found"})
		return
	}
	ctx.Redirect(302, fmt.Sprintf("https://www.instagram.com/oauth/authorize?enable_fb_login=1&force_authentication=0&client_id=%s&redirect_uri=%s&response_type=code&scope=instagram_business_basic", clientId, url.QueryEscape(redirect_uri)))
}

func InstagramAuth(ctx *gin.Context) {
	code := ctx.Query("code")
	if code == "" {
		ctx.JSON(400, gin.H{"error": "Code not found"})
		return
	}
	redirect_uri := ctx.Query("redirect_uri")
	if redirect_uri == "" {
		ctx.JSON(400, gin.H{"error": "Redirect URI not found"})
		return
	}

	accessToken, err := instagram.GetAccessTokenFromCode(code, redirect_uri)
	if err != nil {
		ctx.JSON(400, gin.H{"error": err.Error()})
		return
	}
	log.Println("Access Token:", accessToken.AccessToken)

	llToken, err := instagram.GetLongLivedAccessToken(accessToken.AccessToken)
	if err != nil {
		ctx.JSON(400, gin.H{"error": err.Error()})
		return
	}
	log.Println("Long Lived Access Token:", llToken.AccessToken)

	// Save the access toke in the firestore database

	// Create custom firebase token and send it back to the client

}
