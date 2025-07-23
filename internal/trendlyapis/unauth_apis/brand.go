package trendlyunauth

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
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

	bmember := trendlymodels.BrandMember{
		ManagerID: uid,
		Role:      "user",
		Status:    1,
	}
	_, err = bmember.Set(req.BrandID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s?email=%s", os.Getenv("BRAND_LOGIN_URL"), uRecord.Email))
}
