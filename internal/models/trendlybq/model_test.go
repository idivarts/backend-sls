package trendlybq_test

import (
	"testing"

	"github.com/idivarts/backend-sls/internal/models/trendlybq"
)

func TestGetID(t *testing.T) {
	data := trendlybq.SocialsN8N{
		Username:   "test",
		SocialType: "instagram",
	}
	id := data.GetID()
	if id == "" {
		t.Error("ID should not be empty")
	}
}

func TestInsertToFirestore(t *testing.T) {
	data := &trendlybq.SocialsN8N{
		Username:   "test",
		SocialType: "instagram",
	}
	err := data.InsertToFirestore(false)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Log("Inserted")
}

func TestSocialInsert(t *testing.T) {
	data := trendlybq.SocialsN8N{
		SocialType: "instagram",
		Username:   "test_username",
	}

	err := data.InsertToFirestore(false)
	if err != nil {
		t.Fatal(err.Error())
	}

	t.Log("Inserted")
}

func TestSocialGet(t *testing.T) {
	data := &trendlybq.SocialsN8N{}

	err := data.GetInstagramFromFirestore("test_username")
	if err != nil {
		t.Fatal(err.Error())
	}

	t.Log("Found:", data.LastUpdateTime)
}
