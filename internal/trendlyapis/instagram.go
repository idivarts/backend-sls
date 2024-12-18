package trendlyapis

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/firebase/fauth"
	"github.com/idivarts/backend-sls/pkg/instagram"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const INSTAGRAM_REDIRECT = "https://be.trendly.pro/instagram/auth"

func InstagramRedirect(ctx *gin.Context) {
	redirect_type := ctx.Query("redirect_type")
	if redirect_type == "" {
		ctx.JSON(400, gin.H{"error": "Redirect Type is not found"})
		return
	}
	if redirect_type != "1" || redirect_type != "2" || redirect_type != "3" || redirect_type != "4" {
		ctx.JSON(400, gin.H{"error": "Invalid redirect type. Supported values are 1, 2, 3, 4"})
		return
	}

	clientId := os.Getenv("INSTA_CLIENT_ID")
	if clientId == "" {
		ctx.JSON(400, gin.H{"error": "Instagram client id not found"})
		return
	}
	redirect_uri := fmt.Sprintf("%s?redirect_type=%s", INSTAGRAM_REDIRECT, redirect_type)
	ctx.Redirect(302, fmt.Sprintf("https://www.instagram.com/oauth/authorize?enable_fb_login=1&force_authentication=0&client_id=%s&redirect_uri=%s&response_type=code&scope=instagram_business_basic", clientId, url.QueryEscape(redirect_uri)))
}

func InstagramAuthRedirect(ctx *gin.Context) {
	code := ctx.Query("code")
	if code == "" {
		ctx.JSON(400, gin.H{"error": "Code not found"})
		return
	}
	redirect_type := ctx.Query("redirect_type")
	if redirect_type == "" {
		ctx.JSON(400, gin.H{"error": "Redirect Type not found"})
		return
	}

	redirectUri := ""
	if redirect_type == "1" {
		redirectUri = "http://localhost:8081"
	} else if redirect_type == "2" {
		redirectUri = "https://creators.trendly.pro/"
	} else if redirect_type == "3" || redirect_type == "4" {
		redirectUri = "fb567254166026958://authorize"
	} else {
		ctx.JSON(400, gin.H{"error": "Invalid Redirect Type"})
		return
	}
	ctx.Redirect(302, fmt.Sprintf("%s?code=%s", redirectUri, code))
}

type IInstaAuth struct {
	Code         string `json:"code"`
	RedirectType string `json:"redirect_type"`
}
type ITokenResponse struct {
	FirebaseCustomToken string `json:"firebaseCustomToken"`
	IsExistingUser      bool   `json:"isExistingUser"`
}

func InstagramAuth(ctx *gin.Context) {
	var req IInstaAuth
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(400, gin.H{"error": err.Error()})
		return
	}

	redirect_uri := fmt.Sprintf("%s?redirect_type=%s", INSTAGRAM_REDIRECT, req.RedirectType)
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

	userId := strconv.FormatInt(accessToken.UserID, 10)

	insta, err := instagram.GetInstagram("me", llToken.AccessToken)
	if err != nil {
		ctx.JSON(400, gin.H{"error": err.Error()})
		return
	}

	user := trendlymodels.User{}
	err = user.Get(userId)
	existingUser := true
	if err != nil {
		if status.Code(err) == codes.NotFound {
			// Create User Model if new user
			user = trendlymodels.User{
				Name:          insta.Name,
				ProfileImage:  &insta.ProfilePictureURL,
				PrimarySocial: &userId,
				Email:         nil,
				PhoneNumber:   nil,
				Location:      nil,
				EmailVerified: nil,
				PhoneVerified: nil,
				Profile: &trendlymodels.UserProfile{
					CompletionPercentage: aws.Int(10),
					Content:              &trendlymodels.UserProfileContent{},
					IntroVideo:           nil,
					Category:             []string{},
					Attachments:          []trendlymodels.UserAttachment{},
					TimeCommitment:       nil,
				},
				Preferences: &trendlymodels.UserPreferences{},
				Settings:    &trendlymodels.UserSettings{},
				Backend: &trendlymodels.BackendData{
					Followers: &insta.FollowersCount,
				},
				PushNotificationToken: &trendlymodels.PushNotificationToken{},
			}
			_, err = user.Insert(userId)
			if err != nil {
				ctx.JSON(400, gin.H{"error": err.Error()})
				return
			}
			existingUser = false
		} else {
			ctx.JSON(400, gin.H{"error": err.Error()})
			return
		}
	} else {
		user.PrimarySocial = &userId
		_, err = user.Insert(userId)
		if err != nil {
			ctx.JSON(400, gin.H{"error": err.Error()})
			return
		}
	}

	// Add the socials for that user
	social := trendlymodels.Socials{
		ID:           userId,
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

	// Save the access token in the firestore database
	socialPrivate := trendlymodels.SocialsPrivate{
		AccessToken: &llToken.AccessToken,
		GraphType:   trendlymodels.InstagramGraphType,
	}
	_, err = socialPrivate.Set(userId, userId)
	if err != nil {
		ctx.JSON(400, gin.H{"error": err.Error()})
		return
	}

	// Create custom firebase token and send it back to the client
	token, err := fauth.Client.CustomToken(context.Background(), userId)
	if err != nil {
		ctx.JSON(400, gin.H{"error": err.Error()})
		return
	}

	res := ITokenResponse{
		FirebaseCustomToken: token,
		IsExistingUser:      existingUser,
	}
	ctx.JSON(http.StatusOK, gin.H{"message": "Successfully Logged in", "data": res})

}

func InstagramDeAuth(ctx *gin.Context) {

}

func InstagramDelete(ctx *gin.Context) {

}
