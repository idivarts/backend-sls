package webhook

type SubscriptionNotes struct {
	BrandID     string `json:"brandId"`
	PlanKey     string `json:"planKey"`
	PlanCycle   string `json:"planCycle"`
	PlanVersion string `json:"planVersion"`
}

type SubscriptionEntity struct {
	ID                  string              `json:"id"`
	Entity              string              `json:"entity"`
	PlanID              string              `json:"plan_id"`
	CustomerID          *string             `json:"customer_id"`
	Status              string              `json:"status"`
	CurrentStart        *int64              `json:"current_start"`
	CurrentEnd          *int64              `json:"current_end"`
	EndedAt             *int64              `json:"ended_at"`
	Quantity            int                 `json:"quantity"`
	Notes               SubscriptionNotes   `json:"notes"`
	ChargeAt            int64               `json:"charge_at"`
	StartAt             int64               `json:"start_at"`
	EndAt               int64               `json:"end_at"`
	AuthAttempts        int                 `json:"auth_attempts"`
	TotalCount          int                 `json:"total_count"`
	PaidCount           int                 `json:"paid_count"`
	CustomerNotify      bool                `json:"customer_notify"`
	CreatedAt           int64               `json:"created_at"`
	ExpireBy            int64               `json:"expire_by"`
	ShortURL            *string             `json:"short_url"`
	HasScheduledChanges bool                `json:"has_scheduled_changes"`
	ChangeScheduledAt   *int64              `json:"change_scheduled_at"`
	Source              string              `json:"source"`
	OfferID             *string             `json:"offer_id"`
	RemainingCount      int                 `json:"remaining_count"`
}
