package facebook_test

import (
	"testing"

	"github.com/idivarts/backend-sls/pkg/facebook"
)

func TestUser(t *testing.T) {
	igsid := "739486008356543"
	profile, err := facebook.GetUser(igsid, facebook.TestPageAccessToken)
	if err != nil {
		t.Errorf("Error %s", err.Error())
	}
	t.Log("Profile Information", profile.GenerateUserDescription())
}
