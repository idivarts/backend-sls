package monetize

import (
	"encoding/json"
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
	"github.com/idivarts/backend-sls/pkg/streamchat"
	"github.com/idivarts/backend-sls/templates"
)

type RazorpayEvent struct {
	Entity    string                 `json:"entity"`
	AccountID string                 `json:"account_id"`
	Event     string                 `json:"event"`
	Payload   map[string]interface{} `json:"payload"`
}

func PaymentWebhook(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		log.Printf("Failed to read webhook body: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read body"})
		return
	}

	signature := c.GetHeader("X-Razorpay-Signature")
	if !payments.VerifyWebhookSignature(body, signature, payments.WebhookKey) {
		log.Printf("Invalid Razorpay signature: %s", signature)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid signature"})
		return
	}

	var event RazorpayEvent
	if err := json.Unmarshal(body, &event); err != nil {
		log.Printf("Failed to unmarshal Razorpay event: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to unmarshal event"})
		return
	}

	log.Printf("Received Razorpay Event: %s", event.Event)

	if event.Event == "order.paid" {
		handleOrderPaid(event.Payload)
	}

	c.JSON(http.StatusOK, gin.H{"status": "received"})
}

func handleOrderPaid(payload map[string]interface{}) {
	orderPayload, ok := payload["order"].(map[string]interface{})
	if !ok {
		return
	}

	entity, ok := orderPayload["entity"].(map[string]interface{})
	if !ok {
		return
	}

	orderID, _ := entity["id"].(string)
	notes, _ := entity["notes"].(map[string]interface{})
	contractID, _ := notes["contractId"].(string)

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
	contract.Status = 3 // Paid Status

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

	collab := &trendlymodels.Collaboration{}
	err = collab.Get(contract.CollaborationID)
	collabName := "Your Collaboration"
	if err == nil {
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

func TransferWebhook(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		log.Printf("Failed to read webhook body: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read body"})
		return
	}

	signature := c.GetHeader("X-Razorpay-Signature")
	if !payments.VerifyWebhookSignature(body, signature, payments.WebhookKey) {
		log.Printf("Invalid Razorpay signature: %s", signature)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid signature"})
		return
	}

	var event RazorpayEvent
	if err := json.Unmarshal(body, &event); err != nil {
		log.Printf("Failed to unmarshal Razorpay event: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to unmarshal event"})
		return
	}

	log.Printf("Received Razorpay Transfer Event: %s", event.Event)

	if event.Event == "transfer.processed" {
		handleTransferProcessed(event.Payload)
	}

	c.JSON(http.StatusOK, gin.H{"status": "received"})
}

func handleTransferProcessed(payload map[string]interface{}) {
	transferPayload, ok := payload["transfer"].(map[string]interface{})
	if !ok {
		return
	}

	entity, ok := transferPayload["entity"].(map[string]interface{})
	if !ok {
		return
	}

	transferID, _ := entity["id"].(string)
	notes, _ := entity["notes"].(map[string]interface{})
	contractID, _ := notes["contractId"].(string)

	if contractID == "" {
		log.Printf("No contractId found in notes for transfer: %s", transferID)
		return
	}

	// 1. Fetch Contract
	contract := &trendlymodels.Contract{}
	err := contract.Get(contractID)
	if err != nil {
		log.Printf("Failed to fetch contract %s for processed transfer %s: %v", contractID, transferID, err)
		return
	}

	// 2. Update Contract Status to Settled (10)
	if contract.Payment == nil {
		contract.Payment = &trendlymodels.Payment{}
	}
	contract.Payment.TransferID = transferID
	contract.Status = 10 // Settled/Closed Status

	err = contract.Update(contractID)
	if err != nil {
		log.Printf("Failed to update contract %s status to settled: %v", contractID, err)
		return
	}

	// 3. Prepare Data for notifications
	brand := &trendlymodels.Brand{}
	brand.Get(contract.BrandID)

	influencer := &trendlymodels.User{}
	influencer.Get(contract.UserID)

	collab := &trendlymodels.Collaboration{}
	err = collab.Get(contract.CollaborationID)
	collabName := "Your Collaboration"
	if err == nil {
		collabName = collab.Name
	}

	// 4. Notify Influencer (Payout Completed)
	notifToInfluencer := &trendlymodels.Notification{
		Title:       "Payout Completed! 💰✅",
		Description: fmt.Sprintf("The funds for your work on %s have been successfully transferred to your bank account.", collabName),
		TimeStamp:   time.Now().UnixMilli(),
		IsRead:      false,
		Type:        "payout-completed",
		Data: &trendlymodels.NotificationData{
			CollaborationID: &contract.CollaborationID,
			GroupID:         &contractID,
		},
	}
	_, _, _ = notifToInfluencer.Insert(trendlymodels.USER_COLLECTION, contract.UserID)

	// 5. Notify Brand (Collab Closed)
	notifToBrand := &trendlymodels.Notification{
		Title:       "Collaboration Closed! 🏁",
		Description: fmt.Sprintf("The collaboration for %s is now complete. The influencer has been paid and the funds are settled.", collabName),
		TimeStamp:   time.Now().UnixMilli(),
		IsRead:      false,
		Type:        "collab-closed",
		Data: &trendlymodels.NotificationData{
			CollaborationID: &contract.CollaborationID,
			GroupID:         &contractID,
		},
	}
	_, _, _ = notifToBrand.Insert(trendlymodels.BRAND_COLLECTION, contract.BrandID)

	// 6. Stream System Message
	streamMessage := fmt.Sprintf("🏁 **Workflow Complete!**\n\nThe contractual obligations for this collaboration have been met. Funds have been settled to the influencer. This group chat is now for historical reference. Thank you both! ✨")
	_ = streamchat.SendSystemMessage(contract.StreamChannelID, streamMessage)
}
