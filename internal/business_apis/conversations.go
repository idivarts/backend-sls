package businessapis

import (
	"net/http"

	"github.com/TrendsHub/th-backend/internal/models"
	"github.com/gin-gonic/gin"
)

type PauseConversationUnit struct {
	IGSID      string `json:"igsid"`
	Name       string `json:"name"`
	UserName   string `json:"userName"`
	ProfilePic string `json:"profilePic"`
	// PageName    string `json:"pageName"`
	// IsInstagram bool   `json:"isInstagram"`
}

func GetConversations(c *gin.Context) {
	convs, err := models.GetPausedConversations()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	pConvs := []PauseConversationUnit{}
	for _, v := range convs {
		p := PauseConversationUnit{
			IGSID: v.IGSID,
		}
		if v.UserProfile != nil {
			p = PauseConversationUnit{
				IGSID:      v.IGSID,
				ProfilePic: v.UserProfile.ProfilePic,
				Name:       v.UserProfile.Name,
				UserName:   v.UserProfile.Username,
			}
		}
		pConvs = append(pConvs, p)
	}
	c.JSON(http.StatusOK, gin.H{"message": "List of conversations fetched", "conversations": pConvs})
}
