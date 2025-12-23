package payments

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

func CreateLinkedAccount(req CreateAccountReq) (map[string]interface{}, map[string]interface{}, error) {
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
					"street2":     "",
					"city":        req.Address.City,
					"state":       req.Address.State,
					"postal_code": req.Address.PostalCode,
					"country":     "IN",
				},
			},
		},
	}
	account, err := Client.Account.Create(payload, nil)
	if err != nil {
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

	stk, err := Client.Stakeholder.Create(account["id"].(string), stakeholderPayload, nil)

	return account, stk, err
}

func CreataProductConfiguration(accountId string, bank BankReq) (map[string]interface{}, error) {
	prodConf, err := Client.Product.RequestProductConfiguration(accountId, map[string]interface{}{
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

	prod, err := Client.Product.Edit(accountId, prodConf["id"].(string), productPayload, nil)

	return prod, err
}

func FetchLinkedAccount(accountId string) (map[string]interface{}, error) {
	account, err := Client.Account.Fetch(accountId, nil, nil)
	return account, err
}

func FetchProductConfiguration(accountId string, prodId string) (map[string]interface{}, error) {
	product, err := Client.Product.Fetch(accountId, prodId, nil, nil)
	return product, err
}
