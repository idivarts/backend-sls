package messenger_test

import (
	"testing"

	"github.com/TrendsHub/th-backend/pkg/messenger"
)

func TestUser(t *testing.T) {
	igsid := "739486008356543"
	profile, err := messenger.GetUser(igsid)
	if err != nil {
		t.Errorf("Error %s", err.Error())
	}
	t.Log("Profile Information", profile.GenerateUserDescription())
}
