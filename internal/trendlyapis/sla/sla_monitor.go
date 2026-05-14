package sla

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/idivarts/backend-sls/internal/constants"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/myemail"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
	"github.com/idivarts/backend-sls/templates"
	"google.golang.org/api/iterator"
)

// RunSLAMonitor scans all active contracts and sends nudges or support escalations
// for contracts that have been stuck in a status longer than the SLA threshold.
func RunSLAMonitor() {
	log.Println("[SLA] Starting SLA monitor run")

	// Query contracts in active-but-stuck statuses (3 through 9, excluding 12 disputed)
	activeStatuses := []interface{}{
		int(trendlymodels.ContractStatusShipmentPending),
		int(trendlymodels.ContractStatusShipped),
		int(trendlymodels.ContractStatusDelivered),
		int(trendlymodels.ContractStatusDeliverablePending),
		int(trendlymodels.ContractStatusDeliverableSent),
		int(trendlymodels.ContractStatusPostScheduled),
		int(trendlymodels.ContractStatusPostDone),
	}

	iter := firestoredb.Client.Collection("contracts").
		Where("status", "in", activeStatuses).
		Documents(context.Background())
	defer iter.Stop()

	processed := 0
	nudged := 0
	escalated := 0

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Printf("[SLA] Error iterating contracts: %v", err)
			break
		}

		var contract trendlymodels.Contract
		if err := doc.DataTo(&contract); err != nil {
			log.Printf("[SLA] Failed to parse contract %s: %v", doc.Ref.ID, err)
			continue
		}

		processed++
		n, e := checkAndActOnContract(doc.Ref.ID, &contract)
		nudged += n
		escalated += e
	}

	log.Printf("[SLA] Completed. Processed=%d, Nudged=%d, Escalated=%d", processed, nudged, escalated)
}

// checkAndActOnContract checks SLA thresholds for a single contract and fires notifications.
// Returns (nudgeCount, escalationCount).
func checkAndActOnContract(contractID string, contract *trendlymodels.Contract) (int, int) {
	now := time.Now()
	nudges := 0
	escalations := 0

	// Helper to check if a warning of a given type+level was already sent
	alreadySent := func(warningType, level string) bool {
		for _, w := range contract.SLAWarnings {
			if w.Type == warningType && w.Level == level {
				return true
			}
		}
		return false
	}

	// daysSince returns the number of full days since the given Unix-milli timestamp.
	daysSince := func(unixMilli int64) int {
		if unixMilli == 0 {
			return 0
		}
		return int(now.Sub(time.UnixMilli(unixMilli)).Hours() / 24)
	}

	lastActivityAt := contract.ContractTimestamp.StartedOn
	if len(contract.Activity) > 0 {
		lastActivityAt = contract.Activity[len(contract.Activity)-1].Time
	}
	daysStuck := daysSince(lastActivityAt)

	var warningType string
	var nudgeDays, escalateDays int
	var nudgeParty string // "brand" or "influencer"

	switch contract.Status {
	case trendlymodels.ContractStatusShipmentPending:
		warningType = constants.SLAShipmentOverdue
		nudgeDays = constants.SLAShipmentNudgeDays
		escalateDays = constants.SLAShipmentEscalateDays
		nudgeParty = "brand"
	case trendlymodels.ContractStatusShipped:
		warningType = constants.SLAShipmentInTransitTooLong
		nudgeDays = constants.SLAInTransitEscalateDays // skip nudge, escalate directly
		escalateDays = constants.SLAInTransitEscalateDays
		nudgeParty = "brand"
	case trendlymodels.ContractStatusDelivered:
		warningType = constants.SLADeliveryAckOverdue
		nudgeDays = constants.SLADeliveryAckNudgeDays
		escalateDays = constants.SLADeliveryAckEscalateDays
		nudgeParty = "influencer"
	case trendlymodels.ContractStatusDeliverablePending:
		warningType = constants.SLAVideoOverdue
		nudgeDays = constants.SLAVideoNudgeDays
		escalateDays = constants.SLAVideoEscalateDays
		nudgeParty = "influencer"
	case trendlymodels.ContractStatusDeliverableSent:
		warningType = constants.SLAReviewOverdue
		nudgeDays = constants.SLAReviewNudgeDays
		escalateDays = constants.SLAReviewEscalateDays
		nudgeParty = "brand"
	case trendlymodels.ContractStatusPostScheduled:
		// Only relevant if scheduled date has passed
		if contract.Posting == nil || contract.Posting.ScheduledDate == 0 {
			return 0, 0
		}
		daysOverdue := int(now.Sub(time.UnixMilli(contract.Posting.ScheduledDate)).Hours() / 24)
		if daysOverdue < 0 {
			return 0, 0 // not yet due
		}
		daysStuck = daysOverdue
		warningType = constants.SLAPostingOverdue
		nudgeDays = constants.SLAPostingOverdueDays
		escalateDays = constants.SLAPostingEscalateDays
		nudgeParty = "influencer"
	default:
		return 0, 0
	}

	// Escalate first (higher priority) if threshold exceeded
	if daysStuck >= escalateDays && !alreadySent(warningType, constants.SLALevelSupportEscalation) {
		sendSupportEscalation(contractID, contract, warningType, daysStuck)
		recordSLAWarning(contractID, contract, warningType, constants.SLALevelSupportEscalation)
		escalations++
	} else if daysStuck >= nudgeDays && !alreadySent(warningType, constants.SLALevelNudge) {
		sendNudge(contractID, contract, warningType, nudgeParty, daysStuck)
		recordSLAWarning(contractID, contract, warningType, constants.SLALevelNudge)
		nudges++
	}

	return nudges, escalations
}

func recordSLAWarning(contractID string, contract *trendlymodels.Contract, warningType, level string) {
	contract.SLAWarnings = append(contract.SLAWarnings, trendlymodels.SLAWarning{
		Type:   warningType,
		Level:  level,
		SentAt: time.Now().UnixMilli(),
	})
	if err := contract.Update(contractID); err != nil {
		log.Printf("[SLA] Failed to record SLA warning for contract %s: %v", contractID, err)
	}
}

func sendNudge(contractID string, contract *trendlymodels.Contract, warningType, nudgeParty string, daysStuck int) {
	brand := &trendlymodels.Brand{}
	_ = brand.Get(contract.BrandID)

	influencer := &trendlymodels.User{}
	_ = influencer.Get(contract.UserID)

	collab := &trendlymodels.Collaboration{}
	_ = collab.Get(contract.CollaborationID)
	collabName := collab.Name
	if collabName == "" {
		collabName = "Your Collaboration"
	}

	var templatePath myemail.TemplatePath
	var subject string
	var recipient string
	var emailData map[string]interface{}

	switch warningType {
	case constants.SLAShipmentOverdue:
		templatePath = templates.SLANudgeBrandShip
		subject = templates.SubjectSLANudgeBrandShip
		emailData = map[string]interface{}{
			"BrandName":    brand.Name,
			"CollabTitle":  collabName,
			"DaysStuck":    daysStuck,
			"ContractLink": fmt.Sprintf("https://brands.trendly.now/contract-details/%s", contractID),
		}
	case constants.SLADeliveryAckOverdue:
		templatePath = templates.SLANudgeInfluencerReceipt
		subject = templates.SubjectSLANudgeInfluencerReceipt
		if influencer.Email != nil {
			recipient = *influencer.Email
		}
		emailData = map[string]interface{}{
			"InfluencerName": influencer.Name,
			"BrandName":      brand.Name,
			"CollabTitle":    collabName,
			"DaysStuck":      daysStuck,
			"ContractLink":   fmt.Sprintf("https://creators.trendly.now/contract-details/%s", contractID),
		}
	case constants.SLAVideoOverdue:
		templatePath = templates.SLANudgeInfluencerVideo
		subject = templates.SubjectSLANudgeInfluencerVideo
		if influencer.Email != nil {
			recipient = *influencer.Email
		}
		emailData = map[string]interface{}{
			"InfluencerName": influencer.Name,
			"BrandName":      brand.Name,
			"CollabTitle":    collabName,
			"DaysStuck":      daysStuck,
			"ContractLink":   fmt.Sprintf("https://creators.trendly.now/contract-details/%s", contractID),
		}
	case constants.SLAReviewOverdue:
		templatePath = templates.SLANudgeBrandReview
		subject = templates.SubjectSLANudgeBrandReview
		emailData = map[string]interface{}{
			"BrandName":      brand.Name,
			"InfluencerName": influencer.Name,
			"CollabTitle":    collabName,
			"DaysStuck":      daysStuck,
			"ContractLink":   fmt.Sprintf("https://brands.trendly.now/contract-details/%s", contractID),
		}
	case constants.SLAPostingOverdue:
		templatePath = templates.SLANudgeInfluencerPost
		subject = templates.SubjectSLANudgeInfluencerPost
		if influencer.Email != nil {
			recipient = *influencer.Email
		}
		emailData = map[string]interface{}{
			"InfluencerName": influencer.Name,
			"BrandName":      brand.Name,
			"CollabTitle":    collabName,
			"DaysStuck":      daysStuck,
			"ContractLink":   fmt.Sprintf("https://creators.trendly.now/contract-details/%s", contractID),
		}
	default:
		return
	}

	if recipient == "" && nudgeParty == "brand" {
		// Try to get brand member emails via notification
		notif := &trendlymodels.Notification{
			Title:       "Contract needs your attention",
			Description: fmt.Sprintf("Your contract for %s has been waiting for %d days.", collabName, daysStuck),
			TimeStamp:   time.Now().UnixMilli(),
			IsRead:      false,
			Type:        "sla-nudge",
			Data: &trendlymodels.NotificationData{
				CollaborationID: &contract.CollaborationID,
				GroupID:         &contractID,
			},
		}
		_, brandEmails, _ := notif.Insert(trendlymodels.BRAND_COLLECTION, contract.BrandID)
		if len(brandEmails) > 0 && templatePath != "" {
			_ = myemail.SendCustomHTMLEmailToMultipleRecipients(brandEmails, templatePath, subject, emailData)
		}
		return
	}

	if recipient != "" && templatePath != "" {
		_ = myemail.SendCustomHTMLEmail(recipient, templatePath, subject, emailData)
		// Also send in-app notification to influencer
		notif := &trendlymodels.Notification{
			Title:       "Reminder: Action needed on your contract",
			Description: fmt.Sprintf("Your contract for %s has been waiting for %d days. Please take action.", collabName, daysStuck),
			TimeStamp:   time.Now().UnixMilli(),
			IsRead:      false,
			Type:        "sla-nudge",
			Data: &trendlymodels.NotificationData{
				CollaborationID: &contract.CollaborationID,
				GroupID:         &contractID,
			},
		}
		_, _, _ = notif.Insert(trendlymodels.USER_COLLECTION, contract.UserID)
	}

	log.Printf("[SLA] Nudge sent for contract %s (type: %s, days: %d)", contractID, warningType, daysStuck)
}

func sendSupportEscalation(contractID string, contract *trendlymodels.Contract, warningType string, daysStuck int) {
	brand := &trendlymodels.Brand{}
	_ = brand.Get(contract.BrandID)

	influencer := &trendlymodels.User{}
	_ = influencer.Get(contract.UserID)

	collab := &trendlymodels.Collaboration{}
	_ = collab.Get(contract.CollaborationID)
	collabName := collab.Name
	if collabName == "" {
		collabName = "Your Collaboration"
	}

	emailData := map[string]interface{}{
		"ContractID":     contractID,
		"BrandName":      brand.Name,
		"InfluencerName": influencer.Name,
		"CollabTitle":    collabName,
		"Status":         int(contract.Status),
		"WarningType":    warningType,
		"DaysStuck":      daysStuck,
		"ContractLink":   fmt.Sprintf("https://brands.trendly.now/contract-details/%s", contractID),
	}

	if err := myemail.SendCustomHTMLEmail("support@trendly.now", templates.SLAEscalationSupport, templates.SubjectSLAEscalationSupport, emailData); err != nil {
		log.Printf("[SLA] Failed to send escalation email for contract %s: %v", contractID, err)
		return
	}

	log.Printf("[SLA] Escalation sent for contract %s (type: %s, days: %d)", contractID, warningType, daysStuck)
}
