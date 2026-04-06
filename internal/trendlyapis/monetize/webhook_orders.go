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

// order.paid
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

	if event.Event == "order.paid" && event.Payload.Order != nil {
		handleOrderPaid(&event.Payload.Order.Entity)
	}

	c.JSON(http.StatusOK, gin.H{"status": "received"})
}

func handleOrderPaid(order *webhook.OrderEntity) {
	if order == nil {
		return
	}

	orderID := order.ID
	contractID := ""
	if order.Notes != nil {
		contractID = order.Notes["contractId"]
	}

	if contractID == "" {
		log.Printf("No contractId found in notes for order: %s", orderID)
		return
	}

	// 1. Fetch Contract
	contract := &trendlymodels.Contract{}
	err := contract.Get(contractID)
	if err != nil {
		log.Printf("Failed to fetch contract %s for paid order %s: %v", contractID, orderID, err)
		return
	}

	// 2. Update Contract Status and Payment Status
	if contract.Payment == nil {
		contract.Payment = &trendlymodels.Payment{}
	}
	contract.Payment.Status = "paid"
	contract.Payment.OrderID = orderID

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
			"ContractLink":    fmt.Sprintf("%s/contracts/%s", constants.TRENDLY_BRANDS_FE, contractID),
		}
		_ = myemail.SendCustomHTMLEmailToMultipleRecipients(brandEmails, templates.PaymentOrderCreated, templates.SubjectPaymentOrderCreated, emailDataBrand) // Reusing template for now or use a proper success one
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
	if influencer.Email != nil {
		emailDataInfluencer := map[string]interface{}{
			"InfluencerName": influencer.Name,
			"BrandName":      brand.Name,
			"CollabTitle":    collabName,
			"ContractLink":   fmt.Sprintf("%s/contracts/%s", constants.TRENDLY_CREATORS_FE, contractID),
		}
		_ = myemail.SendCustomHTMLEmail(*influencer.Email, templates.ShipmentMarked, templates.SubjectShipmentMarked, emailDataInfluencer) // Reusing or custom
	}

	// 6. Stream System Message
	streamMessage := fmt.Sprintf("💳 **Payment Confirmed!**\n\nBrand has successfully deposited the funds. Influencer is now authorized to proceed with the content creation! 🚀")
	_ = streamchat.SendSystemMessage(contract.StreamChannelID, streamMessage)
}
