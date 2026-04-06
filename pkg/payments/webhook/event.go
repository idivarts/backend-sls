package webhook

// Event is the Razorpay webhook envelope. Payload fields vary by event type;
// unknown JSON keys are ignored during unmarshal.
type Event struct {
	Entity    string   `json:"entity"`
	AccountID string   `json:"account_id"`
	Event     string   `json:"event"`
	Contains  []string `json:"contains"`
	Payload   Payload  `json:"payload"`
	CreatedAt int64    `json:"created_at"`
}

// Payload holds optional nested resources as sent by Razorpay.
type Payload struct {
	Subscription *struct {
		Entity SubscriptionEntity `json:"entity"`
	} `json:"subscription"`
	Order *struct {
		Entity OrderEntity `json:"entity"`
	} `json:"order"`
	Payment *struct {
		Entity PaymentEntity `json:"entity"`
	} `json:"payment"`
	PaymentLink *struct {
		Entity PaymentLinkEntity `json:"entity"`
	} `json:"payment_link"`
	Transfer *struct {
		Entity TransferEntity `json:"entity"`
	} `json:"transfer"`
	Settlement *struct {
		Entity SettlementEntity `json:"entity"`
	} `json:"settlement"`
	Route *struct {
		Entity RouteProductEntity     `json:"entity"`
		Data   map[string]interface{} `json:"data"`
	} `json:"merchant_product"`
}
