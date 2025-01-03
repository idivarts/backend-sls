package trendlyapis

import (
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/instagram"
	"github.com/idivarts/backend-sls/pkg/messenger"
)

func FacebookLogin(c *gin.Context) {
	var person messenger.FacebookLoginRequest

	if err := c.ShouldBindJSON(&person); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userId, b := middlewares.GetUserId(c)
	if !b {
		c.JSON(http.StatusBadRequest, gin.H{"error": "User not found"})
		return
	}

	for _, v := range person.Accounts.Data {
		lRes, err := messenger.GetLongLivedAccessToken(v.AccessToken)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		log.Println("Token", v.AccessToken, lRes.AccessToken)

		// var instagram *models.InstagramObject = nil
		if v.InstagramBusinessAccount.ID != "" {
			insta, err := messenger.GetInstagram(v.InstagramBusinessAccount.ID, lRes.AccessToken)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			instaPage := trendlymodels.Socials{
				ID:           insta.ID,
				Name:         insta.Name,
				ConnectedID:  &v.ID,
				UserID:       person.ID,
				OwnerName:    person.Name,
				Image:        insta.ProfilePictureURL,
				IsInstagram:  true,
				InstaProfile: insta,
			}
			_, err = instaPage.Insert(userId)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			instaPPage := trendlymodels.SocialsPrivate{
				AccessToken: &lRes.AccessToken,
			}
			_, err = instaPPage.Set(userId, insta.ID)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			log.Println("Instagram Saved Accesstoken", instaPPage)
		}

		fb, err := messenger.GetFacebook(lRes.AccessToken)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		fbPage := trendlymodels.Socials{
			ID:          v.ID,
			Name:        v.Name,
			UserID:      person.ID,
			OwnerName:   person.Name,
			Image:       fb.Picture.Data.URL,
			ConnectedID: &v.InstagramBusinessAccount.ID,
			IsInstagram: false,
			FBProfile:   fb,
		}
		_, err = fbPage.Insert(userId)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		fbPPage := trendlymodels.SocialsPrivate{
			AccessToken: &lRes.AccessToken,
		}
		_, err = fbPPage.Set(userId, v.ID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		log.Println("FB Saved Accesstoken", fbPPage)
	}

	c.JSON(http.StatusOK, gin.H{"message": "Successfully parsed JSON", "user": person})
}

func ConnectInstagram(ctx *gin.Context) {
	var req IInstaAuth
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(400, gin.H{"error": err.Error()})
		return
	}

	userId, b := middlewares.GetUserId(ctx)
	if !b {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "User not found"})
		return
	}

	redirect_uri := fmt.Sprintf("%s/%s", INSTAGRAM_REDIRECT, req.RedirectType)
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

	socialId := strconv.FormatInt(accessToken.UserID, 10)

	insta, err := instagram.GetInstagram("me", llToken.AccessToken)
	if err != nil {
		ctx.JSON(400, gin.H{"error": err.Error()})
		return
	}

	user := trendlymodels.User{}
	err = user.Get(userId)
	if err != nil {
		ctx.JSON(400, gin.H{"error": err.Error()})
		return
	}

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

	ctx.JSON(http.StatusOK, gin.H{"message": "Successfully social added", "social": social})

}
