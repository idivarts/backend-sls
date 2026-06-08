package trendlyunauth

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/constants"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	myjwt "github.com/idivarts/backend-sls/internal/trendlyapis/jwt"
	"github.com/idivarts/backend-sls/pkg/firebase/fauth"
)

type FirebaseActionRequest struct {
	Token   string `form:"token" binding:"required"`   // Out-of-band code for the action
	BrandID string `form:"brandId" binding:"required"` // Operation mode (e.g., resetPassword, verifyEmail)
}

func ValidateFirebaseCallback(c *gin.Context) {
	var req FirebaseActionRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	uid, err := myjwt.DecodeUID(req.Token)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	uRecord, err := fauth.Client.GetUser(context.Background(), uid)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Preserve the team assigned at invite time — only flip the status to
	// accepted. Fall back to the brand's default team if no pending invite exists.
	bmember := trendlymodels.BrandMember{}
	if err = bmember.Get(req.BrandID, uid); err != nil {
		defTeams, _ := trendlymodels.EnsureDefaultTeam(req.BrandID, uid, time.Now().UnixMilli())
		var defTeamID string
		if len(defTeams) > 0 {
			defTeamID = defTeams[0].ID
		}
		bmember = trendlymodels.BrandMember{ManagerID: uid, TeamID: defTeamID}
	}
	bmember.ManagerID = uid
	bmember.Status = 1
	_, err = bmember.Set(req.BrandID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s%s?email=%s", constants.GetBrandsFronted(), os.Getenv("BRAND_LOGIN_URL"), uRecord.Email))
}
