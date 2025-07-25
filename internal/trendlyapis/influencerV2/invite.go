package influencerv2

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/constants"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/myemail"
	"github.com/idivarts/backend-sls/pkg/myutil"
	"github.com/idivarts/backend-sls/templates"
)

func InviteInfluencer(c *gin.Context) {
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

	var req trendlymodels.InfluencerInvite
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

	user := middlewares.GetUserObject(c)

	err = (&trendlymodels.InfluencerInvite{}).Get(influencerId, userId)
	if err == nil {
		// If no error, it means the invitation already exists
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invitation already exists", "message": "Invitation already exists for this influencer"})
		return
	}

	req.Status = 0 // Set status to Pending
	req.InfluencerId = userId
	_, err = req.Insert(influencerId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Failed to send invitation"})
		return
	}

	// Push Notification
	notif := &trendlymodels.Notification{
		Title:       fmt.Sprintf("%s has invited to collaborate", user["name"].(string)),
		Description: "Review their request and decide whether to accept or reject the invitation.",
		IsRead:      false,
		Data: &trendlymodels.NotificationData{
			UserID: &userId,
		},
		TimeStamp: time.Now().UnixMilli(),
		Type:      "influencer-invite",
	}
	_, _, err = notif.Insert(trendlymodels.USER_COLLECTION, influencerId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Error Sending Notification"})
		return
	}

	// Dynamic Variables:
	// {{.RecipientName}}   => Name of the influencer receiving the invite
	// {{.SenderName}}      => Name of the influencer who sent the invite
	// {{.Reason}}          => Reason or idea shared in the invite
	// {{.CollabType}}      => Type of collaboration being proposed (comma separated)
	// {{.CollabMode}}      => Collaboration type: Paid or Free
	// {{.BudgetMin}}       => Minimum proposed budget (can be blank if free)
	// {{.BudgetMax}}       => Maximum proposed budget (can be blank if free)
	// {{.ActionLink}}      => Link to open the Trendly app for this invite
	data := map[string]interface{}{
		"RecipientName": influencer.Name,
		"SenderName":    user["name"].(string),
		"Reason":        req.Reason,
		"CollabType":    strings.Join(req.CollabType, ", "),
		"CollabMode":    req.CollabMode,
		"BudgetMin":     req.BudgetMin,
		"BudgetMax":     req.BudgetMax,
		"ActionLink":    fmt.Sprintf("%s/invites?category=influencers&influencerId=%s", constants.TRENDLY_CREATORS_FE, influencerId),
	}

	err = myemail.SendCustomHTMLEmail(myutil.DerefString(influencer.Email), templates.InfluencerInvite, templates.SubjectInfluencerInvite, data)
	if err != nil {
		fmt.Printf("Error sending email to user %s: %s\n", myutil.DerefString(influencer.Email), err.Error())
		return
	}
}
