package payments_test

import (
	"testing"

	"github.com/idivarts/backend-sls/pkg/payments"
)

func TestCreateAccount(t *testing.T) {
	d1, d2, err := payments.CreateLinkedAccount(payments.CreateAccountReq{
		Name:   "Rahul Sinha",
		Email:  "rahul.la15@idiv.in",
		Phone:  "9905264774",
		UserId: "hello123",
		Address: payments.AddressReq{
			Street:     "229 Shatipally",
			City:       "Kolkata",
			State:      "West Bengal",
			PostalCode: "700107",
		},
		PAN: "INYPS4790C",
	})
	if err != nil {
		t.Error(err)
	}
	t.Log(d1, d2)
}

func TestAddBank(t *testing.T) {
	accountId := "acc_RvBOQXybZiwgdZ"

	data, err := payments.CreataOrUpdateProduct(accountId, payments.BankReq{
		AccountNumber:   "142001551678",
		IFSC:            "ICIC0001420",
		BenificiaryName: "Rahul Sinha",
	})
	if err != nil {
		t.Error(err)
	}
	t.Log(data)
}

func TestGetProduct(t *testing.T) {
	accountId := "acc_RvBOQXybZiwgdZ"
	prodId := "acc_prd_RvBR9D6k7XlDCl"

	data, err := payments.FetchProductConfiguration(accountId, prodId)
	if err != nil {
		t.Error(err)
	}
	t.Log(data)
}

func TestDeleteAccount(t *testing.T) {
	data, err := payments.Client.Account.Delete("acc_RvBOQXybZiwgdZ", nil, nil)
	if err != nil {
		t.Error(err)
	}
	t.Log(data)
}
