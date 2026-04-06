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
	} else if event.Event == "transfer.failed" && event.Payload.Transfer != nil {
		handleTransferFailed(&event.Payload.Transfer.Entity)
	}

	c.JSON(http.StatusOK, gin.H{"status": "received"})
}

// loadTrendlyContractForRazorpayTransfer resolves the Trendly contract from a Route transfer's linked order (source).
func loadTrendlyContractForRazorpayTransfer(transfer *webhook.TransferEntity) (*trendlymodels.Contract, string, string, error) {
	if transfer == nil {
		return nil, "", "", fmt.Errorf("nil transfer")
	}
	orderID, err := payments.OrderIDFromTransferSource(transfer.Source)
	if err != nil {
		return nil, "", "", fmt.Errorf("resolve order: %w", err)
	}
	order, err := payments.FetchOrder(orderID)
	if err != nil {
		return nil, "", orderID, fmt.Errorf("fetch order %s: %w", orderID, err)
	}
	contractID := ""
	if order.Notes != nil {
		if v, ok := order.Notes["contractId"].(string); ok {
			contractID = v
		}
	}
	if contractID == "" {
		return nil, "", orderID, fmt.Errorf("no contractId in order %s notes", orderID)
	}
	contract := &trendlymodels.Contract{}
	if err := contract.Get(contractID); err != nil {
		return nil, contractID, orderID, fmt.Errorf("get contract: %w", err)
	}
	return contract, contractID, orderID, nil
}

func transferFailureDetail(transfer *webhook.TransferEntity) string {
	if transfer == nil {
		return "The payout transfer could not be completed."
	}
	if transfer.Error != nil && transfer.Error.Description != nil && *transfer.Error.Description != "" {
		return *transfer.Error.Description
	}
	if transfer.Error != nil && transfer.Error.Reason != nil && *transfer.Error.Reason != "" {
		return *transfer.Error.Reason
	}
	if transfer.Status != "" {
		return fmt.Sprintf("Transfer status from payment partner: %s", transfer.Status)
	}
	return "The payout transfer could not be completed. If this continues, contact Trendly support."
}

func handleTransferProcessed(transfer *webhook.TransferEntity) {
	if transfer == nil {
		return
	}

	transferID := transfer.ID

	contract, contractID, orderID, err := loadTrendlyContractForRazorpayTransfer(transfer)
	if err != nil {
		log.Printf("transfer processed: %v (transfer %s)", err, transferID)
		return
	}

	if contract.Payment != nil && contract.Payment.OrderID != "" && contract.Payment.OrderID != orderID {
		log.Printf("transfer processed: contract %s order mismatch (webhook order %s, contract order %s)", contractID, orderID, contract.Payment.OrderID)
		return
	}
	if contract.Payment != nil && contract.Payment.TransferID != "" && contract.Payment.TransferID != transferID {
		log.Printf("transfer processed: contract %s transfer mismatch (webhook transfer %s, contract transfer %s)", contractID, transferID, contract.Payment.TransferID)
		return
	}

	// 2. Update Contract Status to Settled (10)
	if contract.Payment == nil {
		contract.Payment = &trendlymodels.Payment{}
	}
	contract.Payment.TransferID = transferID
	contract.Payment.Status = "transfer-processed"
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
		TimeStamp: time.Now().UnixMilli(),
		IsRead:    false,
		Type:      "payout-completed",
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
		TimeStamp: time.Now().UnixMilli(),
		IsRead:    false,
		Type:      "collab-closed",
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

func handleTransferFailed(transfer *webhook.TransferEntity) {
	if transfer == nil {
		return
	}

	transferID := transfer.ID

	contract, contractID, orderID, err := loadTrendlyContractForRazorpayTransfer(transfer)
	if err != nil {
		log.Printf("transfer failed: %v (transfer %s)", err, transferID)
		return
	}

	if contract.Payment != nil && contract.Payment.OrderID != "" && contract.Payment.OrderID != orderID {
		log.Printf("transfer failed: contract %s order mismatch (webhook order %s, contract order %s)", contractID, orderID, contract.Payment.OrderID)
		return
	}

	if contract.Status == trendlymodels.ContractStatusSettled {
		log.Printf("transfer failed: skip contract %s already settled (transfer %s)", contractID, transferID)
		return
	}
	if contract.Payment != nil && contract.Payment.TransferID != "" && contract.Payment.TransferID != transferID {
		log.Printf("transfer processed: contract %s transfer mismatch (webhook transfer %s, contract transfer %s)", contractID, transferID, contract.Payment.TransferID)
		return
	}

	if contract.Payment == nil {
		contract.Payment = &trendlymodels.Payment{}
	}
	contract.Payment.TransferID = transferID
	contract.Payment.Status = "transfer-failed"
	// Contract.Status left unchanged so mid-lifecycle contracts are not forced to a late stage; adjust when you add a dedicated payout-failed status.

	err = contract.Update(contractID)
	if err != nil {
		log.Printf("transfer failed: could not update contract %s: %v", contractID, err)
		return
	}

	failureDetail := transferFailureDetail(transfer)

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

	notifToInfluencer := &trendlymodels.Notification{
		Title: "Payout transfer failed",
		Description: fmt.Sprintf(
			"We could not complete the payout transfer for %s. Check your email or open the contract for details.",
			collabName,
		),
		TimeStamp: time.Now().UnixMilli(),
		IsRead:    false,
		Type:      "payout-transfer-failed",
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
			"FailureDetail":  failureDetail,
		}
		if err := myemail.SendCustomHTMLEmail(*influencer.Email, templates.PayoutTransferFailedInfluencer, templates.SubjectPayoutTransferFailedInfluencer, influencerEmailData); err != nil {
			log.Printf("transfer failed: could not send influencer email for contract %s: %v", contractID, err)
		}
	}

	notifToBrand := &trendlymodels.Notification{
		Title: "Influencer payout transfer failed",
		Description: fmt.Sprintf(
			"The payout transfer for %s did not complete. Check your email or open the contract for details.",
			collabName,
		),
		TimeStamp: time.Now().UnixMilli(),
		IsRead:    false,
		Type:      "collab-payout-transfer-failed",
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
			"FailureDetail":   failureDetail,
		}
		if err := myemail.SendCustomHTMLEmailToMultipleRecipients(brandEmails, templates.PayoutTransferFailedBrand, templates.SubjectPayoutTransferFailedBrand, brandEmailData); err != nil {
			log.Printf("transfer failed: could not send brand emails for contract %s: %v", contractID, err)
		}
	}

	streamMessage := fmt.Sprintf(
		"**Payout transfer issue**\n\nThe influencer payout transfer for **%s** could not be completed. Both sides have been notified by email and in-app. Trendly will follow up as needed.\n\nDetails: %s",
		collabName,
		failureDetail,
	)
	_ = streamchat.SendSystemMessage(contract.StreamChannelID, streamMessage)
}
