package webhook

// RouteProductEntity matches merchant_product in Route webhooks (product.route.*).
// See: https://razorpay.com/docs/webhooks/payloads/route/
type RouteProductEntity struct {
	Entity           string `json:"entity"`
	ID               string `json:"id"`
	MerchantID       string `json:"merchant_id"`
	ProductName      string `json:"product_name"`
	ActivationStatus string `json:"activation_status"`
}
