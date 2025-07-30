package paymentwebhooks_test

import (
	"encoding/json"
	"testing"

	paymentwebhooks "github.com/idivarts/backend-sls/internal/trendlyapis/razorpay/payment_webhooks"
)

const data = `{
  "account_id": "acc_PhlJddXEtzLSDX",
  "contains": [
    "subscription"
  ],
  "created_at": 1753913474,
  "entity": "event",
  "event": "subscription.authenticated",
  "payload": {
    "subscription": {
      "entity": {
        "auth_attempts": 0,
        "change_scheduled_at": null,
        "charge_at": 1754172594,
        "created_at": 1753913395,
        "current_end": null,
        "current_start": null,
        "customer_id": null,
        "customer_notify": true,
        "end_at": 1783017000,
        "ended_at": null,
        "entity": "subscription",
        "expire_by": 1753999794,
        "has_scheduled_changes": false,
        "id": "sub_QzRG6xEMalfmcB",
        "notes": {
          "brandId": "F1wkGrD6qEoTYBKDF2DG",
          "isGrowthPlan": "1",
          "planName": "Growth Plan"
        },
        "offer_id": null,
        "paid_count": 0,
        "payment_method": "upi",
        "plan_id": "plan_QzPMAkZjXYbJV8",
        "quantity": 1,
        "remaining_count": 12,
        "short_url": null,
        "source": "api",
        "start_at": 1754172594,
        "status": "authenticated",
        "total_count": 12
      }
    }
  }
}`

func TestParse(t *testing.T) {
	var event paymentwebhooks.RazorpayWebhookEvent
	bodyBytes := []byte(data)
	if err := json.Unmarshal(bodyBytes, &event); err != nil {
		t.Error(err)
		return
	}
	err := paymentwebhooks.HandleSubscription(event)
	if err != nil {
		t.Error(err)
	}
	t.Log("Success")
}
