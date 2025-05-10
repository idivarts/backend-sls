package trendlyCollabs_test

import (
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/constants"
	trendlyCollabs "github.com/idivarts/backend-sls/internal/trendlyapis/collaborations"
	"github.com/idivarts/backend-sls/pkg/myemail"
	"github.com/idivarts/backend-sls/pkg/mytime"
	"github.com/idivarts/backend-sls/templates"
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

func TestMultiEmail(t *testing.T) {
	data := map[string]interface{}{
		"BrandName":       "brand.Name",
		"InfluencerName":  "userName",
		"CollabTitle":     "collab.Name",
		"InfluencerEmail": "userEmail",
		"ApplicationTime": mytime.FormatPrettyIST(time.Now()),
		"CollabLink":      fmt.Sprintf("%s/collaboration-details/%s", constants.TRENDLY_BRANDS_FE, "collabId"),
	}

	err := myemail.SendCustomHTMLEmailToMultipleRecipients([]string{"rahul@idiv.in", "debanganamukherjee86@gmail.com"}, templates.ApplicationSent, templates.SubjectInfluencerAppliedToCollab, data)
	if err != nil {
		t.Error(err.Error())
		return
	}
	t.Log("Successful")
}
