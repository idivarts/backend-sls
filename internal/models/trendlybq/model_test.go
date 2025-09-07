package trendlybq_test

import (
	"testing"

	"github.com/idivarts/backend-sls/internal/models/trendlybq"
)

func TestSocialInsert(t *testing.T) {
	data := trendlybq.Socials{
		ID: "123",
	}

	err := data.Insert()
	if err != nil {
		t.Fatal(err.Error())
	}

	t.Log("Inserted")
}
