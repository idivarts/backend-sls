package monetize

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/constants"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/myemail"
	"github.com/idivarts/backend-sls/pkg/payments"
	"github.com/idivarts/backend-sls/pkg/payments/webhook"
	"github.com/idivarts/backend-sls/pkg/streamchat"
	"github.com/idivarts/backend-sls/templates"
)

// payment.captured
// payment.failed

func PaymentWebhook(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		log.Printf("Failed to read webhook body: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read body"})
		return
	}

	signature := c.GetHeader("X-Razorpay-Signature")
	event, err := webhook.VerifyAndParse(body, signature, payments.WebhookKey)
	if err != nil {
		if errors.Is(err, webhook.ErrInvalidSignature) {
			log.Printf("Invalid Razorpay signature: %s", signature)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid signature"})
			return
		}
		log.Printf("Failed to parse Razorpay event: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to unmarshal event"})
		return
	}

	log.Printf("Received Razorpay Event: %s", event.Event)

	if event.Event == "payment.captured" && event.Payload.Payment != nil {
		handlePaymentCaptured(&event.Payload.Payment.Entity)
	} else if event.Event == "payment.failed" && event.Payload.Payment != nil {
		handlePaymentFailed(&event.Payload.Payment.Entity)
	}

	c.JSON(http.StatusOK, gin.H{"status": "received"})
}

func handlePaymentFailed(payment *webhook.PaymentEntity) {
	if payment == nil {
		return
	}

	orderID := payment.OrderID
	if orderID == "" {
		log.Printf("payment.failed: missing order_id on payment %s", payment.ID)
		return
	}

	order, err := payments.FetchOrder(orderID)
	if err != nil {
		log.Printf("payment.failed: failed to fetch order %s: %v", orderID, err)
		return
	}

	contractID := ""
	if order.Notes != nil {
		if v, ok := order.Notes["contractId"].(string); ok {
			contractID = v
		}
	}
	if contractID == "" {
		log.Printf("payment.failed: no contractId in order notes for order %s", orderID)
		return
	}

	contract := &trendlymodels.Contract{}
	err = contract.Get(contractID)
	if err != nil {
		log.Printf("payment.failed: failed to fetch contract %s for order %s: %v", contractID, orderID, err)
		return
	}

	if contract.Payment == nil || contract.Payment.OrderID != orderID {
		log.Printf("payment.failed: contract %s order mismatch (webhook order %s, contract order %v)", contractID, orderID, contract.Payment)
		return
	}

	// Do not overwrite a contract that already progressed past the pre-payment stage (e.g. late webhook after success).
	if contract.Status != trendlymodels.ContractStatusOrderCreated && contract.Status != trendlymodels.ContractStatusPaymentFailed {
		log.Printf("payment.failed: skip contract %s with status %d for order %s", contractID, contract.Status, orderID)
		return
	}

	contract.Payment.Status = trendlymodels.PaymentStatusFailed
	contract.Payment.PaymentID = payment.ID
	contract.Status = trendlymodels.ContractStatusPaymentFailed

	err = contract.Update(contractID)
	if err != nil {
		log.Printf("payment.failed: failed to update contract %s: %v", contractID, err)
		return
	}

	brand := &trendlymodels.Brand{}
	_ = brand.Get(contract.BrandID)

	collab := &trendlymodels.Collaboration{}
	_ = collab.Get(contract.CollaborationID)
	collabName := "Your Collaboration"
	if collab.Name != "" {
		collabName = collab.Name
	}

	failureDetail := "The payment could not be completed. Please try again or use a different payment method."
	if payment.ErrorDescription != nil && *payment.ErrorDescription != "" {
		failureDetail = *payment.ErrorDescription
	}

	notifToBrand := &trendlymodels.Notification{
		Title:       "Payment unsuccessful",
		Description: fmt.Sprintf("We could not complete payment for %s. Open the contract to try again.", collabName),
		TimeStamp:   time.Now().UnixMilli(),
		IsRead:      false,
		Type:        "payment-failed",
		Data: &trendlymodels.NotificationData{
			CollaborationID: &contract.CollaborationID,
			GroupID:         &contractID,
		},
	}
	_, brandEmails, _ := notifToBrand.Insert(trendlymodels.BRAND_COLLECTION, contract.BrandID)
	if len(brandEmails) > 0 {
		emailData := map[string]interface{}{
			"BrandMemberName": brand.Name,
			"CollabTitle":     collabName,
			"ContractLink":    fmt.Sprintf("%s/contracts/%s", constants.TRENDLY_BRANDS_FE, contractID),
			"FailureDetail":   failureDetail,
		}
		if err := myemail.SendCustomHTMLEmailToMultipleRecipients(brandEmails, templates.PaymentFailedBrand, templates.SubjectPaymentFailedBrand, emailData); err != nil {
			log.Printf("payment.failed: failed to send brand email for contract %s: %v", contractID, err)
		}
	}

	streamMessage := fmt.Sprintf("⚠️ **Payment unsuccessful**\n\nThe payment for **%s** did not go through. Brand managers: please retry from the contract when you are ready.", collabName)
	_ = streamchat.SendSystemMessage(contract.StreamChannelID, streamMessage)
}

func handlePaymentCaptured(payment *webhook.PaymentEntity) {
	if payment == nil {
		return
	}

	orderID := payment.OrderID
	if orderID == "" {
		log.Printf("payment.captured: missing order_id on payment %s", payment.ID)
		return
	}

	order, err := payments.FetchOrder(orderID)
	if err != nil {
		log.Printf("payment.captured: failed to fetch order %s: %v", orderID, err)
		return
	}

	contractID := ""
	if order.Notes != nil {
		if s, ok := order.Notes["contractId"].(string); ok {
			contractID = s
		}
	}

	if contractID == "" {
		log.Printf("No contractId found in notes for order: %s", orderID)
		return
	}

	// 1. Fetch Contract
	contract := &trendlymodels.Contract{}
	err = contract.Get(contractID)
	if err != nil {
		log.Printf("Failed to fetch contract %s for paid order %s: %v", contractID, orderID, err)
		return
	}

	// 2. Update Contract Status and Payment Status
	if contract.Payment == nil {
		contract.Payment = &trendlymodels.Payment{}
	}
	contract.Payment.Status = trendlymodels.PaymentStatusPaid
	contract.Payment.OrderID = orderID
	contract.Payment.PaymentID = payment.ID

	collab := &trendlymodels.Collaboration{}
	err = collab.Get(contract.CollaborationID)

	if collab.PromotionSubject == trendlymodels.PromotionSubjectPhysicalProduct {
		contract.Status = trendlymodels.ContractStatusShipmentPending
	} else {
		contract.Status = trendlymodels.ContractStatusDeliverablePending
	}

	err = contract.Update(contractID)
	if err != nil {
		log.Printf("Failed to update contract %s status to paid: %v", contractID, err)
		return
	}

	// 3. Prepare Data for notifications
	brand := &trendlymodels.Brand{}
	brand.Get(contract.BrandID)

	influencer := &trendlymodels.User{}
	influencer.Get(contract.UserID)

	collabName := "Your Collaboration"
	if collab.Name != "" {
		collabName = collab.Name
	}

	// 4. Notify Brand
	notifToBrand := &trendlymodels.Notification{
		Title:       "Payment Successful! ✅",
		Description: fmt.Sprintf("Your payment for %s was successful. Influencer has been notified.", collabName),
		TimeStamp:   time.Now().UnixMilli(),
		IsRead:      false,
		Type:        "payment-success",
		Data: &trendlymodels.NotificationData{
			CollaborationID: &contract.CollaborationID,
			GroupID:         &contractID,
		},
	}
	_, brandEmails, _ := notifToBrand.Insert(trendlymodels.BRAND_COLLECTION, contract.BrandID)
	if len(brandEmails) > 0 {
		emailDataBrand := map[string]interface{}{
			"BrandMemberName": brand.Name,
			"CollabTitle":     collabName,
			"ContractLink":    fmt.Sprintf("%s/contracts/%s", constants.GetBrandsFronted(), contractID),
		}
		if err := myemail.SendCustomHTMLEmailToMultipleRecipients(brandEmails, templates.PaymentReceivedContractStartedBrand, templates.SubjectPaymentReceivedContractStartedBrand, emailDataBrand); err != nil {
			log.Printf("order.paid: failed to send brand email for contract %s: %v", contractID, err)
		}
	}

	// 5. Notify Influencer
	notifToInfluencer := &trendlymodels.Notification{
		Title:       "Payment Received! 💰",
		Description: fmt.Sprintf("%s has completed the payment for %s. You can start working!", brand.Name, collabName),
		TimeStamp:   time.Now().UnixMilli(),
		IsRead:      false,
		Type:        "payment-received",
		Data: &trendlymodels.NotificationData{
			CollaborationID: &contract.CollaborationID,
			GroupID:         &contractID,
		},
	}
	_, _, _ = notifToInfluencer.Insert(trendlymodels.USER_COLLECTION, contract.UserID)
	if influencer.Email != nil && *influencer.Email != "" {
		emailDataInfluencer := map[string]interface{}{
			"InfluencerName": influencer.Name,
			"BrandName":      brand.Name,
			"CollabTitle":    collabName,
			"ContractLink":   fmt.Sprintf("%s/contracts/%s", constants.GetCreatorsFronted(), contractID),
		}
		if err := myemail.SendCustomHTMLEmail(*influencer.Email, templates.ContractFundedStartedInfluencer, templates.SubjectContractFundedStartedInfluencer, emailDataInfluencer); err != nil {
			log.Printf("order.paid: failed to send influencer email for contract %s: %v", contractID, err)
		}
	}

	// 6. Stream System Message
	streamMessage := fmt.Sprintf("💳 **Payment Confirmed!**\n\nBrand has successfully deposited the funds. Influencer is now authorized to proceed with the content creation! 🚀")
	_ = streamchat.SendSystemMessage(contract.StreamChannelID, streamMessage)
}
