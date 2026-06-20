package monetize

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/constants"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
	"github.com/idivarts/backend-sls/pkg/myemail"
	"github.com/idivarts/backend-sls/pkg/payments"
	"github.com/idivarts/backend-sls/pkg/payments/webhook"
	"github.com/idivarts/backend-sls/templates"
	"google.golang.org/api/iterator"
)

func RouteWebhook(c *gin.Context) {
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

	log.Printf("Received Razorpay Route Event: %s", event.Event)

	switch event.Event {
	case "product.route.under_review", "product.route.activated", "product.route.needs_clarification", "product.route.rejected":
		handleRouteProductWebhook(event)
	}

	c.JSON(http.StatusOK, gin.H{"status": "received"})
}

func handleRouteProductWebhook(event *webhook.Event) {
	if event == nil || event.Payload.Route == nil {
		return
	}

	entity := &event.Payload.Route.Entity
	merchantID := strings.TrimSpace(entity.MerchantID)
	if merchantID == "" {
		log.Printf("route product webhook %s: missing merchant_id", event.Event)
		return
	}

	if pn := strings.TrimSpace(entity.ProductName); pn != "" && pn != "route" {
		log.Printf("route product webhook %s: skip product_name=%q", event.Event, pn)
		return
	}

	userID, err := resolveInfluencerUserIDForRouteMerchant(context.Background(), merchantID)
	if err != nil {
		log.Printf("route product webhook %s: resolve user for merchant %s: %v", event.Event, merchantID, err)
		return
	}

	kycStatus := trendlymodels.KYCStatus(strings.TrimSpace(entity.ActivationStatus))
	if kycStatus == "" {
		kycStatus = kycStatusFromRouteProductEvent(event.Event)
	}
	if kycStatus == "" {
		log.Printf("route product webhook %s: could not determine KYC status", event.Event)
		return
	}

	reason := routeProductReasonFromData(event.Payload.Route.Data)
	if err := applyRouteProductKYCUpdate(userID, merchantID, kycStatus, reason); err != nil {
		log.Printf("route product webhook %s: update user %s KYC: %v", event.Event, userID, err)
		return
	}

	title, desc := routeProductInAppCopy(event.Event)
	if title == "" {
		return
	}

	ts := time.Now().UnixMilli()
	notif := &trendlymodels.Notification{
		Title:       title,
		Description: desc,
		TimeStamp:   ts,
		IsRead:      false,
		Type:        routeProductNotificationType(event.Event),
	}
	if _, _, err := notif.Insert(trendlymodels.USER_COLLECTION, userID); err != nil {
		log.Printf("route product webhook %s: notification for user %s: %v", event.Event, userID, err)
	}

	sendRouteProductKYCEmailIfEligible(userID, kycStatus, reason)
}

func kycStatusFromRouteProductEvent(eventName string) trendlymodels.KYCStatus {
	switch eventName {
	case "product.route.under_review":
		return trendlymodels.KYCStatusUnderReview
	case "product.route.needs_clarification":
		return trendlymodels.KYCStatusNeedsClarification
	case "product.route.activated":
		return trendlymodels.KYCStatusActivated
	case "product.route.rejected":
		return trendlymodels.KYCStatusRejected
	default:
		return ""
	}
}

func routeProductNotificationType(eventName string) string {
	switch eventName {
	case "product.route.under_review":
		return "kyc-route-under-review"
	case "product.route.needs_clarification":
		return "kyc-route-needs-clarification"
	case "product.route.activated":
		return "kyc-route-activated"
	case "product.route.rejected":
		return "kyc-route-rejected"
	default:
		return "kyc-route-update"
	}
}

func routeProductInAppCopy(eventName string) (title, description string) {
	switch eventName {
	case "product.route.under_review":
		return "Payout setup under review",
			"We’re reviewing your bank and identity details for payouts. We’ll update you here when something changes."
	case "product.route.needs_clarification":
		return "Action needed for payout setup",
			"We need a bit more information to finish verifying your payout details. Open the Monetize section in your account settings to review what’s needed."
	case "product.route.activated":
		return "Payout setup approved",
			"Your payout account is verified. You’re all set to receive payments on Trendly."
	case "product.route.rejected":
		return "Payout setup couldn’t be approved",
			"We weren’t able to approve your payout verification. Check your details in account settings or contact support if you need help."
	default:
		return "", ""
	}
}

func routeProductReasonFromData(data map[string]interface{}) *string {
	if len(data) == 0 {
		return nil
	}
	keys := []string{"reason", "description", "message", "clarification_reason", "notes"}
	for _, k := range keys {
		if v, ok := data[k]; ok {
			if s, ok := v.(string); ok {
				s = strings.TrimSpace(s)
				if s != "" {
					return &s
				}
			}
		}
	}
	return nil
}

func resolveInfluencerUserIDForRouteMerchant(ctx context.Context, merchantID string) (string, error) {
	account, err := payments.FetchLinkedAccount(merchantID)
	if err != nil {
		return "", fmt.Errorf("fetch linked account: %w", err)
	}

	ref := strings.TrimSpace(account.ReferenceID)
	if ref != "" {
		if uid, err := userIDFromFirestoreByReferenceID(ctx, ref); err == nil && uid != "" {
			return uid, nil
		}
	}

	return userIDFromFirestoreByKYCAccountID(ctx, merchantID)
}

func userIDFromFirestoreByKYCAccountID(ctx context.Context, accountID string) (string, error) {
	iter := firestoredb.Client.Collection("users").Where("kyc.accountId", "==", accountID).Limit(2).Documents(ctx)
	defer iter.Stop()

	var ids []string
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return "", err
		}
		ids = append(ids, doc.Ref.ID)
	}
	if len(ids) == 0 {
		return "", fmt.Errorf("no user with kyc.accountId=%s", accountID)
	}
	if len(ids) > 1 {
		return "", fmt.Errorf("multiple users with kyc.accountId=%s", accountID)
	}
	return ids[0], nil
}

func userIDFromFirestoreByReferenceID(ctx context.Context, ref string) (string, error) {
	snap, err := firestoredb.Client.Collection("users").Doc(ref).Get(ctx)
	if err == nil && snap.Exists() {
		return ref, nil
	}

	iter := firestoredb.Client.Collection("users").
		OrderBy(firestore.DocumentID, firestore.Asc).
		StartAt(ref).
		EndAt(ref + "\uf8ff").
		Limit(2).
		Documents(ctx)
	defer iter.Stop()

	var ids []string
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return "", err
		}
		ids = append(ids, doc.Ref.ID)
	}
	if len(ids) == 0 {
		return "", fmt.Errorf("no user doc for reference_id prefix %q", ref)
	}
	if len(ids) > 1 {
		return "", fmt.Errorf("ambiguous user docs for reference_id prefix %q: %v", ref, ids)
	}
	return ids[0], nil
}

func sendRouteProductKYCEmailIfEligible(userID string, kycStatus trendlymodels.KYCStatus, webhookReason *string) {
	if !strings.EqualFold(string(kycStatus), string(trendlymodels.KYCStatusActivated)) &&
		!strings.EqualFold(string(kycStatus), string(trendlymodels.KYCStatusRejected)) {
		return
	}

	user := &trendlymodels.User{}
	if err := user.Get(userID); err != nil {
		log.Printf("route product KYC email: get user %s: %v", userID, err)
		return
	}
	if user.Email == nil || strings.TrimSpace(*user.Email) == "" {
		return
	}

	fe := constants.GetCreatorsFronted()
	data := map[string]interface{}{
		"InfluencerName": user.Name,
		"TrendlyAppLink": fe,
	}

	var sendErr error
	switch {
	case strings.EqualFold(string(kycStatus), string(trendlymodels.KYCStatusActivated)):
		sendErr = myemail.SendCustomHTMLEmail(*user.Email, templates.KYCRouteActivatedInfluencer, templates.SubjectKYCRouteActivatedInfluencer, data)
	case strings.EqualFold(string(kycStatus), string(trendlymodels.KYCStatusRejected)):
		rej := ""
		if webhookReason != nil {
			rej = strings.TrimSpace(*webhookReason)
		}
		if rej == "" && user.KYC != nil && user.KYC.Reason != nil {
			rej = strings.TrimSpace(*user.KYC.Reason)
		}
		data["RejectionReason"] = rej
		sendErr = myemail.SendCustomHTMLEmail(*user.Email, templates.KYCRouteRejectedInfluencer, templates.SubjectKYCRouteRejectedInfluencer, data)
	}

	if sendErr != nil {
		log.Printf("route product KYC email (%s) for user %s: %v", kycStatus, userID, sendErr)
	}
}

func applyRouteProductKYCUpdate(userID, merchantID string, kycStatus trendlymodels.KYCStatus, reason *string) error {
	user := &trendlymodels.User{}
	if err := user.Get(userID); err != nil {
		return err
	}

	if user.KYC == nil {
		user.KYC = &trendlymodels.KYC{}
	}
	if user.KYC.AccountID == "" {
		user.KYC.AccountID = merchantID
	}

	user.KYC.Status = kycStatus
	if strings.EqualFold(string(kycStatus), string(trendlymodels.KYCStatusActivated)) {
		user.KYC.Reason = nil
	} else if reason != nil {
		user.KYC.Reason = reason
	}
	ts := time.Now().UnixMilli()
	user.KYC.UpdatedAt = &ts

	user.IsKYCDone = strings.EqualFold(string(kycStatus), string(trendlymodels.KYCStatusActivated))

	_, err := user.Insert(userID)
	return err
}
