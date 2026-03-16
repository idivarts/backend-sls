package matchmaking

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlyrdb"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/myutil"
)

func GetInfluencerForInfluencer(c *gin.Context) {
	userId, b := middlewares.GetUserId(c)
	if !b {
		c.JSON(http.StatusBadRequest, gin.H{"error": "User not authenticated", "message": "UserId not found"})
		return
	}

	user := &trendlymodels.User{}
	err := user.Get(userId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "User not found"})
		return
	}

	influencers, err := RunBQ2(myutil.DerefString(user.Location))

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Influencers not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": "Influencers found", "influencers": influencers})
}

func RunBQ2(location string) ([]string, error) {
	influencersModel := trendlyrdb.Influencers{}
	return influencersModel.GetInfluencerForInfluencerIDs(location, 100)
}
