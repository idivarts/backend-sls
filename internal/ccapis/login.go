package ccapis

import (
	"log"
	"net/http"

	"github.com/TrendsHub/th-backend/internal/models"
	"github.com/TrendsHub/th-backend/pkg/messenger"
	"github.com/gin-gonic/gin"
)

func FacebookLogin(c *gin.Context) {
	var person messenger.FacebookLoginRequest

	if err := c.ShouldBindJSON(&person); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	pages, err := models.GetPagesByUserId(person.ID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	for i := 0; i < len(pages); i++ {
		pages[i].Status = 0
		_, err = pages[i].Insert()
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
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
			inst, err := messenger.GetInstagram(v.InstagramBusinessAccount.ID, lRes.AccessToken)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			instaPage := models.Source{
				OrganizationID:     person.OrganizationID,
				PageID:             inst.ID,
				Name:               inst.Name,
				UserID:             person.ID,
				OwnerName:          person.Name,
				IsWebhookConnected: false,
				Status:             1,
				UserName:           &inst.Username,
				Bio:                &inst.Biography,
				SourceType:         models.Instagram,
				ConnectedID:        &v.ID,
				AccessToken:        &lRes.AccessToken,
				// IsInstagram:            true,
				// AssistantID:            string(openai.ArjunAssistant),
				// ReminderTimeMultiplier: 60 * 60 * 6,
				// ReplyTimeMin:           15,
				// ReplyTimeMax:           120,
			}
			_, err = instaPage.Insert()
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
		}

		fbPage := models.Source{
			OrganizationID:     person.OrganizationID,
			PageID:             v.ID,
			Name:               v.Name,
			UserID:             person.ID,
			OwnerName:          person.Name,
			IsWebhookConnected: false,
			Status:             1,
			UserName:           nil,
			Bio:                nil,
			SourceType:         models.Facebook,
			ConnectedID:        &v.InstagramBusinessAccount.ID,
			AccessToken:        &lRes.AccessToken,
		}
		_, err = fbPage.Insert()
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "Successfully parsed JSON", "user": person})
}
