package monetize

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/payments"
	"github.com/idivarts/backend-sls/pkg/payments/webhook"
	"github.com/idivarts/backend-sls/pkg/streamchat"
)

// transfer.processed
// transfer.failed

func TransferWebhook(c *gin.Context) {
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

	log.Printf("Received Razorpay Transfer Event: %s", event.Event)

	if event.Event == "transfer.processed" && event.Payload.Transfer != nil {
		handleTransferProcessed(&event.Payload.Transfer.Entity)
	}

	c.JSON(http.StatusOK, gin.H{"status": "received"})
}

func handleTransferProcessed(transfer *webhook.TransferEntity) {
	if transfer == nil {
		return
	}

	transferID := transfer.ID
	contractID := ""
	if transfer.Notes != nil {
		if v, ok := transfer.Notes["contractId"].(string); ok {
			contractID = v
		}
	}

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
	contract.Status = trendlymodels.ContractStatusSettled

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
