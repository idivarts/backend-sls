package monetize

import (
	"context"
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
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
	"github.com/idivarts/backend-sls/templates"
	"google.golang.org/api/iterator"
)

type RaiseDisputeReq struct {
	Type        string   `json:"type" binding:"required"`
	Description string   `json:"description" binding:"required"`
	Evidence    []string `json:"evidence"` // S3 URLs, optional
}

func RaiseDisputeAsInfluencer(c *gin.Context) {
	raiseDispute(c, "influencer")
}

func RaiseDisputeAsBrand(c *gin.Context) {
	raiseDispute(c, "brand")
}

func raiseDispute(c *gin.Context, role string) {
	var req RaiseDisputeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Invalid request payload"})
		return
	}

	data, err := initializeData(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Failed to retrieve initialization data"})
		return
	}

	if data.Contract.Status == trendlymodels.ContractStatusDisputed {
		c.JSON(http.StatusBadRequest, gin.H{"message": "A dispute is already open on this contract"})
		return
	}
	if data.Contract.Status == trendlymodels.ContractStatusCancelled ||
		data.Contract.Status == trendlymodels.ContractStatusSettled {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Cannot raise a dispute on a completed or cancelled contract"})
		return
	}

	raisedBy, _ := middlewares.GetUserId(c)

	// Freeze the contract at its current status
	data.Contract.Dispute = &trendlymodels.DisputeDetails{
		RaisedBy:     raisedBy,
		RaisedByRole: role,
		Type:         req.Type,
		Description:  req.Description,
		Evidence:     req.Evidence,
		Status:       constants.DisputeStatusOpen,
		RaisedAt:     time.Now().UnixMilli(),
	}
	data.Contract.Status = trendlymodels.ContractStatusDisputed
	data.Contract.Activity = append(data.Contract.Activity, trendlymodels.Activity{
		Type:   "dispute_raised",
		Time:   time.Now().UnixMilli(),
		Detail: fmt.Sprintf("%s raised a dispute: %s", role, req.Type),
	})

	if err := data.Contract.Update(data.ContractID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Failed to update contract"})
		return
	}

	// Fetch both parties for notifications
	influencer := &trendlymodels.User{}
	_ = influencer.Get(data.Contract.UserID)

	collab := &trendlymodels.Collaboration{}
	_ = collab.Get(data.Contract.CollaborationID)
	collabName := collab.Name
	if collabName == "" {
		collabName = "Your Collaboration"
	}

	// Notify influencer
	notifInfluencer := &trendlymodels.Notification{
		Title:       "Dispute Raised ⚠️",
		Description: fmt.Sprintf("A dispute has been raised on your contract for %s. Our team will review and get back to you.", collabName),
		TimeStamp:   time.Now().UnixMilli(),
		IsRead:      false,
		Type:        "dispute-raised",
		Data: &trendlymodels.NotificationData{
			CollaborationID: &data.Contract.CollaborationID,
			GroupID:         &data.ContractID,
		},
	}
	_, _, _ = notifInfluencer.Insert(trendlymodels.USER_COLLECTION, data.Contract.UserID)

	// Notify brand
	notifBrand := &trendlymodels.Notification{
		Title:       "Dispute Raised ⚠️",
		Description: fmt.Sprintf("A dispute has been raised on your contract for %s. Our team will review and get back to you.", collabName),
		TimeStamp:   time.Now().UnixMilli(),
		IsRead:      false,
		Type:        "dispute-raised",
		Data: &trendlymodels.NotificationData{
			CollaborationID: &data.Contract.CollaborationID,
			GroupID:         &data.ContractID,
		},
	}
	_, brandEmails, _ := notifBrand.Insert(trendlymodels.BRAND_COLLECTION, data.Contract.BrandID)

	influencerEmail := ""
	if influencer.Email != nil {
		influencerEmail = *influencer.Email
	}

	disputeTypeLabel := req.Type
	contractLinkCreator := fmt.Sprintf("%s/contract-details/%s", constants.TRENDLY_CREATORS_FE, data.ContractID)
	contractLinkBrand := fmt.Sprintf("%s/contract-details/%s", constants.TRENDLY_BRANDS_FE, data.ContractID)

	// Email influencer
	if influencerEmail != "" {
		emailData := map[string]interface{}{
			"RecipientName":   influencer.Name,
			"BrandName":       data.Brand.Name,
			"CollabTitle":     collabName,
			"DisputeType":     disputeTypeLabel,
			"Description":     req.Description,
			"RaisedBy":        role,
			"ContractLink":    contractLinkCreator,
		}
		if err := myemail.SendCustomHTMLEmail(influencerEmail, templates.DisputeRaisedInfluencer, templates.SubjectDisputeRaised, emailData); err != nil {
			log.Printf("Failed to send dispute email to influencer: %v", err)
		}
	}

	// Email brand members
	if len(brandEmails) > 0 {
		emailData := map[string]interface{}{
			"RecipientName":   data.Brand.Name,
			"InfluencerName":  influencer.Name,
			"CollabTitle":     collabName,
			"DisputeType":     disputeTypeLabel,
			"Description":     req.Description,
			"RaisedBy":        role,
			"ContractLink":    contractLinkBrand,
		}
		if err := myemail.SendCustomHTMLEmailToMultipleRecipients(brandEmails, templates.DisputeRaisedBrand, templates.SubjectDisputeRaised, emailData); err != nil {
			log.Printf("Failed to send dispute email to brand: %v", err)
		}
	}

	// Email support
	supportEmailData := map[string]interface{}{
		"ContractID":     data.ContractID,
		"BrandName":      data.Brand.Name,
		"InfluencerName": influencer.Name,
		"CollabTitle":    collabName,
		"DisputeType":    disputeTypeLabel,
		"Description":    req.Description,
		"RaisedBy":       role,
		"Evidence":       req.Evidence,
		"ContractLink":   contractLinkBrand,
	}
	if err := myemail.SendCustomHTMLEmail("support@trendly.now", templates.DisputeRaisedSupport, templates.SubjectDisputeRaisedSupport, supportEmailData); err != nil {
		log.Printf("Failed to send dispute email to support: %v", err)
	}

	// Stream message
	streamMsg := fmt.Sprintf("⚠️ **Dispute Raised**\n\nA dispute has been raised on this contract.\n**Type:** %s\n**Raised by:** %s\n\nThis contract is now on hold while our team reviews. We'll get back to both parties shortly.", req.Type, role)
	if err := streamchat.SendSystemMessage(data.Contract.StreamChannelID, streamMsg); err != nil {
		log.Printf("Failed to send stream message: %v", err)
	}

	c.JSON(http.StatusOK, gin.H{"message": "Dispute raised successfully. Contract is now on hold."})
}

type ResolveDisputeReq struct {
	Resolution   string `json:"resolution" binding:"required"`
	RefundAmount int64  `json:"refundAmount"` // paise; 0 = no refund
	NextStatus   int    `json:"nextStatus"`   // ContractStatus int to move to after resolution
}

// ResolveDispute is admin-only. Requires the calling manager to have isAdmin == true.
func ResolveDispute(c *gin.Context) {
	manager := middlewares.GetManagerModel(c)
	if !manager.IsAdmin {
		c.JSON(http.StatusForbidden, gin.H{"message": "Only Trendly admins can resolve disputes"})
		return
	}

	var req ResolveDisputeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Invalid request payload"})
		return
	}

	data, err := initializeData(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Failed to retrieve initialization data"})
		return
	}

	if data.Contract.Status != trendlymodels.ContractStatusDisputed {
		c.JSON(http.StatusBadRequest, gin.H{"message": "This contract does not have an open dispute"})
		return
	}

	adminID, _ := middlewares.GetUserId(c)

	// Issue refund if requested
	if req.RefundAmount > 0 && data.Contract.Payment != nil && data.Contract.Payment.PaymentID != "" {
		_, err := payments.CreateRefund(data.Contract.Payment.PaymentID, req.RefundAmount, "Dispute resolution: "+req.Resolution)
		if err != nil {
			log.Printf("Refund failed during dispute resolution: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Refund failed — dispute not resolved"})
			return
		}
		if data.Contract.CancellationReq != nil {
			data.Contract.CancellationReq.RefundAmount = req.RefundAmount
		}
	}

	// Resolve dispute
	now := time.Now().UnixMilli()
	data.Contract.Dispute.Status = constants.DisputeStatusResolved
	data.Contract.Dispute.Resolution = req.Resolution
	data.Contract.Dispute.AdminID = adminID
	data.Contract.Dispute.ResolvedAt = now

	// Move to next status
	nextStatus := trendlymodels.ContractStatus(req.NextStatus)
	if req.NextStatus == 0 {
		nextStatus = trendlymodels.ContractStatusCancelled
	}
	data.Contract.Status = nextStatus
	if nextStatus == trendlymodels.ContractStatusCancelled || nextStatus == trendlymodels.ContractStatusSettled {
		data.Contract.ContractTimestamp.EndedOn = &now
	}

	data.Contract.Activity = append(data.Contract.Activity, trendlymodels.Activity{
		Type:   "dispute_resolved",
		Time:   now,
		Detail: fmt.Sprintf("Admin resolved dispute: %s", req.Resolution),
	})

	if err := data.Contract.Update(data.ContractID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Failed to update contract"})
		return
	}

	// Notify both parties
	influencer := &trendlymodels.User{}
	_ = influencer.Get(data.Contract.UserID)

	collab := &trendlymodels.Collaboration{}
	_ = collab.Get(data.Contract.CollaborationID)
	collabName := collab.Name
	if collabName == "" {
		collabName = "Your Collaboration"
	}

	notifInfluencer := &trendlymodels.Notification{
		Title:       "Dispute Resolved ✅",
		Description: fmt.Sprintf("Your dispute for %s has been resolved by our team.", collabName),
		TimeStamp:   now,
		IsRead:      false,
		Type:        "dispute-resolved",
		Data: &trendlymodels.NotificationData{
			CollaborationID: &data.Contract.CollaborationID,
			GroupID:         &data.ContractID,
		},
	}
	_, _, _ = notifInfluencer.Insert(trendlymodels.USER_COLLECTION, data.Contract.UserID)

	notifBrand := &trendlymodels.Notification{
		Title:       "Dispute Resolved ✅",
		Description: fmt.Sprintf("The dispute for %s has been resolved by our team.", collabName),
		TimeStamp:   now,
		IsRead:      false,
		Type:        "dispute-resolved",
		Data: &trendlymodels.NotificationData{
			CollaborationID: &data.Contract.CollaborationID,
			GroupID:         &data.ContractID,
		},
	}
	_, brandEmails, _ := notifBrand.Insert(trendlymodels.BRAND_COLLECTION, data.Contract.BrandID)

	influencerEmail := ""
	if influencer.Email != nil {
		influencerEmail = *influencer.Email
	}

	refundInfo := ""
	if req.RefundAmount > 0 {
		refundInfo = fmt.Sprintf("₹%d", req.RefundAmount/100)
	}

	resolvedEmailData := map[string]interface{}{
		"InfluencerName": influencer.Name,
		"BrandName":      data.Brand.Name,
		"CollabTitle":    collabName,
		"Resolution":     req.Resolution,
		"RefundAmount":   refundInfo,
		"ContractLink":   fmt.Sprintf("%s/contract-details/%s", constants.TRENDLY_CREATORS_FE, data.ContractID),
	}
	if influencerEmail != "" {
		_ = myemail.SendCustomHTMLEmail(influencerEmail, templates.DisputeResolved, templates.SubjectDisputeResolved, resolvedEmailData)
	}
	if len(brandEmails) > 0 {
		resolvedEmailData["ContractLink"] = fmt.Sprintf("%s/contract-details/%s", constants.TRENDLY_BRANDS_FE, data.ContractID)
		_ = myemail.SendCustomHTMLEmailToMultipleRecipients(brandEmails, templates.DisputeResolved, templates.SubjectDisputeResolved, resolvedEmailData)
	}

	streamMsg := fmt.Sprintf("✅ **Dispute Resolved**\n\nOur team has reviewed the dispute and reached a resolution.\n**Resolution:** %s", req.Resolution)
	if req.RefundAmount > 0 {
		streamMsg += fmt.Sprintf("\n**Refund issued:** ₹%d", req.RefundAmount/100)
	}
	_ = streamchat.SendSystemMessage(data.Contract.StreamChannelID, streamMsg)

	c.JSON(http.StatusOK, gin.H{"message": "Dispute resolved successfully"})
}

// ListOpenDisputes returns all disputed contracts. Admin-only.
func ListOpenDisputes(c *gin.Context) {
	manager := middlewares.GetManagerModel(c)
	if !manager.IsAdmin {
		c.JSON(http.StatusForbidden, gin.H{"message": "Only Trendly admins can view all disputes"})
		return
	}

	iter := firestoredb.Client.Collection("contracts").
		Where("status", "==", int(trendlymodels.ContractStatusDisputed)).
		Documents(context.Background())
	defer iter.Stop()

	type DisputeListItem struct {
		ContractID string                      `json:"contractId"`
		Contract   trendlymodels.Contract      `json:"contract"`
	}

	var items []DisputeListItem
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		var contract trendlymodels.Contract
		if err := doc.DataTo(&contract); err != nil {
			log.Printf("Failed to parse contract %s: %v", doc.Ref.ID, err)
			continue
		}
		items = append(items, DisputeListItem{ContractID: doc.Ref.ID, Contract: contract})
	}

	c.JSON(http.StatusOK, gin.H{"disputes": items, "count": len(items)})
}
