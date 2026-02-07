package monetize

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/constants"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/myemail"
	"github.com/idivarts/backend-sls/pkg/streamchat"
	"github.com/idivarts/backend-sls/templates"
)

func ReSchedulePosting(c *gin.Context) {
	var req struct{}
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Invalid request payload"})
		return
	}

	// The real implementation will go here in the future

	c.JSON(http.StatusOK, gin.H{"message": "This is a placeholder endpoint for Trendly Monetize APIs."})
}

func MarkPosted(c *gin.Context) {
	var req struct{}
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Invalid request payload"})
		return
	}

	// The real implementation will go here in the future

	c.JSON(http.StatusOK, gin.H{"message": "This is a placeholder endpoint for Trendly Monetize APIs."})
}

type PostRescheduleReq struct {
	Note string `json:"note" binding:"required"`
}

func RequestPostReSchedule(c *gin.Context) {
	var req PostRescheduleReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Invalid request payload"})
		return
	}

	data, err := initializeData(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Failed to retrieve initialization data"})
		return
	}

	// 1. Fetch Influencer for details
	influencer := &trendlymodels.User{}
	err = influencer.Get(data.Contract.UserID)
	if err != nil {
		log.Printf("Failed to get influencer: %v", err)
	}

	collab := &trendlymodels.Collaboration{}
	err = collab.Get(data.Contract.CollaborationID)
	collabName := "Your Collaboration"
	if err == nil {
		collabName = collab.Name
	}

	// 2. Notify Brand (Push & Email)
	notif := &trendlymodels.Notification{
		Title:       "Reschedule Requested! 🗓️",
		Description: fmt.Sprintf("%s has requested a change in the posting date for %s.", influencer.Name, collabName),
		TimeStamp:   time.Now().UnixMilli(),
		IsRead:      false,
		Type:        "post-reschedule-requested",
		Data: &trendlymodels.NotificationData{
			CollaborationID: &data.Contract.CollaborationID,
			GroupID:         &data.ContractID,
		},
	}
	_, brandEmails, err := notif.Insert(trendlymodels.BRAND_COLLECTION, data.Contract.BrandID)
	if err == nil && len(brandEmails) > 0 {
		emailData := map[string]interface{}{
			"BrandMemberName": data.Brand.Name,
			"InfluencerName":  influencer.Name,
			"CollabTitle":     collabName,
			"Note":            req.Note,
			"ShipmentLink":    fmt.Sprintf("%s/contracts/%s", constants.TRENDLY_BRANDS_FE, data.ContractID),
		}
		err = myemail.SendCustomHTMLEmailToMultipleRecipients(brandEmails, templates.PostRescheduleRequest, templates.SubjectPostRescheduleRequested, emailData)
		if err != nil {
			log.Printf("Failed to send reschedule request email to brand: %v", err)
		}
	}

	// 3. Send Stream System Message
	streamMessage := fmt.Sprintf("🗓️ **Reschedule Requested!**\n\n%s has requested to change the posting date.\n\n**Note from Creator:** %s", influencer.Name, req.Note)
	err = streamchat.SendSystemMessage(data.Contract.StreamChannelID, streamMessage)
	if err != nil {
		log.Printf("Failed to send stream message: %v", err)
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Reschedule request sent to brand successfully",
	})
}
