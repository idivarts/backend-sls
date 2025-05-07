package trendlyCollabs_test

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	trendlyCollabs "github.com/idivarts/backend-sls/internal/trendlyapis/collaborations"
)

func TestInvites(t *testing.T) {
	// Create a ResponseRecorder to inspect the response later
	w := httptest.NewRecorder()
	// Create a new Gin context
	c, _ := gin.CreateTestContext(w)

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
	t.Log("Success", w.Body.String())
}
