package trendlyapis

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/messenger"
)

func FacebookLogin(c *gin.Context) {
	var person messenger.FacebookLoginRequest

	if err := c.ShouldBindJSON(&person); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userId, _ := middlewares.GetUserId(c)

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
