package businessapis

import (
	"net/http"

	"github.com/TrendsHub/th-backend/internal/models"
	"github.com/TrendsHub/th-backend/pkg/messenger"
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
		page := models.Page{
			PageID:      v.ID,
			UserID:      person.ID,
			AccessToken: lRes.AccessToken,
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
