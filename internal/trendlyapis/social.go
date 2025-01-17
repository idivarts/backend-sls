package trendlyapis

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/instagram"
	"github.com/idivarts/backend-sls/pkg/messenger"
)

func FetchInsights(c *gin.Context) {
	userId, b := middlewares.GetUserId(c)
	if !b {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid User"})
		return
	}

	user := &trendlymodels.User{}
	err := user.Get(userId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid User", "error": err.Error()})
		return
	}

	pSocial := user.PrimarySocial
	if pSocial == nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Primary Social not found"})
		return
	}

	social := &trendlymodels.SocialsPrivate{}
	social.Get(userId, *pSocial)

	if social.GraphType == 0 {
		// facebook

	} else if social.GraphType == 1 {
		// instagram

	} else {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid Social"})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{"message": "FetchReach"})
}

func FetchMedias(c *gin.Context) {
	userId := c.Query("userId")
	// if !b {
	// 	c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid User"})
	// 	return
	// }

	user := &trendlymodels.User{}
	err := user.Get(userId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid User", "error": err.Error()})
		return
	}

	pSocial := user.PrimarySocial
	if pSocial == nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Primary Social not found"})
		return
	}

	socialPriv := &trendlymodels.SocialsPrivate{}
	err = socialPriv.Get(userId, *pSocial)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Primary Social not found", "error": err.Error()})
		return
	}

	social := &trendlymodels.Socials{}
	err = social.Get(userId, *pSocial)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Primary Social not found", "error": err.Error()})
		return
	}

	response := map[string]interface{}{}
	if social.IsInstagram {
		// instagram
		medias, err := instagram.GetMedia(*socialPriv.AccessToken, instagram.IGetMediaParams{
			GraphType: int(socialPriv.GraphType),
			PageID:    *pSocial,
		})
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "Error Fetching Medias", "error": err.Error()})
			return
		}

		response["isInstagram"] = true
		response["medias"] = medias
	} else {
		// facebook
		posts, err := messenger.GetPosts(*socialPriv.AccessToken, messenger.IFBPostsParams{})
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "Error Fetching Posts", "error": err.Error()})
			return
		}
		response["isInstagram"] = false
		response["posts"] = posts
	}

	c.JSON(http.StatusAccepted, gin.H{"message": "Successfully fetched Media/Posts", "data": response})
}
