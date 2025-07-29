package payments_test

import (
	"log"
	"testing"

	"github.com/idivarts/backend-sls/pkg/payments"
)

func TestCreateOrder(t *testing.T) {
	payments.CreateOrder(499, map[string]interface{}{})
}

func TestCreatePaymentLink(t *testing.T) {
	link, err := payments.CreatePaymentLink(499, payments.Customer{
		Name:        "Rahul",
		Email:       "rahul.test1@idiv.in",
		PhoneNumber: "9905264774",
	}, map[string]interface{}{})
	if err != nil {
		t.Error(err)
	}
	log.Println("Link", link)
}
