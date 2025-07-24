package influencerv2

import (
	"context"
	"fmt"
	"net/http"

	stream_chat "github.com/GetStream/stream-chat-go/v5"
	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/constants"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/myemail"
	"github.com/idivarts/backend-sls/pkg/myutil"
	"github.com/idivarts/backend-sls/pkg/streamchat"
	"github.com/idivarts/backend-sls/templates"
)

func AcceptInfluencerInvite(c *gin.Context) {
	influencerId := c.Param("influencerId")
	userId, b := middlewares.GetUserId(c)
	if !b || influencerId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "User not found", "message": "UserId is needed found"})
		return
	}
	if influencerId == userId {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request", "message": "You cannot invite yourself"})
		return
	}

	influencer := &trendlymodels.User{}
	err := influencer.Get(influencerId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Influencer not found"})
		return
	}

	// user := middlewares.GetUserObject(c)

	invitation := &trendlymodels.InfluencerInvite{}
	err = invitation.Get(userId, influencerId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Invitation doesnt exist"})
		return
	}

	invitation.Status = 1 // Set status to Rejected
	_, err = invitation.Insert(userId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Failed to reject invitation"})
		return
	}
	// Create a Message thread between these two people
	channel, err := streamchat.Client.CreateChannel(context.Background(), "messaging", "", userId, &stream_chat.ChannelRequest{
		Members: []string{userId, influencerId},
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Unable to create channel"})
		return
	}

	// Dynamic Variables:
	// {{.RecipientName}}   => Name of the influencer who sent the invite
	// {{.AcceptorName}}    => Name of the influencer who accepted the invite
	// {{.ChatLink}}        => Link to open the chat inside the Trendly app
	data := map[string]interface{}{
		"RecipientName": influencer.Name,
		"AcceptorName":  middlewares.GetUserObject(c)["name"].(string),
		"ChatLink":      fmt.Sprintf("%s/messages?channelId=%s", constants.TRENDLY_CREATORS_FE, channel.Channel.ID),
	}

	err = myemail.SendCustomHTMLEmail(myutil.DerefString(influencer.Email), templates.InfluencerInvite, templates.SubjectInfluencerInvite, data)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Error Sending Email"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Invitation accepted successfully", "channel": channel.Channel})
}

type RejectInvite struct {
	Reason string `json:"reason" binding:"required"`
}

func RejectInfluencerInvite(c *gin.Context) {
	influencerId := c.Param("influencerId")
	userId, b := middlewares.GetUserId(c)
	if !b || influencerId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "User not found", "message": "UserId is needed found"})
		return
	}
	if influencerId == userId {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request", "message": "You cannot invite yourself"})
		return
	}

	var req RejectInvite
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Invalid request"})
		return
	}

	influencer := &trendlymodels.User{}
	err := influencer.Get(influencerId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Influencer not found"})
		return
	}

	// user := middlewares.GetUserObject(c)

	invitation := &trendlymodels.InfluencerInvite{}
	err = invitation.Get(userId, influencerId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Invitation doesnt exist"})
		return
	}

	invitation.Status = 2 // Set status to Rejected
	_, err = invitation.Insert(userId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Failed to reject invitation"})
		return
	}

	// Dynamic Variables:
	// {{.RecipientName}}   => Name of the influencer who sent the invite
	// {{.RejectionReason}} => Selected reason for rejection
	// {{.AcceptorName}}    => Name of the influencer who rejected the invite
	// {{.ExploreLink}}     => Link to explore more influencers or opportunities
	data := map[string]interface{}{
		"RecipientName":   influencer.Name,
		"RejectionReason": req.Reason,
		"AcceptorName":    middlewares.GetUserObject(c)["name"].(string),
		"ExploreLink":     fmt.Sprintf("%s/influencers", constants.TRENDLY_CREATORS_FE), // Example link, replace with actual link if needed
	}

	err = myemail.SendCustomHTMLEmail(myutil.DerefString(influencer.Email), templates.InfluencerInvite, templates.SubjectInfluencerInvite, data)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Error Sending Email"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Invitation rejected successfully"})
}
