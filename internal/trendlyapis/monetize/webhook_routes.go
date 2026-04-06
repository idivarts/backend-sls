package monetize

import (
	"errors"
	"io"
	"log"
	"net/http"

	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/payments"
	"github.com/idivarts/backend-sls/pkg/payments/webhook"
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

	kycStatus := strings.TrimSpace(entity.ActivationStatus)
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
}

func kycStatusFromRouteProductEvent(eventName string) string {
	switch eventName {
	case "product.route.under_review":
		return "under_review"
	case "product.route.needs_clarification":
		return "needs_clarification"
	case "product.route.activated":
		return "activated"
	case "product.route.rejected":
		return "rejected"
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
			"We need a bit more information to finish verifying your payout details. Open Monetize / account settings to review and update what’s required."
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

	return ref, nil
}

func applyRouteProductKYCUpdate(userID, merchantID, kycStatus string, reason *string) error {
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
	if strings.EqualFold(kycStatus, "activated") {
		user.KYC.Reason = nil
	} else if reason != nil {
		user.KYC.Reason = reason
	}
	ts := time.Now().UnixMilli()
	user.KYC.UpdatedAt = &ts

	user.IsKYCDone = strings.EqualFold(kycStatus, "activated")

	_, err := user.Insert(userID)
	return err
}
