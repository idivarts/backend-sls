package influencerv2

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
)

func InviteInfluencer(c *gin.Context) {
	influencerId := c.Param("influencerId")
	userId, b := middlewares.GetUserId(c)

	if !b {
		c.JSON(http.StatusBadRequest, gin.H{"error": "User not found", "message": "UserId is needed found"})
		return
	}

	firestoredb.Client.Collection("users").Doc(influencerId).Collection("invitations").Doc(userId)

}
