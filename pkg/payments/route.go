package payments

import (
	"encoding/json"
)

type CreateAccountReq struct {
	Name    string     `json:"name"`
	Email   string     `json:"email"`
	Phone   string     `json:"phone"`
	UserId  string     `json:"user_id"`
	Address AddressReq `json:"address"`
	PAN     string     `json:"pan"`
}

type AddressReq struct {
	Street     string `json:"street"`
	City       string `json:"city"`
	State      string `json:"state"`
	PostalCode string `json:"postal_code"`
}

type BankReq struct {
	AccountNumber   string `json:"account_number"`
	IFSC            string `json:"ifsc"`
	BenificiaryName string `json:"beneficiary_name"`
}

// Razorpay Typed Objects
type RPAccount struct {
	ID                string     `json:"id"`
	Email             string     `json:"email"`
	Phone             string     `json:"phone"`
	Type              string     `json:"type"`
	ReferenceID       string     `json:"reference_id"`
	LegalBusinessName string     `json:"legal_business_name"`
	BusinessType      string     `json:"business_type"`
	Status            string     `json:"status"`
	ContactName       string     `json:"contact_name"`
	Profile           *RPProfile `json:"profile"`
}

type RPProfile struct {
	Category    string               `json:"category"`
	Subcategory string               `json:"subcategory"`
	Addresses   map[string]RPAddress `json:"addresses"`
}

type RPAddress struct {
	Street     string `json:"street,omitempty"`
	Street1    string `json:"street1,omitempty"`
	Street2    string `json:"street2,omitempty"`
	City       string `json:"city"`
	State      string `json:"state"`
	PostalCode string `json:"postal_code"`
	Country    string `json:"country"`
}

type RPStakeholder struct {
	ID        string               `json:"id"`
	Name      string               `json:"name"`
	Email     string               `json:"email"`
	Phone     string               `json:"phone"`
	Addresses map[string]RPAddress `json:"addresses"`
	KYC       *RPKYC               `json:"kyc"`
}

type RPKYC struct {
	PAN string `json:"pan"`
}

type RPProduct struct {
	ID          string         `json:"id"`
	AccountId   string         `json:"account_id"`
	ProductName string         `json:"product_name"`
	Status      string         `json:"status"`
	TncAccepted bool           `json:"tnc_accepted"`
	Settlements *RPSettlements `json:"settlements"`
}

type RPSettlements struct {
	AccountNumber   string `json:"account_number"`
	IFSCCode        string `json:"ifsc_code"`
	BeneficiaryName string `json:"beneficiary_name"`
}

func structFromMap(data map[string]interface{}, target interface{}) error {
	bytes, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return json.Unmarshal(bytes, target)
}

func CreateLinkedAccount(req CreateAccountReq) (*RPAccount, *RPStakeholder, error) {
	payload := map[string]interface{}{
		"email":               req.Email,
		"phone":               req.Phone,
		"type":                "route",
		"reference_id":        req.UserId,
		"legal_business_name": req.Name,
		"business_type":       "individual",
		"contact_name":        req.Name,
		"profile": map[string]interface{}{
			"category":    "it_and_software",
			"subcategory": "saas",
			"addresses": map[string]interface{}{
				"registered": map[string]interface{}{
					"street1":     req.Address.Street,
					"street2":     "-",
					"city":        req.Address.City,
					"state":       req.Address.State,
					"postal_code": req.Address.PostalCode,
					"country":     "IN",
				},
			},
		},
	}
	accountMap, err := Client.Account.Create(payload, nil)
	if err != nil {
		return nil, nil, err
	}

	account := &RPAccount{}
	if err := structFromMap(accountMap, account); err != nil {
		return nil, nil, err
	}

	// 🔹 Stakeholder payload
	stakeholderPayload := map[string]interface{}{
		"name":  req.Name,
		"email": req.Email,
		"addresses": map[string]interface{}{
			"residential": map[string]interface{}{
				"street":      req.Address.Street,
				"city":        req.Address.City,
				"state":       req.Address.State,
				"postal_code": req.Address.PostalCode,
				"country":     "IN",
			},
		},
		"kyc": map[string]interface{}{
			"pan": req.PAN,
		},
	}

	stkMap, err := Client.Stakeholder.Create(account.ID, stakeholderPayload, nil)
	if err != nil {
		return account, nil, err
	}

	stk := &RPStakeholder{}
	if err := structFromMap(stkMap, stk); err != nil {
		return account, nil, err
	}

	return account, stk, err
}

func UpdateAccountAndStakeHolderAddress(accountId string, stakeholderId string, req CreateAccountReq) (*RPAccount, *RPStakeholder, error) {
	payload := map[string]interface{}{
		"email":               req.Email,
		"legal_business_name": req.Name,
		"profile": map[string]interface{}{
			"addresses": map[string]interface{}{
				"registered": map[string]interface{}{
					"street1":     req.Address.Street,
					"street2":     "-",
					"city":        req.Address.City,
					"state":       req.Address.State,
					"postal_code": req.Address.PostalCode,
					"country":     "IN",
				},
			},
		},
	}
	accountMap, err := Client.Account.Edit(accountId, payload, nil)
	if err != nil {
		return nil, nil, err
	}

	account := &RPAccount{}
	if err := structFromMap(accountMap, account); err != nil {
		return nil, nil, err
	}

	// 🔹 Stakeholder payload
	stakeholderPayload := map[string]interface{}{
		"addresses": map[string]interface{}{
			"residential": map[string]interface{}{
				"street":      req.Address.Street,
				"city":        req.Address.City,
				"state":       req.Address.State,
				"postal_code": req.Address.PostalCode,
				"country":     "IN",
			},
		},
		"kyc": map[string]interface{}{
			"pan": req.PAN,
		},
	}

	stkMap, err := Client.Stakeholder.Edit(accountId, stakeholderId, stakeholderPayload, nil)
	if err != nil {
		return account, nil, err
	}

	stk := &RPStakeholder{}
	if err := structFromMap(stkMap, stk); err != nil {
		return account, nil, err
	}

	return account, stk, err
}

func CreataOrUpdateProduct(accountId string, bank BankReq) (*RPProduct, error) {
	prodConfMap, err := Client.Product.RequestProductConfiguration(accountId, map[string]interface{}{
		"product_name": "route",
		"tnc_accepted": true,
	}, nil)

	if err != nil {
		return nil, err
	}

	productPayload := map[string]interface{}{
		"settlements": map[string]interface{}{
			"account_number":   bank.AccountNumber,
			"ifsc_code":        bank.IFSC,
			"beneficiary_name": bank.BenificiaryName,
		},
		"tnc_accepted": true,
	}

	prodMap, err := Client.Product.Edit(accountId, prodConfMap["id"].(string), productPayload, nil)
	if err != nil {
		return nil, err
	}

	prod := &RPProduct{}
	if err := structFromMap(prodMap, prod); err != nil {
		return nil, err
	}

	return prod, err
}

func GetProduct(accountId string) (*RPProduct, error) {
	productMap, err := Client.Product.RequestProductConfiguration(accountId, map[string]interface{}{
		"product_name": "route",
		"tnc_accepted": true,
	}, nil)

	if err != nil {
		return nil, err
	}

	product := &RPProduct{}
	if err := structFromMap(productMap, product); err != nil {
		return nil, err
	}

	return product, nil
}

func FetchLinkedAccount(accountId string) (*RPAccount, error) {
	accountMap, err := Client.Account.Fetch(accountId, nil, nil)
	if err != nil {
		return nil, err
	}

	account := &RPAccount{}
	if err := structFromMap(accountMap, account); err != nil {
		return nil, err
	}

	return account, err
}

func FetchProductConfiguration(accountId string, prodId string) (*RPProduct, error) {
	productMap, err := Client.Product.Fetch(accountId, prodId, nil, nil)
	if err != nil {
		return nil, err
	}

	product := &RPProduct{}
	if err := structFromMap(productMap, product); err != nil {
		return nil, err
	}

	return product, nil
}

func DeleteAccount(accountId string) (map[string]interface{}, error) {
	response, err := Client.Account.Delete(accountId, nil, nil)
	return response, err
}
