package businessapis

import (
	"errors"
	"net/http"

	eventhandling "github.com/TrendsHub/th-backend/internal/message_sqs/event_handling"
	sqsevents "github.com/TrendsHub/th-backend/internal/message_sqs/events"
	"github.com/TrendsHub/th-backend/internal/models"
	"github.com/TrendsHub/th-backend/pkg/openai"
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

func GetPausedConversations(c *gin.Context) {
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

type IStartConversationRequest struct {
	IGSID                 string `json:"igsid" binding:"required"`
	Message               string `json:"Message" binding:"required"`
	AdditionalInstruction string `json:"AdditionalInstruction"`
}

func StartPausedConversation(c *gin.Context) {
	var req IStartConversationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	cData := &models.Conversation{}
	err := cData.Get(req.IGSID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if cData.IGSID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": errors.New("Can't find this entry")})
		return
	}
	_, err = openai.SendMessage(cData.ThreadID, req.Message, nil, false)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	cData.IsConversationPaused = 0
	_, err = cData.Insert()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// openai.SendMessage()
	conv := &sqsevents.ConversationEvent{
		Action:   sqsevents.RUN_OPENAI,
		IGSID:    req.IGSID,
		ThreadID: cData.ThreadID,
		MID:      cData.LastMID,
	}
	err = eventhandling.RunOpenAI(conv, req.AdditionalInstruction)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Conversation started successfully"})
}
