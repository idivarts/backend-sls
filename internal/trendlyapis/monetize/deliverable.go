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

type DeliverableChangeReq struct {
	Notes string `json:"notes" binding:"required"`
}

func RequestDeliverableChange(c *gin.Context) {
	var req DeliverableChangeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Invalid request payload"})
		return
	}

	data, err := initializeData(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Failed to retrieve initialization data"})
		return
	}

	// 1. Update Contract Deliverable Revisions and Status
	if data.Contract.Deliverable == nil {
		data.Contract.Deliverable = &trendlymodels.Deliverable{}
	}
	data.Contract.Deliverable.RevisionCount++
	data.Contract.Deliverable.RevisionNotes = append(data.Contract.Deliverable.RevisionNotes, req.Notes)
	data.Contract.Deliverable.Status = "revision-requested"
	data.Contract.Status = 6 // Moving back to "Received" (Influencer has the product and needs to rework)

	err = data.Contract.Update(data.ContractID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Failed to update contract"})
		return
	}

	// 2. Fetch Influencer for notification
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

	// 3. Send Push Notification to Influencer
	notif := &trendlymodels.Notification{
		Title:       "Revision Requested! 📽️",
		Description: fmt.Sprintf("%s has requested some changes for %s. Review the feedback now!", data.Brand.Name, collabName),
		TimeStamp:   time.Now().UnixMilli(),
		IsRead:      false,
		Type:        "deliverable-revision",
		Data: &trendlymodels.NotificationData{
			CollaborationID: &data.Contract.CollaborationID,
			GroupID:         &data.ContractID,
		},
	}
	_, _, err = notif.Insert(trendlymodels.USER_COLLECTION, data.Contract.UserID)
	if err != nil {
		log.Printf("Failed to send push notification: %v", err)
	}

	// 4. Send Email to Influencer
	if influencer.Email != nil {
		emailData := map[string]interface{}{
			"InfluencerName":  influencer.Name,
			"BrandName":       data.Brand.Name,
			"CollabTitle":     collabName,
			"Feedback":        req.Notes,
			"DeliverableLink": fmt.Sprintf("%s/contracts/%s", constants.TRENDLY_CREATORS_FE, data.ContractID),
		}
		err = myemail.SendCustomHTMLEmail(*influencer.Email, templates.DeliverableRevisionRequested, templates.SubjectDeliverableRevisionRequested, emailData)
		if err != nil {
			log.Printf("Failed to send deliverable revision email: %v", err)
		}
	}

	// 5. Send Stream System Message
	streamMessage := fmt.Sprintf("📽️ **Revision Requested!**\n\n%s has reviewed the content and requested some changes.\n\n**Feedback:** %s", data.Brand.Name, req.Notes)
	err = streamchat.SendSystemMessage(data.Contract.StreamChannelID, streamMessage)
	if err != nil {
		log.Printf("Failed to send stream message: %v", err)
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Revision requested successfully",
	})
}

type DeliverableReq struct {
	VideoURL string `json:"videoUrl" binding:"required"`
	Note     string `json:"note"`
}

func SendDeliverable(c *gin.Context) {
	var req DeliverableReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Invalid request payload"})
		return
	}

	data, err := initializeData(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Failed to retrieve initialization data"})
		return
	}

	// 1. Update Contract Deliverable and Status
	if data.Contract.Deliverable == nil {
		data.Contract.Deliverable = &trendlymodels.Deliverable{}
	}
	data.Contract.Deliverable.DeliverableLinks = append(data.Contract.Deliverable.DeliverableLinks, req.VideoURL)
	data.Contract.Deliverable.Notes = req.Note
	data.Contract.Deliverable.Status = "submitted"
	data.Contract.Status = 7 // Marking as Deliverable Sent

	err = data.Contract.Update(data.ContractID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Failed to update contract"})
		return
	}

	// 2. Fetch Influencer for details
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

	// 3. Notify Brand (Push & Email)
	notif := &trendlymodels.Notification{
		Title:       "Deliverable Received! 📽️",
		Description: fmt.Sprintf("%s has submitted the video for %s. Review it now!", influencer.Name, collabName),
		TimeStamp:   time.Now().UnixMilli(),
		IsRead:      false,
		Type:        "deliverable-sent",
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
			"Notes":           req.Note,
			"ReviewLink":      fmt.Sprintf("%s/contracts/%s", constants.TRENDLY_BRANDS_FE, data.ContractID),
		}
		err = myemail.SendCustomHTMLEmailToMultipleRecipients(brandEmails, templates.DeliverableSent, templates.SubjectDeliverableSent, emailData)
		if err != nil {
			log.Printf("Failed to send deliverable sent email: %v", err)
		}
	}

	// 4. Send Stream System Message
	streamMessage := fmt.Sprintf("📽️ **Deliverable Submitted!**\n\n%s has shared the content for review. Ready for feedback! ✨", influencer.Name)
	if req.Note != "" {
		streamMessage += fmt.Sprintf("\n\n**Note from Creator:** %s", req.Note)
	}
	err = streamchat.SendSystemMessage(data.Contract.StreamChannelID, streamMessage)
	if err != nil {
		log.Printf("Failed to send stream message: %v", err)
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Deliverable submitted successfully",
	})
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
