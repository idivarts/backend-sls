package ccapis

import (
	"net/http"

	"github.com/TrendsHub/th-backend/internal/models"
	"github.com/gin-gonic/gin"
)

type IUser struct {
	Name       string `json:"name"`
	UserName   string `json:"userName"`
	ProfilePic string `json:"profilePic"`
}
type IPage struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	UserName    string `json:"userName"`
	IsInstagram bool   `json:"isInstagram"`
}
type ConversationUnit struct {
	IGSID              string `json:"igsid"`
	User               IUser  `json:"user"`
	LastBotMessageTime int64  `json:"lastBotMessageTime"`
	BotMessageCount    int    `json:"botMessageCount"`
	CurrentPhase       int    `json:"currentPhase"`
	ReminderCount      int    `json:"reminderCount"`
	Status             int    `json:"status"`
	InformationCount   int    `json:"informationCount"`
	Page               IPage  `json:"page"`
}

type IConversationsReq struct {
	PageID *string `form:"pageId,omitempty"`
	Phase  *int    `form:"phase,omitempty"`
}

func GetConversations(c *gin.Context) {
	var req IConversationsReq
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	convs, err := models.GetConversations(req.PageID, req.Phase)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	pConvs := []ConversationUnit{}
	for _, v := range convs {
		fields, err := v.Information.FindEmptyFields()
		infCount := 0
		if err == nil {
			infCount = 12 - len(fields)
		}
		p := ConversationUnit{
			IGSID:              v.IGSID,
			LastBotMessageTime: v.LastBotMessageTime,
			BotMessageCount:    v.BotMessageCount,
			CurrentPhase:       v.CurrentPhase,
			ReminderCount:      v.ReminderCount,
			Status:             v.Status,
			InformationCount:   infCount,
			Page:               IPage{},
		}
		if v.UserProfile != nil {
			p.User = IUser{ProfilePic: v.UserProfile.ProfilePic, Name: v.UserProfile.Name, UserName: v.UserProfile.Username}
		}
		if v.PageID != "" {
			pData := models.Page{}
			err := pData.Get(v.PageID)
			if err == nil {
				p.Page = IPage{
					ID:          pData.PageID,
					Name:        pData.Name,
					UserName:    pData.UserName,
					IsInstagram: pData.IsInstagram,
				}
			}
		}
		pConvs = append(pConvs, p)
	}
	c.JSON(http.StatusOK, gin.H{"message": "List of conversations fetched", "conversations": pConvs})
}
