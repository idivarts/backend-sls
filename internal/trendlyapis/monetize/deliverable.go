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

func RequestDeliverable(c *gin.Context) {
	data, err := initializeData(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Failed to retrieve initialization data"})
		return
	}

	// 1. Fetch Influencer for notification
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

	// 2. Send Push Notification to Influencer
	notif := &trendlymodels.Notification{
		Title:       "Deliverable Requested! 📽️",
		Description: fmt.Sprintf("%s has requested the video for %s. Please share your timeline!", data.Brand.Name, collabName),
		TimeStamp:   time.Now().UnixMilli(),
		IsRead:      false,
		Type:        "deliverable-requested",
		Data: &trendlymodels.NotificationData{
			CollaborationID: &data.Contract.CollaborationID,
			GroupID:         &data.ContractID,
		},
	}
	_, _, err = notif.Insert(trendlymodels.USER_COLLECTION, data.Contract.UserID)
	if err != nil {
		log.Printf("Failed to send push notification: %v", err)
	}

	// 3. Send Email to Influencer
	if influencer.Email != nil {
		emailData := map[string]interface{}{
			"InfluencerName":  influencer.Name,
			"BrandName":       data.Brand.Name,
			"CollabTitle":     collabName,
			"DeliverableLink": fmt.Sprintf("%s/contracts/%s", constants.TRENDLY_CREATORS_FE, data.ContractID),
		}
		err = myemail.SendCustomHTMLEmail(*influencer.Email, templates.DeliverableRequested, templates.SubjectDeliverableRequested, emailData)
		if err != nil {
			log.Printf("Failed to send deliverable requested email: %v", err)
		}
	}

	// 4. Send Stream System Message
	streamMessage := fmt.Sprintf("📽️ **Deliverable Requested!**\n\n%s is excited to see the content! Could you please share when you plan to submit the video and your expected timeline? ✨", data.Brand.Name)
	err = streamchat.SendSystemMessage(data.Contract.StreamChannelID, streamMessage)
	if err != nil {
		log.Printf("Failed to send stream message: %v", err)
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Deliverable requested successfully",
	})
}

func ApproveDeliverable(c *gin.Context) {
	var req struct{}
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Invalid request payload"})
		return
	}

	// The real implementation will go here in the future

	c.JSON(http.StatusOK, gin.H{"message": "This is a placeholder endpoint for Trendly Monetize APIs."})
}

func SendDeliverable(c *gin.Context) {
	var req struct{}
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Invalid request payload"})
		return
	}

	// The real implementation will go here in the future

	c.JSON(http.StatusOK, gin.H{"message": "This is a placeholder endpoint for Trendly Monetize APIs."})
}

func RequestDeliverableApproval(c *gin.Context) {
	var req struct{}
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Invalid request payload"})
		return
	}

	// The real implementation will go here in the future

	c.JSON(http.StatusOK, gin.H{"message": "This is a placeholder endpoint for Trendly Monetize APIs."})
}
