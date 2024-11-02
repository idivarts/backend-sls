package messenger_test

import (
	"testing"

	"github.com/idivarts/backend-sls/pkg/messenger"
)

func TestUser(t *testing.T) {
	igsid := "739486008356543"
	profile, err := messenger.GetUser(igsid, messenger.TestPageAccessToken)
	if err != nil {
		t.Errorf("Error %s", err.Error())
	}
	t.Log("Profile Information", profile.GenerateUserDescription())
}
