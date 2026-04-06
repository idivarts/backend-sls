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

// transfer.processed
// transfer.failed
// settlement.processed

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

	orderID, err := payments.OrderIDFromTransferSource(transfer.Source)
	if err != nil {
		log.Printf("transfer processed: could not resolve order from transfer %s: %v", transferID, err)
		return
	}

	order, err := payments.FetchOrder(orderID)
	if err != nil {
		log.Printf("transfer processed: failed to fetch order %s for transfer %s: %v", orderID, transferID, err)
		return
	}

	contractID := ""
	if order.Notes != nil {
		if v, ok := order.Notes["contractId"].(string); ok {
			contractID = v
		}
	}
	if contractID == "" {
		log.Printf("transfer processed: no contractId in order %s notes for transfer %s", orderID, transferID)
		return
	}

	// 1. Fetch Contract
	contract := &trendlymodels.Contract{}
	err = contract.Get(contractID)
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

	// 4. Notify Influencer (transfer processed — bank settlement may lag 1–2 days)
	notifToInfluencer := &trendlymodels.Notification{
		Title: "Payout initiated 💸",
		Description: fmt.Sprintf(
			"Your payout for %s has been released from Trendly. It often takes 1-2 business days for the amount to appear in your bank account, depending on your bank.",
			collabName,
		),
		TimeStamp:   time.Now().UnixMilli(),
		IsRead:      false,
		Type:        "payout-completed",
		Data: &trendlymodels.NotificationData{
			CollaborationID: &contract.CollaborationID,
			GroupID:         &contractID,
		},
	}
	_, _, _ = notifToInfluencer.Insert(trendlymodels.USER_COLLECTION, contract.UserID)
	if influencer.Email != nil && *influencer.Email != "" {
		influencerEmailData := map[string]interface{}{
			"InfluencerName": influencer.Name,
			"CollabTitle":    collabName,
			"ContractLink":   fmt.Sprintf("%s/contracts/%s", constants.TRENDLY_CREATORS_FE, contractID),
		}
		if err := myemail.SendCustomHTMLEmail(*influencer.Email, templates.PayoutTransferInfluencer, templates.SubjectPayoutTransferInfluencer, influencerEmailData); err != nil {
			log.Printf("transfer processed: failed to send influencer email for contract %s: %v", contractID, err)
		}
	}

	// 5. Notify Brand (collab closed — same settlement caveat for influencer payout)
	notifToBrand := &trendlymodels.Notification{
		Title: "Collaboration closed 🏁",
		Description: fmt.Sprintf(
			"The collaboration %s is complete and the influencer payout has been initiated. Their bank usually credits the funds within 1-2 business days.",
			collabName,
		),
		TimeStamp:   time.Now().UnixMilli(),
		IsRead:      false,
		Type:        "collab-closed",
		Data: &trendlymodels.NotificationData{
			CollaborationID: &contract.CollaborationID,
			GroupID:         &contractID,
		},
	}
	_, brandEmails, _ := notifToBrand.Insert(trendlymodels.BRAND_COLLECTION, contract.BrandID)
	if len(brandEmails) > 0 {
		brandEmailData := map[string]interface{}{
			"BrandMemberName": brand.Name,
			"CollabTitle":     collabName,
			"ContractLink":    fmt.Sprintf("%s/contracts/%s", constants.TRENDLY_BRANDS_FE, contractID),
		}
		if err := myemail.SendCustomHTMLEmailToMultipleRecipients(brandEmails, templates.PayoutTransferBrand, templates.SubjectPayoutTransferBrand, brandEmailData); err != nil {
			log.Printf("transfer processed: failed to send brand emails for contract %s: %v", contractID, err)
		}
	}

	// 6. Stream System Message (both parties — transfer processed ≠ instant bank credit)
	streamMessage := "🏁 **Collaboration complete**\n\nTrendly has **processed the payout transfer** for this collaboration. The influencer's bank typically shows the money within **1-2 business days** (timing depends on the bank).\n\nThis chat stays available for reference. Thank you both! ✨"
	_ = streamchat.SendSystemMessage(contract.StreamChannelID, streamMessage)
}
