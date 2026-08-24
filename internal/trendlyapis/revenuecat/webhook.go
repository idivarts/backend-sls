package revenuecat

import (
	"crypto/subtle"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/myutil"
)

const providerName = "revenuecat"

// rcWebhook is the RevenueCat v1 webhook envelope. We model only the fields we
// consume; RevenueCat sends many more.
type rcWebhook struct {
	APIVersion string  `json:"api_version"`
	Event      rcEvent `json:"event"`
}

type rcEvent struct {
	ID                    string   `json:"id"`
	Type                  string   `json:"type"`
	AppUserID             string   `json:"app_user_id"`
	OriginalAppUserID     string   `json:"original_app_user_id"`
	ProductID             string   `json:"product_id"`
	EntitlementIDs        []string `json:"entitlement_ids"`
	PeriodType            string   `json:"period_type"`
	Store                 string   `json:"store"`
	Environment           string   `json:"environment"`
	PurchasedAtMs         int64    `json:"purchased_at_ms"`
	ExpirationAtMs        int64    `json:"expiration_at_ms"`
	TransactionID         string   `json:"transaction_id"`
	OriginalTransactionID string   `json:"original_transaction_id"`
	CancelReason          string   `json:"cancel_reason"`
	// TRANSFER events carry the app_user_ids the entitlement moved between.
	TransferredFrom []string `json:"transferred_from"`
	TransferredTo   []string `json:"transferred_to"`
}

// Handler is the RevenueCat webhook endpoint (ANY /revenuecat/webhook). It
// authenticates via the shared Authorization header configured in the
// RevenueCat dashboard (env REVENUECAT_WEBHOOK_AUTH), then applies the event to
// the org billing engine. It always responds 200 for events it has consciously
// no-op'd (unknown org/plan, ignored event type) so RevenueCat does not retry
// forever; it responds 5xx only on transient failures so RevenueCat retries.
func Handler(c *gin.Context) {
	expected := os.Getenv("REVENUECAT_WEBHOOK_AUTH")
	if expected == "" {
		// Fail closed: without a configured secret we cannot trust the caller.
		log.Println("revenuecat webhook: REVENUECAT_WEBHOOK_AUTH not configured")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "webhook secret not configured"})
		return
	}
	got := c.GetHeader("Authorization")
	if subtle.ConstantTimeCompare([]byte(got), []byte(expected)) != 1 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization"})
		return
	}

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read body"})
		return
	}
	log.Println("revenuecat webhook received", string(body))

	var payload rcWebhook
	if err := json.Unmarshal(body, &payload); err != nil {
		// Malformed body — ack so RC does not retry an unparseable event.
		c.JSON(http.StatusOK, gin.H{"message": "acknowledged (unparseable)", "error": err.Error()})
		return
	}

	if err := process(&payload.Event); err != nil {
		// Transient failure → 5xx so RevenueCat retries delivery.
		log.Println("revenuecat webhook: process failed", payload.Event.Type, payload.Event.ID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// process dispatches a single RevenueCat event to the org billing engine. A nil
// return means "handled or consciously ignored" (ack); a non-nil return means
// "transient failure, please retry".
func process(e *rcEvent) error {
	// Idempotency: a retried delivery of the same event id must not re-apply
	// side effects (critical for additive token top-ups).
	isNew, err := trendlymodels.MarkWebhookEventProcessed(providerName, e.ID)
	if err != nil {
		return err // transient — let RC retry
	}
	if !isNew {
		log.Println("revenuecat webhook: duplicate event ignored", e.Type, e.ID)
		return nil
	}

	switch e.Type {
	case "INITIAL_PURCHASE", "RENEWAL", "PRODUCT_CHANGE", "UNCANCELLATION":
		return applyActiveSubscription(e)

	case "NON_RENEWING_PURCHASE":
		return applyTopup(e)

	case "BILLING_ISSUE":
		return setAccessState(e.AppUserID, "past_due")

	case "EXPIRATION":
		// Subscription lapsed (auto-renew off + period ended, or refund/billing
		// failure resolved to expiry). Revoke access; the paywall/lock reads this.
		return setAccessState(e.AppUserID, "canceled")

	case "CANCELLATION":
		// Auto-renew disabled — the user keeps access until EXPIRATION. Leave the
		// access state untouched; do not lock early.
		log.Println("revenuecat webhook: cancellation (access retained until expiry)", e.AppUserID, e.CancelReason)
		return nil

	case "TRANSFER":
		// The entitlement moved to a different app_user_id (org). Revoke the orgs
		// it moved away from; the destination org gets its own purchase/renewal
		// events which reactivate it.
		for _, from := range e.TransferredFrom {
			if from == "" {
				continue
			}
			if err := setAccessState(from, "canceled"); err != nil {
				return err
			}
		}
		log.Println("revenuecat webhook: transfer", e.TransferredFrom, "->", e.TransferredTo)
		return nil

	default:
		log.Println("revenuecat webhook: ignored event type", e.Type)
		return nil
	}
}

// applyActiveSubscription funds the org and flips it to active for a subscription
// that is currently in good standing. Mirrors the Razorpay "active" path but
// keyed off the IAP expiration for the wallet reset + period end (IAP renews on
// its own anniversary, not the 1st-of-month anchor).
func applyActiveSubscription(e *rcEvent) error {
	orgID := e.AppUserID
	planKey := PlanKeyForProduct(e.ProductID, e.EntitlementIDs)
	if orgID == "" || planKey == "" {
		log.Println("revenuecat webhook: unknown org/plan, ignoring", orgID, e.ProductID, e.EntitlementIDs)
		return nil // ack — nothing actionable
	}

	org := &trendlymodels.Organization{}
	if err := org.Get(orgID); err != nil {
		log.Println("revenuecat webhook: org not found, ignoring", orgID, err)
		return nil // ack — not our org (or deleted)
	}

	reset := e.ExpirationAtMs
	if reset <= 0 {
		reset = trendlymodels.NextMonthlyReset(time.Now())
	}

	billing := org.Billing
	if billing == nil {
		billing = &trendlymodels.BrandBilling{}
	}
	billing.Provider = myutil.StrPtr(providerName)
	billing.AccessState = myutil.StrPtr("active")
	billing.BillingMode = myutil.StrPtr("recurring")
	billing.BillingStatus = myutil.StrPtr("active")
	billing.PlanKey = &planKey
	billing.PeriodEnd = &reset
	if s := storeLabel(e.Store); s != "" {
		billing.Store = &s
	}
	if e.OriginalTransactionID != "" {
		billing.ProviderRef = &e.OriginalTransactionID
	}

	if err := org.SetBilling(orgID, billing); err != nil {
		return err
	}
	// Fund the wallet + refresh entitlements via the shared engine.
	return trendlymodels.ApplyPlanToOrg(orgID, planKey, reset)
}

// applyTopup credits a token pack purchased as an in-app consumable.
func applyTopup(e *rcEvent) error {
	orgID := e.AppUserID
	tokens := TopupTokensForProduct(e.ProductID)
	if orgID == "" || tokens <= 0 {
		log.Println("revenuecat webhook: unknown top-up product, ignoring", orgID, e.ProductID)
		return nil
	}
	return trendlymodels.AddTopup(orgID, tokens)
}

// setAccessState updates just the billing access state for an org, no-op'ing
// (ack) when the org is missing so RC stops retrying.
func setAccessState(orgID, state string) error {
	if orgID == "" {
		return nil
	}
	org := &trendlymodels.Organization{}
	if err := org.Get(orgID); err != nil {
		log.Println("revenuecat webhook: org not found for state change, ignoring", orgID, state, err)
		return nil
	}
	return trendlymodels.SetOrgAccessState(orgID, state)
}
