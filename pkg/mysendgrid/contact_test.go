package mysendgrid_test

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/idivarts/backend-sls/pkg/crm"
	"github.com/idivarts/backend-sls/pkg/mysendgrid"
)

func TestDataPush(t *testing.T) {
	err := mysendgrid.CreateOrUpdateContacts([]crm.ContactDetails{{
		Email:             "rahul2@idiv.in",
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

func TestGetData(t *testing.T) {
	contact, err := mysendgrid.FetchContacts()
	if err != nil {
		t.Error(err.Error())
	}
	t.Log("Success", len(contact), "contacts found")
}

func TestGetJobStatus(t *testing.T) {
	status, err := mysendgrid.GetJobStatus("dfa71a35-a149-4f9d-890b-b1dfc9251f49")
	if err != nil {
		t.Error(err.Error())
	}
	t.Log("Job Status:", status)
}
