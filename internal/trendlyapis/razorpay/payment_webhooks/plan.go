package paymentwebhooks

type PlanEntity struct {
	ID        string    `json:"id"`
	Entity    string    `json:"entity"`
	Interval  int       `json:"interval"`
	Period    string    `json:"period"`
	Item      item      `json:"item"`
	Notes     PlanNotes `json:"notes"`
	CreatedAt int64     `json:"created_at"`
}

type item struct {
	ID           string  `json:"id"`
	Active       bool    `json:"active"`
	Name         string  `json:"name"`
	Description  string  `json:"description"`
	Amount       int     `json:"amount"`
	UnitAmount   int     `json:"unit_amount"`
	Currency     string  `json:"currency"`
	Type         string  `json:"type"`
	Unit         *string `json:"unit"`
	TaxInclusive bool    `json:"tax_inclusive"`
	HsnCode      *string `json:"hsn_code"`
	SacCode      *string `json:"sac_code"`
	TaxRate      *string `json:"tax_rate"`
	TaxID        *string `json:"tax_id"`
	TaxGroupID   *string `json:"tax_group_id"`
	CreatedAt    int64   `json:"created_at"`
	UpdatedAt    int64   `json:"updated_at"`
}

type PlanNotes struct {
	PlanKey     string `json:"planKey"`
	PlanCycle   string `json:"planCycle"`
	PlanVersion string `json:"planVersion"`
}
