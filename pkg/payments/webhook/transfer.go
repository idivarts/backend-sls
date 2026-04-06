package webhook

// TransferEntity is the transfer object embedded in transfer.* webhook payloads.
type TransferEntity struct {
	ID                    string                 `json:"id"`
	Entity                string                 `json:"entity"`
	Status                string                 `json:"status"`
	SettlementStatus      *string                `json:"settlement_status"`
	Source                string                 `json:"source"`
	Recipient             string                 `json:"recipient"`
	Amount                int64                  `json:"amount"`
	Currency              string                 `json:"currency"`
	AmountReversed        int64                  `json:"amount_reversed"`
	Notes                 map[string]interface{} `json:"notes"`
	Fees                  int64                  `json:"fees"`
	Tax                   int64                  `json:"tax"`
	OnHold                bool                   `json:"on_hold"`
	OnHoldUntil           *int64                 `json:"on_hold_until"`
	RecipientSettlementID *string                `json:"recipient_settlement_id"`
	CreatedAt             int64                  `json:"created_at"`
	LinkedAccountNotes    []string               `json:"linked_account_notes"`
	ProcessedAt           *int64                 `json:"processed_at"`
	Error                 *TransferEntity        `json:"error"`
}

// SettlementEntity represents a Razorpay settlement resource in "transfer.*" or "settlement.*" webhook payloads.
type SettlementEntity struct {
	ID        string `json:"id"`
	Entity    string `json:"entity"`
	Amount    int64  `json:"amount"`
	Status    string `json:"status"`
	Fees      int64  `json:"fees"`
	Tax       int64  `json:"tax"`
	UTR       string `json:"utr"`
	CreatedAt int64  `json:"created_at"`
}

// TransferEntityError is the nested error object on a Route transfer entity.
type TransferEntityError struct {
	Code        *string `json:"code"`
	Description *string `json:"description"`
	Field       *string `json:"field"`
	Source      *string `json:"source"`
	Step        *string `json:"step"`
	Reason      *string `json:"reason"`
}
