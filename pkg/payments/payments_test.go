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
	_, link, err := payments.CreatePaymentLink(499, payments.Customer{
		Name:        "Rahul",
		Email:       "rahul.test1@idiv.in",
		PhoneNumber: "9905264774",
	}, map[string]interface{}{})
	if err != nil {
		t.Error(err)
	}
	log.Println("Link", link)
}

// plan_QPkwSFj9oy45l6

func TestCreateSubscriptionLink(t *testing.T) {
	_, link, err := payments.CreateSubscriptionLink("plan_QPkwSFj9oy45l6", 12, 3, 1, map[string]interface{}{}, "")
	if err != nil {
		t.Error(err)
	}
	log.Println("Link", link)
}
func TestUpdateSubscription(t *testing.T) {
	res, err := payments.UpdateSubscription("sub_S4tlcF5imQIVim", "plan_RA4xx2KPqnK1na")
	if err != nil {
		t.Error(err)
		return
	}
	log.Println("Updated", res)
}
