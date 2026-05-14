package monetize

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/constants"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/myemail"
	"github.com/idivarts/backend-sls/pkg/payments"
	"github.com/idivarts/backend-sls/pkg/streamchat"
	"github.com/idivarts/backend-sls/templates"
)

type RequestCancellationReq struct {
	Reason string `json:"reason" binding:"required"`
}

func RequestCancellationAsInfluencer(c *gin.Context) {
	requestCancellation(c, "influencer")
}

func RequestCancellationAsBrand(c *gin.Context) {
	requestCancellation(c, "brand")
}

func requestCancellation(c *gin.Context, role string) {
	var req RequestCancellationReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Invalid request payload"})
		return
	}

	data, err := initializeData(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Failed to retrieve initialization data"})
		return
	}

	if data.Contract.Status == trendlymodels.ContractStatusCancelled ||
		data.Contract.Status == trendlymodels.ContractStatusSettled {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Contract is already closed"})
		return
	}
	if data.Contract.Status == trendlymodels.ContractStatusDisputed {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Cannot request cancellation while a dispute is open"})
		return
	}
	if data.Contract.CancellationReq != nil && data.Contract.CancellationReq.Status == constants.CancellationStatusPending {
		c.JSON(http.StatusBadRequest, gin.H{"message": "A cancellation request is already pending"})
		return
	}

	// Influencer cannot cancel after posting is scheduled (status 8+)
	if role == "influencer" && data.Contract.Status >= trendlymodels.ContractStatusPostScheduled {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Cancellation is not allowed after posting has been scheduled"})
		return
	}

	requestedBy, _ := middlewares.GetUserId(c)

	// For pre-payment statuses (0, 1, 2) — auto-cancel, no approval needed
	if data.Contract.Status <= trendlymodels.ContractStatusPaymentFailed {
		now := time.Now().UnixMilli()
		data.Contract.Status = trendlymodels.ContractStatusCancelled
		data.Contract.ContractTimestamp.EndedOn = &now
		data.Contract.CancellationReq = &trendlymodels.CancellationRequest{
			RequestedBy:     requestedBy,
			RequestedByRole: role,
			Reason:          req.Reason,
			Status:          constants.CancellationStatusApproved,
			RequestedAt:     now,
			RespondedAt:     now,
			RefundAmount:    0,
		}
		data.Contract.Activity = append(data.Contract.Activity, trendlymodels.Activity{
			Type:   "contract_cancelled",
			Time:   now,
			Detail: fmt.Sprintf("%s cancelled contract before payment (auto-approved)", role),
		})
		if err := data.Contract.Update(data.ContractID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Failed to cancel contract"})
			return
		}
		sendCancellationApprovedNotifications(data, role, req.Reason, 0)
		c.JSON(http.StatusOK, gin.H{"message": "Contract cancelled successfully", "autoCancelled": true})
		return
	}

	// For later statuses — request approval from the other party
	data.Contract.CancellationReq = &trendlymodels.CancellationRequest{
		RequestedBy:     requestedBy,
		RequestedByRole: role,
		Reason:          req.Reason,
		Status:          constants.CancellationStatusPending,
		RequestedAt:     time.Now().UnixMilli(),
	}
	data.Contract.Activity = append(data.Contract.Activity, trendlymodels.Activity{
		Type:   "cancellation_requested",
		Time:   time.Now().UnixMilli(),
		Detail: fmt.Sprintf("%s requested cancellation: %s", role, req.Reason),
	})

	if err := data.Contract.Update(data.ContractID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Failed to save cancellation request"})
		return
	}

	sendCancellationRequestedNotifications(data, role, req.Reason)

	c.JSON(http.StatusOK, gin.H{"message": "Cancellation request sent. Waiting for the other party to respond."})
}

type RespondToCancellationReq struct {
	Approve bool `json:"approve"` // true = approve, false = reject
}

func RespondToCancellationAsInfluencer(c *gin.Context) {
	respondToCancellation(c, "influencer")
}

func RespondToCancellationAsBrand(c *gin.Context) {
	respondToCancellation(c, "brand")
}

func respondToCancellation(c *gin.Context, responderRole string) {
	var req RespondToCancellationReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Invalid request payload"})
		return
	}

	data, err := initializeData(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Failed to retrieve initialization data"})
		return
	}

	if data.Contract.CancellationReq == nil || data.Contract.CancellationReq.Status != constants.CancellationStatusPending {
		c.JSON(http.StatusBadRequest, gin.H{"message": "No pending cancellation request found"})
		return
	}

	// Ensure the responder is the other party (not the requester)
	if data.Contract.CancellationReq.RequestedByRole == responderRole {
		c.JSON(http.StatusBadRequest, gin.H{"message": "You cannot respond to your own cancellation request"})
		return
	}

	now := time.Now().UnixMilli()

	if !req.Approve {
		data.Contract.CancellationReq.Status = constants.CancellationStatusRejected
		data.Contract.CancellationReq.RespondedAt = now
		data.Contract.Activity = append(data.Contract.Activity, trendlymodels.Activity{
			Type:   "cancellation_rejected",
			Time:   now,
			Detail: fmt.Sprintf("%s rejected the cancellation request", responderRole),
		})
		if err := data.Contract.Update(data.ContractID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Failed to update contract"})
			return
		}
		sendCancellationRejectedNotifications(data, responderRole)
		c.JSON(http.StatusOK, gin.H{"message": "Cancellation request rejected"})
		return
	}

	// Approved — calculate refund based on contract stage
	refundAmountPaise := calculateRefundAmount(data.Contract)

	// Issue refund if applicable
	if refundAmountPaise > 0 && data.Contract.Payment != nil && data.Contract.Payment.PaymentID != "" {
		_, err := payments.CreateRefund(data.Contract.Payment.PaymentID, refundAmountPaise, "Mutual contract cancellation")
		if err != nil {
			log.Printf("Refund failed during cancellation: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Refund failed — cancellation not processed"})
			return
		}
	}

	data.Contract.CancellationReq.Status = constants.CancellationStatusApproved
	data.Contract.CancellationReq.RespondedAt = now
	data.Contract.CancellationReq.RefundAmount = refundAmountPaise
	data.Contract.Status = trendlymodels.ContractStatusCancelled
	data.Contract.ContractTimestamp.EndedOn = &now
	data.Contract.Activity = append(data.Contract.Activity, trendlymodels.Activity{
		Type:   "contract_cancelled",
		Time:   now,
		Detail: fmt.Sprintf("%s approved cancellation (refund: ₹%d)", responderRole, refundAmountPaise/100),
	})

	if err := data.Contract.Update(data.ContractID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Failed to update contract"})
		return
	}

	sendCancellationApprovedNotifications(data, data.Contract.CancellationReq.RequestedByRole, data.Contract.CancellationReq.Reason, refundAmountPaise)

	c.JSON(http.StatusOK, gin.H{
		"message":      "Contract cancelled successfully",
		"refundAmount": refundAmountPaise,
	})
}

// calculateRefundAmount returns refund in paise based on contract stage.
// Follows the plan's refund schedule.
func calculateRefundAmount(contract *trendlymodels.Contract) int64 {
	if contract.Payment == nil || contract.Payment.Amount == 0 {
		return 0 // barter or unpaid
	}
	fullAmountPaise := int64(contract.Payment.Amount) * 100

	switch contract.Status {
	case trendlymodels.ContractStatusShipmentPending:
		return fullAmountPaise // full refund before shipping
	case trendlymodels.ContractStatusDeliverablePending:
		return fullAmountPaise / 2 // 50% — product already used
	case trendlymodels.ContractStatusDeliverableSent, trendlymodels.ContractStatusPostScheduled,
		trendlymodels.ContractStatusPostDone:
		return 0 // work done, no refund
	default:
		// Status 4 (Shipped) or 5 (Delivered) — needs admin judgment; return 0 and let admin adjust
		return 0
	}
}

func sendCancellationRequestedNotifications(data *struct {
	ContractID string
	Contract   *trendlymodels.Contract
	Brand      *trendlymodels.Brand
}, requesterRole, reason string) {
	collab := &trendlymodels.Collaboration{}
	_ = collab.Get(data.Contract.CollaborationID)
	collabName := collab.Name
	if collabName == "" {
		collabName = "Your Collaboration"
	}

	influencer := &trendlymodels.User{}
	_ = influencer.Get(data.Contract.UserID)

	// Notify the other party
	otherPartyDesc := fmt.Sprintf("A cancellation has been requested for the contract for %s. Please review and respond.", collabName)

	if requesterRole == "influencer" {
		// Notify brand
		notifBrand := &trendlymodels.Notification{
			Title:       "Cancellation Requested ❌",
			Description: otherPartyDesc,
			TimeStamp:   time.Now().UnixMilli(),
			IsRead:      false,
			Type:        "cancellation-requested",
			Data: &trendlymodels.NotificationData{
				CollaborationID: &data.Contract.CollaborationID,
				GroupID:         &data.ContractID,
			},
		}
		_, brandEmails, _ := notifBrand.Insert(trendlymodels.BRAND_COLLECTION, data.Contract.BrandID)
		if len(brandEmails) > 0 {
			emailData := map[string]interface{}{
				"RecipientName":  data.Brand.Name,
				"OtherPartyName": influencer.Name,
				"CollabTitle":    collabName,
				"Reason":         reason,
				"ContractLink":   fmt.Sprintf("%s/contract-details/%s", constants.TRENDLY_BRANDS_FE, data.ContractID),
			}
			_ = myemail.SendCustomHTMLEmailToMultipleRecipients(brandEmails, templates.CancellationRequested, templates.SubjectCancellationRequested, emailData)
		}
	} else {
		// Notify influencer
		notifInfluencer := &trendlymodels.Notification{
			Title:       "Cancellation Requested ❌",
			Description: otherPartyDesc,
			TimeStamp:   time.Now().UnixMilli(),
			IsRead:      false,
			Type:        "cancellation-requested",
			Data: &trendlymodels.NotificationData{
				CollaborationID: &data.Contract.CollaborationID,
				GroupID:         &data.ContractID,
			},
		}
		_, _, _ = notifInfluencer.Insert(trendlymodels.USER_COLLECTION, data.Contract.UserID)
		if influencer.Email != nil {
			emailData := map[string]interface{}{
				"RecipientName":  influencer.Name,
				"OtherPartyName": data.Brand.Name,
				"CollabTitle":    collabName,
				"Reason":         reason,
				"ContractLink":   fmt.Sprintf("%s/contract-details/%s", constants.TRENDLY_CREATORS_FE, data.ContractID),
			}
			_ = myemail.SendCustomHTMLEmail(*influencer.Email, templates.CancellationRequested, templates.SubjectCancellationRequested, emailData)
		}
	}

	streamMsg := fmt.Sprintf("❌ **Cancellation Requested**\n\n**%s** has requested to cancel this contract.\n**Reason:** %s\n\nThe other party must approve or reject this request.", requesterRole, reason)
	_ = streamchat.SendSystemMessage(data.Contract.StreamChannelID, streamMsg)
}

func sendCancellationApprovedNotifications(data *struct {
	ContractID string
	Contract   *trendlymodels.Contract
	Brand      *trendlymodels.Brand
}, requesterRole, reason string, refundAmountPaise int64) {
	collab := &trendlymodels.Collaboration{}
	_ = collab.Get(data.Contract.CollaborationID)
	collabName := collab.Name
	if collabName == "" {
		collabName = "Your Collaboration"
	}

	influencer := &trendlymodels.User{}
	_ = influencer.Get(data.Contract.UserID)

	refundLabel := ""
	if refundAmountPaise > 0 {
		refundLabel = fmt.Sprintf("₹%d", refundAmountPaise/100)
	}

	// Notify influencer
	notifInfluencer := &trendlymodels.Notification{
		Title:       "Contract Cancelled",
		Description: fmt.Sprintf("The contract for %s has been cancelled.", collabName),
		TimeStamp:   time.Now().UnixMilli(),
		IsRead:      false,
		Type:        "contract-cancelled",
		Data: &trendlymodels.NotificationData{
			CollaborationID: &data.Contract.CollaborationID,
			GroupID:         &data.ContractID,
		},
	}
	_, _, _ = notifInfluencer.Insert(trendlymodels.USER_COLLECTION, data.Contract.UserID)

	// Notify brand
	notifBrand := &trendlymodels.Notification{
		Title:       "Contract Cancelled",
		Description: fmt.Sprintf("The contract for %s has been cancelled.", collabName),
		TimeStamp:   time.Now().UnixMilli(),
		IsRead:      false,
		Type:        "contract-cancelled",
		Data: &trendlymodels.NotificationData{
			CollaborationID: &data.Contract.CollaborationID,
			GroupID:         &data.ContractID,
		},
	}
	_, brandEmails, _ := notifBrand.Insert(trendlymodels.BRAND_COLLECTION, data.Contract.BrandID)

	if influencer.Email != nil {
		emailData := map[string]interface{}{
			"RecipientName": influencer.Name,
			"BrandName":     data.Brand.Name,
			"CollabTitle":   collabName,
			"Reason":        reason,
			"RefundAmount":  refundLabel,
			"ContractLink":  fmt.Sprintf("%s/contract-details/%s", constants.TRENDLY_CREATORS_FE, data.ContractID),
		}
		_ = myemail.SendCustomHTMLEmail(*influencer.Email, templates.CancellationApproved, templates.SubjectCancellationApproved, emailData)
	}
	if len(brandEmails) > 0 {
		emailData := map[string]interface{}{
			"RecipientName":  data.Brand.Name,
			"InfluencerName": influencer.Name,
			"CollabTitle":    collabName,
			"Reason":         reason,
			"RefundAmount":   refundLabel,
			"ContractLink":   fmt.Sprintf("%s/contract-details/%s", constants.TRENDLY_BRANDS_FE, data.ContractID),
		}
		_ = myemail.SendCustomHTMLEmailToMultipleRecipients(brandEmails, templates.CancellationApproved, templates.SubjectCancellationApproved, emailData)
	}

	streamMsg := "❌ **Contract Cancelled**\n\nThis contract has been cancelled by mutual agreement."
	if refundLabel != "" {
		streamMsg += fmt.Sprintf("\n**Refund issued to brand:** %s", refundLabel)
	}
	_ = streamchat.SendSystemMessage(data.Contract.StreamChannelID, streamMsg)
}

func sendCancellationRejectedNotifications(data *struct {
	ContractID string
	Contract   *trendlymodels.Contract
	Brand      *trendlymodels.Brand
}, responderRole string) {
	collab := &trendlymodels.Collaboration{}
	_ = collab.Get(data.Contract.CollaborationID)
	collabName := collab.Name
	if collabName == "" {
		collabName = "Your Collaboration"
	}

	influencer := &trendlymodels.User{}
	_ = influencer.Get(data.Contract.UserID)

	requesterRole := data.Contract.CancellationReq.RequestedByRole
	if requesterRole == "influencer" {
		// Notify influencer their request was rejected
		notif := &trendlymodels.Notification{
			Title:       "Cancellation Rejected",
			Description: fmt.Sprintf("Your cancellation request for %s was rejected by the brand.", collabName),
			TimeStamp:   time.Now().UnixMilli(),
			IsRead:      false,
			Type:        "cancellation-rejected",
			Data: &trendlymodels.NotificationData{
				CollaborationID: &data.Contract.CollaborationID,
				GroupID:         &data.ContractID,
			},
		}
		_, _, _ = notif.Insert(trendlymodels.USER_COLLECTION, data.Contract.UserID)
		if influencer.Email != nil {
			emailData := map[string]interface{}{
				"RecipientName": influencer.Name,
				"BrandName":     data.Brand.Name,
				"CollabTitle":   collabName,
				"ContractLink":  fmt.Sprintf("%s/contract-details/%s", constants.TRENDLY_CREATORS_FE, data.ContractID),
			}
			_ = myemail.SendCustomHTMLEmail(*influencer.Email, templates.CancellationRejected, templates.SubjectCancellationRejected, emailData)
		}
	} else {
		// Notify brand their request was rejected
		notif := &trendlymodels.Notification{
			Title:       "Cancellation Rejected",
			Description: fmt.Sprintf("Your cancellation request for %s was rejected by the influencer.", collabName),
			TimeStamp:   time.Now().UnixMilli(),
			IsRead:      false,
			Type:        "cancellation-rejected",
			Data: &trendlymodels.NotificationData{
				CollaborationID: &data.Contract.CollaborationID,
				GroupID:         &data.ContractID,
			},
		}
		_, brandEmails, _ := notif.Insert(trendlymodels.BRAND_COLLECTION, data.Contract.BrandID)
		if len(brandEmails) > 0 {
			emailData := map[string]interface{}{
				"RecipientName":  data.Brand.Name,
				"InfluencerName": influencer.Name,
				"CollabTitle":    collabName,
				"ContractLink":   fmt.Sprintf("%s/contract-details/%s", constants.TRENDLY_BRANDS_FE, data.ContractID),
			}
			_ = myemail.SendCustomHTMLEmailToMultipleRecipients(brandEmails, templates.CancellationRejected, templates.SubjectCancellationRejected, emailData)
		}
	}

	streamMsg := fmt.Sprintf("The cancellation request has been rejected by %s. The contract continues as normal.", responderRole)
	_ = streamchat.SendSystemMessage(data.Contract.StreamChannelID, streamMsg)
}
