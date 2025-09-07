package trendlybq_test

import (
	"testing"

	"github.com/idivarts/backend-sls/internal/models/trendlybq"
)

func TestSocialInsert(t *testing.T) {
	data := trendlybq.Socials{
		SocialType: "instagram",
		Username:   "test_username",
	}

	err := data.Insert()
	if err != nil {
		t.Fatal(err.Error())
	}

	t.Log("Inserted")
}

func TestSocialGet(t *testing.T) {
	data := &trendlybq.Socials{}

	err := data.GetInstagram("test_username")
	if err != nil {
		t.Fatal(err.Error())
	}

	t.Log("Found:", data)
}
