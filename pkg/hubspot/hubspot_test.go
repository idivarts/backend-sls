package hubspot_test

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/idivarts/backend-sls/pkg/hubspot"
	"github.com/idivarts/backend-sls/pkg/myemail"
)

func TestDataPush(t *testing.T) {
	err := hubspot.CreateOrUpdateContacts([]myemail.ContactDetails{{
		Email:             "rahul@idiv.in",
		Name:              "Rahul Sinha",
		Phone:             "7604007156",
		IsManager:         true,
		CompanyName:       "Trendly",
		ProfileCompletion: 90,
		CreationTime:      aws.Int64(time.Now().UnixMilli()),
		LastActivityTime:  aws.Int64(time.Now().UnixMilli()),
	}})
	if err != nil {
		t.Error(err.Error())
	}
	t.Log("Success")
}
