package trendlyunauth

import (
	"fmt"
	"log"
	"net/url"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/constants"
)

func InstagramRedirect(ctx *gin.Context) {
	redirect_type := ctx.Query("redirect_type")
	if redirect_type == "" {
		ctx.JSON(400, gin.H{"error": "Redirect Type is not found"})
		return
	}
	if redirect_type != "1" && redirect_type != "2" && redirect_type != "3" && redirect_type != "4" {
		ctx.JSON(400, gin.H{"error": "Invalid redirect type. Supported values are 1, 2, 3, 4"})
		return
	}

	extraScope := ""
	insights := ctx.Query("insights")
	if insights != "" {
		extraScope = ",instagram_business_manage_insights"
	}

	clientId := os.Getenv("INSTA_CLIENT_ID")
	if clientId == "" {
		ctx.JSON(400, gin.H{"error": "Instagram client id not found"})
		return
	}
	redirect_uri := fmt.Sprintf("%s/%s", constants.INSTAGRAM_REDIRECT, redirect_type)
	log.Println("Redirect URI:", redirect_uri)

	ctx.Redirect(302, fmt.Sprintf("https://www.instagram.com/oauth/authorize?enable_fb_login=1&force_authentication=0&client_id=%s&redirect_uri=%s&response_type=code&scope=instagram_business_basic%s", clientId, url.QueryEscape(redirect_uri), extraScope))
}

func InstagramAuthRedirect(ctx *gin.Context) {
	code := ctx.Query("code")
	if code == "" {
		ctx.JSON(400, gin.H{"error": "Code not found"})
		return
	}
	redirect_type := ctx.Param("redirect_type")
	if redirect_type == "" {
		ctx.JSON(400, gin.H{"error": "Redirect Type not found"})
		return
	}

	redirectUri := ""
	if redirect_type == "1" {
		redirectUri = "http://localhost:8081/insta-redirect"
	} else if redirect_type == "2" {
		redirectUri = fmt.Sprintf("%s%s", constants.GetCreatorsFronted(), "/insta-redirect")
	} else if redirect_type == "3" || redirect_type == "4" {
		redirectUri = "fb567254166026958://authorize"
	} else {
		ctx.JSON(400, gin.H{"error": "Invalid Redirect Type"})
		return
	}
	ctx.Redirect(302, fmt.Sprintf("%s?code=%s", redirectUri, code))
}

func InstagramDeAuth(ctx *gin.Context) {

}

func InstagramDelete(ctx *gin.Context) {

}
