package myemail_test

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/idivarts/backend-sls/pkg/myemail"
)

func TestDataPush(t *testing.T) {
	err := myemail.CreateOrUpdateContacts([]myemail.ContactDetails{{
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
	contact, err := myemail.FetchContacts()
	if err != nil {
		t.Error(err.Error())
	}
	t.Log("Success", len(contact), "contacts found")
}

func TestGetJobStatus(t *testing.T) {
	status, err := myemail.GetJobStatus("dfa71a35-a149-4f9d-890b-b1dfc9251f49")
	if err != nil {
		t.Error(err.Error())
	}
	t.Log("Job Status:", status)
}
