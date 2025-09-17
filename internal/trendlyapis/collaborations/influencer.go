package trendlyCollabs

import (
	"context"
	"net/http"

	stream_chat "github.com/GetStream/stream-chat-go/v5"
	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/myutil"
	"github.com/idivarts/backend-sls/pkg/streamchat"
)

type requestWithBrands struct {
	BrandId string `json:"brandId" binding:"required"`
}

func InfluencerUnlocked(c *gin.Context) {
	var req requestWithBrands
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Input invalid", "error": err.Error()})
		return
	}

	influenerId := c.Param(("influencerId"))
	managerId, b := middlewares.GetUserId(c)
	if !b {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Error fetching userId from token"})
		return
	}

	brand := &trendlymodels.Brand{}
	err := brand.Get(req.BrandId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error fetching brand"})
		return
	}

	brand.UnlockedInfluencers, b = myutil.AppendUnique(brand.UnlockedInfluencers, influenerId)
	if b {
		if brand.Credits.Influencer <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "insufficient-credits", "message": "Insufficient Credits"})
			return
		}
		brand.Credits.Influencer -= 1
	}

	_, err = brand.Insert(req.BrandId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error Inserting Brand with Unlocked Influencers"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Successfully Notified for starting contract", "influencerId": influenerId, "managerId": managerId, "brandId": req.BrandId})
}

func SendMessage(c *gin.Context) {
	var req requestWithBrands
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Input invalid", "error": err.Error()})
		return
	}

	influenerId := c.Param(("influencerId"))
	managerId, b := middlewares.GetUserId(c)
	if !b {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Error fetching userId from token"})
		return
	}

	manager := &trendlymodels.Manager{}
	err := manager.Get(managerId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error fetching user"})
		return
	}

	user := &trendlymodels.User{}
	err = user.Get(influenerId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error fetching user"})
		return
	}

	brand := &trendlymodels.Brand{}
	err = brand.Get(req.BrandId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error fetching brand"})
		return
	}

	has := false
	for i := range brand.UnlockedInfluencers {
		if brand.UnlockedInfluencers[i] == influenerId {
			has = true
			break
		}
	}

	if !has {
		c.JSON(http.StatusBadRequest, gin.H{"error": "influencer-locked", "message": "Influencer not unlocked"})
		return
	}

	channel, err := streamchat.Client.CreateChannel(context.Background(), "messaging", "", managerId, &stream_chat.ChannelRequest{
		Members: []string{managerId, influenerId},
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error creating channel"})
		return
	}

	// Send Stream Notification
	err = streamchat.SendSystemMessage(channel.Channel.ID, "This message is initiated as the brand "+brand.Name+" wanted to connect with you")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Stream Error"})
		return
	}

	if !manager.IsChatConnected {
		manager.IsChatConnected = true
		_, err = manager.Insert(managerId)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error saving manager"})
			return
		}

	}
	if !user.IsChatConnected {
		user.IsChatConnected = true
		_, err = user.Insert(influenerId)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error saving influencer"})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "Successfully Notified for starting contract", "channel": channel})
}
