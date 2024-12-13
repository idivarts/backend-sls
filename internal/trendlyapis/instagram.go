package trendlyapis

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/pkg/firebase/fauth"
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

type IInstaAuth struct {
	Code        string `json:"code"`
	RedirectUri string `json:"redirect_uri"`
}
type ITokenResponse struct {
	Token string `json:"token"`
}

func InstagramAuth(ctx *gin.Context) {
	var req IInstaAuth
	if err := ctx.BindJSON(&req); err != nil {
		ctx.JSON(400, gin.H{"error": err.Error()})
		return
	}

	accessToken, err := instagram.GetAccessTokenFromCode(req.Code, req.RedirectUri)
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

	// Save the access token in the firestore database

	// Add the socials for that user

	// Create custom firebase token and send it back to the client
	token, err := fauth.Client.CustomToken(context.Background(), strconv.FormatInt(accessToken.UserID, 10))
	if err != nil {
		ctx.JSON(400, gin.H{"error": err.Error()})
		return
	}

	res := ITokenResponse{
		Token: token,
	}
	ctx.JSON(http.StatusOK, gin.H{"message": "Successfully parsed JSON", "data": res})

}
