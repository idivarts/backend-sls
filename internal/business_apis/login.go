package businessapis

import (
	"log"
	"net/http"

	"github.com/TrendsHub/th-backend/internal/models"
	"github.com/TrendsHub/th-backend/pkg/messenger"
	"github.com/TrendsHub/th-backend/pkg/openai"
	"github.com/gin-gonic/gin"
)

func Login(c *gin.Context) {
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

		var instagram *models.InstagramObject = nil
		if v.InstagramBusinessAccount.ID != "" {
			inst, err := messenger.GetInstagram(v.InstagramBusinessAccount.ID, lRes.AccessToken)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			instagram = &models.InstagramObject{
				ID:       inst.ID,
				Name:     inst.Name,
				UserName: inst.Username,
				Bio:      inst.Biography,
			}
		}

		page := models.Page{
			PageID:      v.ID,
			UserID:      person.ID,
			Name:        v.Name,
			Instagram:   instagram,
			AccessToken: lRes.AccessToken,
			AssistantID: string(openai.ArjunAssistant),
			Status:      1,
		}
		_, err = page.Insert()
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "Successfully parsed JSON", "user": person})
}
