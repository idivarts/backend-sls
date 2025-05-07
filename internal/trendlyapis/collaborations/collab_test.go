package trendlyCollabs_test

import (
	"log"
	"testing"

	"github.com/gin-gonic/gin"
	trendlyCollabs "github.com/idivarts/backend-sls/internal/trendlyapis/collaborations"
)

func TestInvites(t *testing.T) {
	c := &gin.Context{}
	// /collaborations/1zpmgWGbbkUxuzx9g1sW/invitations/PVqKf3REinNvALQRQHFvKXpN1gx2
	c.Params = gin.Params{
		{
			Key:   "collabId",
			Value: "1zpmgWGbbkUxuzx9g1sW",
		},
		{
			Key:   "userId",
			Value: "PVqKf3REinNvALQRQHFvKXpN1gx2",
		},
	}
	trendlyCollabs.SendInvitation(c)
	log.Println("Success")
}
