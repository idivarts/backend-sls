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
	FirebaseCustomToken string `json:"firebaseCustomToken"`
	IsExistingUser      bool   `json:"isExistingUser"`
}

func InstagramAuth(ctx *gin.Context) {
	var req IInstaAuth
	if err := ctx.ShouldBindJSON(&req); err != nil {
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
