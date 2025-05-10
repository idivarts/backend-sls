package hubspot_test

import (
	"testing"

	"github.com/idivarts/backend-sls/pkg/hubspot"
)

func TestDataPush(t *testing.T) {
	err := hubspot.CreateOrUpdateContact(hubspot.ContactDetails{
		Email: "rahul@idiv.in",
		Name:  "Rahul Sinha 2",
		Phone: "7604007156",
	})
	if err != nil {
		t.Error(err.Error())
	}
	t.Log("Success")
}
