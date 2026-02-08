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

type ReScheduleReq struct {
	NewScheduledDate int64 `json:"newScheduledDate" binding:"required"`
}

func ReSchedulePosting(c *gin.Context) {
	var req ReScheduleReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Invalid request payload"})
		return
	}

	data, err := initializeData(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Failed to retrieve initialization data"})
		return
	}

	// 1. Update Contract Posting Schedule
	if data.Contract.Posting == nil {
		data.Contract.Posting = &trendlymodels.Posting{}
	}
	data.Contract.Posting.ScheduledDate = req.NewScheduledDate
	data.Contract.Posting.Status = "rescheduled"

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

	// 3. Notify Influencer (Push & Email)
	newDateStr := time.UnixMilli(req.NewScheduledDate).Format("Jan 02, 2006")

	notif := &trendlymodels.Notification{
		Title:       "Posting Rescheduled! 🗓️",
		Description: fmt.Sprintf("%s has rescheduled the posting date for %s to %s.", data.Brand.Name, collabName, newDateStr),
		TimeStamp:   time.Now().UnixMilli(),
		IsRead:      false,
		Type:        "post-rescheduled",
		Data: &trendlymodels.NotificationData{
			CollaborationID: &data.Contract.CollaborationID,
			GroupID:         &data.ContractID,
		},
	}
	_, _, err = notif.Insert(trendlymodels.USER_COLLECTION, data.Contract.UserID)
	if err != nil {
		log.Printf("Failed to send push notification: %v", err)
	}

	if influencer.Email != nil {
		emailData := map[string]interface{}{
			"InfluencerName": influencer.Name,
			"BrandName":      data.Brand.Name,
			"CollabTitle":    collabName,
			"NewDate":        newDateStr,
			"ContractLink":   fmt.Sprintf("%s/contracts/%s", constants.TRENDLY_CREATORS_FE, data.ContractID),
		}
		err = myemail.SendCustomHTMLEmail(*influencer.Email, templates.PostRescheduledInfluencer, templates.SubjectPostRescheduledByBrand, emailData)
		if err != nil {
			log.Printf("Failed to send reschedule email to influencer: %v", err)
		}
	}

	// 4. Send Stream System Message
	streamMessage := fmt.Sprintf("🗓️ **Posting Rescheduled!**\n\n%s has updated the posting date to **%s**.", data.Brand.Name, newDateStr)
	err = streamchat.SendSystemMessage(data.Contract.StreamChannelID, streamMessage)
	if err != nil {
		log.Printf("Failed to send stream message: %v", err)
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Posting rescheduled successfully",
	})
}

type MarkPostedReq struct {
	ProofScreenshot string `json:"proofScreenshot" binding:"required"`
	PostURL         string `json:"postUrl" binding:"required"`
	Notes           string `json:"notes"`
}

func MarkPosted(c *gin.Context) {
	var req MarkPostedReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Invalid request payload"})
		return
	}

	data, err := initializeData(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Failed to retrieve initialization data"})
		return
	}

	// 1. Update Contract Posting and Status
	if data.Contract.Posting == nil {
		data.Contract.Posting = &trendlymodels.Posting{}
	}
	data.Contract.Posting.ProofScreenshot = req.ProofScreenshot
	data.Contract.Posting.PostURL = req.PostURL
	data.Contract.Posting.Notes = req.Notes
	data.Contract.Posting.Status = "posted"
	data.Contract.Status = 9 // Posted Status

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
	notifToBrand := &trendlymodels.Notification{
		Title:       "Post is Live! 🚀",
		Description: fmt.Sprintf("%s has posted the content for %s. Review it within 2 days!", influencer.Name, collabName),
		TimeStamp:   time.Now().UnixMilli(),
		IsRead:      false,
		Type:        "post-live",
		Data: &trendlymodels.NotificationData{
			CollaborationID: &data.Contract.CollaborationID,
			GroupID:         &data.ContractID,
		},
	}
	_, brandEmails, err := notifToBrand.Insert(trendlymodels.BRAND_COLLECTION, data.Contract.BrandID)
	if err == nil && len(brandEmails) > 0 {
		emailDataBrand := map[string]interface{}{
			"BrandMemberName": data.Brand.Name,
			"InfluencerName":  influencer.Name,
			"CollabTitle":     collabName,
			"PostURL":         req.PostURL,
			"Notes":           req.Notes,
			"ProofScreenshot": req.ProofScreenshot,
			"ReviewLink":      fmt.Sprintf("%s/contracts/%s", constants.TRENDLY_BRANDS_FE, data.ContractID),
		}
		err = myemail.SendCustomHTMLEmailToMultipleRecipients(brandEmails, templates.PostMarkedLiveBrand, templates.SubjectPostMarkedLiveForBrand, emailDataBrand)
		if err != nil {
			log.Printf("Failed to send post live email to brand: %v", err)
		}
	}

	// 4. Notify Influencer (Push & Email)
	notifToInfluencer := &trendlymodels.Notification{
		Title:       "Congratulations! 🎉",
		Description: "Your post is live. Funds will be released in 2 days after brand review.",
		TimeStamp:   time.Now().UnixMilli(),
		IsRead:      false,
		Type:        "post-live-confirmation",
		Data: &trendlymodels.NotificationData{
			CollaborationID: &data.Contract.CollaborationID,
			GroupID:         &data.ContractID,
		},
	}
	_, _, err = notifToInfluencer.Insert(trendlymodels.USER_COLLECTION, data.Contract.UserID)
	if err == nil && influencer.Email != nil {
		emailDataInfluencer := map[string]interface{}{
			"InfluencerName": influencer.Name,
			"BrandName":      data.Brand.Name,
			"CollabTitle":    collabName,
			"ReviewLink":     fmt.Sprintf("%s/contracts/%s", constants.TRENDLY_CREATORS_FE, data.ContractID),
		}
		err = myemail.SendCustomHTMLEmail(*influencer.Email, templates.PostMarkedLiveInfluencer, templates.SubjectPostMarkedLiveForInfluencer, emailDataInfluencer)
		if err != nil {
			log.Printf("Failed to send post live email to influencer: %v", err)
		}
	}

	// 5. Send Stream System Message
	streamMessage := fmt.Sprintf("🚀 **Post is Live!**\n\n%s has officially shared the post! Check it out here: %s\n\n**Note:** Brands have 2 days to review before payment release. ✨", influencer.Name, req.PostURL)
	if req.Notes != "" {
		streamMessage += fmt.Sprintf("\n\n**Note from Creator:** %s", req.Notes)
	}
	err = streamchat.SendSystemMessage(data.Contract.StreamChannelID, streamMessage)
	if err != nil {
		log.Printf("Failed to send stream message: %v", err)
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Post marked as live successfully",
	})
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
