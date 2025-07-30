package paymentwebhooks

type OrderEntity struct {
	AccountNumber         *string                `json:"account_number"`
	Amount                int64                  `json:"amount"`
	AmountDue             int64                  `json:"amount_due"`
	AmountPaid            int64                  `json:"amount_paid"`
	AppOffer              bool                   `json:"app_offer"`
	Attempts              int                    `json:"attempts"`
	Authorized            bool                   `json:"authorized"`
	Bank                  *string                `json:"bank"`
	BankAccount           *string                `json:"bank_account"`
	CheckoutConfigID      *string                `json:"checkout_config_id"`
	CreatedAt             int64                  `json:"created_at"`
	Currency              string                 `json:"currency"`
	CustomerID            *string                `json:"customer_id"`
	Discount              bool                   `json:"discount"`
	FirstPaymentMinAmount *int64                 `json:"first_payment_min_amount"`
	ForceOffer            *bool                  `json:"force_offer"`
	ID                    string                 `json:"id"`
	LateAuthConfigID      *string                `json:"late_auth_config_id"`
	MerchantID            string                 `json:"merchant_id"`
	Method                *string                `json:"method"`
	Notes                 map[string]string      `json:"notes"`
	Offers                map[string]interface{} `json:"offers"`
	OrderMetas            []interface{}          `json:"order_metas"`
	OrderRelationships    []interface{}          `json:"order_relationships"`
	PartialPayment        bool                   `json:"partial_payment"`
	PayerName             *string                `json:"payer_name"`
	PaymentCapture        bool                   `json:"payment_capture"`
	ProductID             string                 `json:"product_id"`
	ProductType           string                 `json:"product_type"`
	ProviderContext       *string                `json:"provider_context"`
	PublicKey             string                 `json:"public_key"`
	PublicResponse        *string                `json:"public_response"`
	Receipt               string                 `json:"receipt"`
	Reference2            *string                `json:"reference2"`
	Reference3            *string                `json:"reference3"`
	Reference4            *string                `json:"reference4"`
	Reference5            *string                `json:"reference5"`
	Reference6            *string                `json:"reference6"`
	Reference7            *string                `json:"reference7"`
	Reference8            *string                `json:"reference8"`
	Source                *string                `json:"source"`
	Status                string                 `json:"status"`
	Transfers             *interface{}           `json:"transfers"`
	UpdatedAt             int64                  `json:"updated_at"`
}
