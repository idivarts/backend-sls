package trendlyapis_test

import (
	"log"
	"testing"

	"github.com/idivarts/backend-sls/internal/trendlyapis"
)

func TestGenerateLink(t *testing.T) {
	link, err := trendlyapis.GenerateInvitationLink("rahul.m5@idiv.in", false, "2MpUMTb1SUXLZBCtyn3h")
	if err != nil {
		t.Error(err)
	}
	log.Println("Link", ":", link, err)
}
