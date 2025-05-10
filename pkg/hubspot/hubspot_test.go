package hubspot_test

import (
	"testing"

	"github.com/idivarts/backend-sls/pkg/hubspot"
)

func TestDataPush(t *testing.T) {
	err := hubspot.CreateOrUpdateContacts([]hubspot.ContactDetails{{
		Email:             "rahul@idiv.in",
		Name:              "Rahul Sinha",
		Phone:             "7604007156",
		IsManager:         true,
		CompanyName:       "Trendly",
		ProfileCompletion: 90,
	}})
	if err != nil {
		t.Error(err.Error())
	}
	t.Log("Success")
}
