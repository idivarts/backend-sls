package webhook

import (
	"encoding/json"
	"errors"

	"github.com/idivarts/backend-sls/pkg/payments"
)

// ErrInvalidSignature is returned when the X-Razorpay-Signature does not match the body.
var ErrInvalidSignature = errors.New("webhook: invalid signature")

// VerifyAndParse verifies the webhook HMAC (same algorithm as payments.VerifyWebhookSignature)
// and unmarshals the body into an Event.
func VerifyAndParse(body []byte, signature string, secret string) (*Event, error) {
	if !payments.VerifyWebhookSignature(body, signature, secret) {
		return nil, ErrInvalidSignature
	}
	var ev Event
	if err := json.Unmarshal(body, &ev); err != nil {
		return nil, err
	}
	return &ev, nil
}

// Parse unmarshals the raw JSON body without verification (e.g. tests or pre-verified payloads).
func Parse(body []byte) (*Event, error) {
	var ev Event
	if err := json.Unmarshal(body, &ev); err != nil {
		return nil, err
	}
	return &ev, nil
}
