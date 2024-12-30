package trendlyapis_test

import (
	"context"
	"log"
	"testing"

	"firebase.google.com/go/v4/auth"
	"github.com/idivarts/backend-sls/internal/trendlyapis"
	"github.com/idivarts/backend-sls/pkg/firebase/fauth"
)

func TestGenerateLink(t *testing.T) {
	userRecord, err := fauth.Client.GetUserByEmail(context.Background(), "rahul.t3@idiv.in")
	if err != nil {
		t.Error(err)
	}
	link, err := trendlyapis.GenerateInvitationLink(userRecord.Email, userRecord.EmailVerified, "2MpUMTb1SUXLZBCtyn3h", userRecord.UID)
	if err != nil {
		t.Error(err)
	}
	log.Println("Link", ":", link, err)
}

func TestCreateNewUser(t *testing.T) {
	userToCreate := (&auth.UserToCreate{}).Email("rahul.t3@idiv.in").EmailVerified(false)

	userRecord, err := fauth.Client.CreateUser(context.Background(), userToCreate)
	if err != nil {
		t.Error(err)
	}
	link, err := trendlyapis.GenerateInvitationLink(userRecord.Email, userRecord.EmailVerified, "2MpUMTb1SUXLZBCtyn3h", userRecord.UID)
	if err != nil {
		t.Error(err)
	}
	log.Println("Link", ":", link, err)
}
