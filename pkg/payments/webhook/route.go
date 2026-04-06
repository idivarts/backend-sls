package webhook

// RouteEntity matches the transfer resource embedded in Route webhook payloads
// (e.g. transfer.processed, transfer.failed). See:
// https://razorpay.com/docs/webhooks/route/
type RouteEntity struct {
	ID               string `json:"id"`
	MerchantID       string `json:"merchant_id"`
	ActivationStatus string `json:"activation_status"`
}
