package trendlyCollabs

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/constants"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/myemail"
	"github.com/idivarts/backend-sls/templates"
)

func UserFeedback(c *gin.Context) {
	var req UserFeedbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Invalid request payload"})
		return
	}

	contractId := c.Param(("contractId"))

	sessionUserID, ok := middlewares.GetUserId(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Error fetching userId from token"})
		return
	}

	contract := trendlymodels.Contract{}
	err := contract.Get(contractId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error fetching contract"})
		return
	}

	if sessionUserID != contract.UserID {
		c.JSON(http.StatusForbidden, gin.H{"message": "Only the influencer on this contract can submit feedback"})
		return
	}

	if !contractAllowsFeedback(contract.Status) {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Feedback can only be submitted when the contract is post-done or settled"})
		return
	}

	ratings := req.Ratings
	ts := time.Now().UnixMilli()
	fb := &trendlymodels.InfluencerFeedback{
		Ratings:       &ratings,
		TimeSubmitted: &ts,
	}
	if req.FeedbackReview != "" {
		review := req.FeedbackReview
		fb.FeedbackReview = &review
	}
	contract.FeedbackFromInfluencer = fb
	err = contract.Update(contractId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error updating contract"})
		return
	}

	collab := trendlymodels.Collaboration{}
	err = collab.Get(contract.CollaborationID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error fetching collaboration"})
		return
	}

	brand := trendlymodels.Brand{}
	err = brand.Get(contract.BrandID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error fetching Brand"})
		return
	}

	user := trendlymodels.User{}
	err = user.Get(contract.UserID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error fetching user"})
		return
	}

	// Send Push Notification
	notif := &trendlymodels.Notification{
		Title:       fmt.Sprintf("%s has given you a rating", user.Name),
		Description: fmt.Sprintf("You have received a new rating for the collaboration %s", collab.Name),
		IsRead:      false,
		Data: &trendlymodels.NotificationData{
			CollaborationID: &contract.CollaborationID,
			UserID:          &contract.UserID,
			GroupID:         &contractId,
		},
		TimeStamp: time.Now().UnixMilli(),
		Type:      "feedback-given",
	}
	_, emails, err := notif.Insert(trendlymodels.BRAND_COLLECTION, collab.BrandID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Send Email notification

	if len(emails) > 0 {
		data := map[string]interface{}{
			"BrandMemberName": brand.Name,
			"InfluencerName":  user.Name,
			"CollabTitle":     collab.Name,
			"FeedbackLink":    fmt.Sprintf("%s/contract-details/%s", constants.TRENDLY_BRANDS_FE, contractId),
		}
		err = myemail.SendCustomHTMLEmailToMultipleRecipients(emails, templates.CollaborationRatedByInfluencer, templates.SubjectInfluencerRatedYou, data)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "Successfully given feedback"})
}

func BrandFeedback(c *gin.Context) {
	var req BrandFeedbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Invalid request payload"})
		return
	}

	contractId := c.Param(("contractId"))

	sessionManagerID, ok := middlewares.GetUserId(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Error fetching userId from token"})
		return
	}

	contract := trendlymodels.Contract{}
	err := contract.Get(contractId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error fetching contract"})
		return
	}

	member := trendlymodels.BrandMember{}
	err = member.Get(contract.BrandID, sessionManagerID)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"message": "Only a member of this contract's brand can submit feedback"})
		return
	}

	if !contractAllowsFeedback(contract.Status) {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Feedback can only be submitted when the contract is post-done or settled"})
		return
	}

	ratings := req.Ratings
	ts := time.Now().UnixMilli()
	managerID := sessionManagerID
	fb := &trendlymodels.BrandContractFeedback{
		Ratings:       &ratings,
		ManagerID:     &managerID,
		TimeSubmitted: &ts,
	}
	if req.FeedbackReview != "" {
		review := req.FeedbackReview
		fb.FeedbackReview = &review
	}
	contract.FeedbackFromBrand = fb
	err = contract.Update(contractId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error updating contract"})
		return
	}

	collab := trendlymodels.Collaboration{}
	err = collab.Get(contract.CollaborationID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error fetching collaboration"})
		return
	}

	brand := trendlymodels.Brand{}
	err = brand.Get(contract.BrandID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error fetching Brand"})
		return
	}

	user := trendlymodels.User{}
	err = user.Get(contract.UserID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error fetching user"})
		return
	}

	// Notify influencer (push + persisted notification)
	notif := &trendlymodels.Notification{
		Title:       fmt.Sprintf("%s has given you a rating", brand.Name),
		Description: fmt.Sprintf("You have received a new rating for the collaboration %s", collab.Name),
		IsRead:      false,
		Data: &trendlymodels.NotificationData{
			CollaborationID: &contract.CollaborationID,
			UserID:          &contract.UserID,
			GroupID:         &contractId,
		},
		TimeStamp: time.Now().UnixMilli(),
		Type:      "brand-feedback-given",
	}
	_, emails, err := notif.Insert(trendlymodels.USER_COLLECTION, contract.UserID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len(emails) > 0 {
		data := map[string]interface{}{
			"InfluencerName": user.Name,
			"BrandName":      brand.Name,
			"CollabTitle":    collab.Name,
			"FeedbackLink":   fmt.Sprintf("%s/contract-details/%s", constants.GetCreatorsFronted(), contractId),
		}
		err = myemail.SendCustomHTMLEmailToMultipleRecipients(emails, templates.CollaborationRatedByBrand, templates.SubjectBrandRatedInfluencer, data)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "Successfully given feedback"})
}

func contractAllowsFeedback(status trendlymodels.ContractStatus) bool {
	switch status {
	case trendlymodels.ContractStatusPostDone, trendlymodels.ContractStatusSettled:
		return true
	default:
		return false
	}
}
