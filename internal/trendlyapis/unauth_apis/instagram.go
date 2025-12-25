package trendlyunauth

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/constants"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/instagram"
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

	clientId := os.Getenv("INSTA_CLIENT_ID")
	if clientId == "" {
		ctx.JSON(400, gin.H{"error": "Instagram client id not found"})
		return
	}
	redirect_uri := fmt.Sprintf("%s/%s", constants.INSTAGRAM_REDIRECT, redirect_type)
	ctx.Redirect(302, fmt.Sprintf("https://www.instagram.com/oauth/authorize?enable_fb_login=1&force_authentication=0&client_id=%s&redirect_uri=%s&response_type=code&scope=instagram_business_basic,instagram_business_manage_insights", clientId, url.QueryEscape(redirect_uri)))
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

type ITokenResponse struct {
}

func InstagramAuth(ctx *gin.Context) {
	var req constants.IInstaAuth
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(400, gin.H{"error": err.Error()})
		return
	}

	userId, b := middlewares.GetUserId(ctx)
	if !b {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "User not authenticated"})
		return
	}

	user := trendlymodels.User{}
	err := user.Get(userId)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error getting user"})
		return
	}

	redirect_uri := fmt.Sprintf("%s/%s", constants.INSTAGRAM_REDIRECT, req.RedirectType)
	accessToken, err := instagram.GetAccessTokenFromCode(req.Code, redirect_uri)
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

	insta, err := instagram.GetInstagram("me", llToken.AccessToken)
	if err != nil {
		ctx.JSON(400, gin.H{"error": err.Error()})
		return
	}

	socialId := strconv.FormatInt(accessToken.UserID, 10)

	// Add the socials for that user
	social := trendlymodels.Socials{
		ID:           socialId,
		Name:         insta.Name,
		Image:        insta.ProfilePictureURL,
		IsInstagram:  true,
		ConnectedID:  nil,
		UserID:       userId,
		OwnerName:    insta.Name,
		InstaProfile: insta,
		FBProfile:    nil,
	}
	_, err = social.Insert(userId)
	if err != nil {
		ctx.JSON(400, gin.H{"error": err.Error()})
		return
	}

	user.PrimarySocial = &socialId
	_, err = user.Insert(userId)
	if err != nil {
		ctx.JSON(400, gin.H{"error": err.Error()})
		return
	}

	// Save the access token in the firestore database
	socialPrivate := trendlymodels.SocialsPrivate{
		AccessToken: &llToken.AccessToken,
		GraphType:   trendlymodels.InstagramGraphType,
	}
	_, err = socialPrivate.Set(userId, socialId)
	if err != nil {
		ctx.JSON(400, gin.H{"error": err.Error()})
		return
	}

	res := ITokenResponse{}
	ctx.JSON(http.StatusOK, gin.H{"message": "Successfully Logged in", "data": res})
}

func InstagramDeAuth(ctx *gin.Context) {

}

func InstagramDelete(ctx *gin.Context) {

}
