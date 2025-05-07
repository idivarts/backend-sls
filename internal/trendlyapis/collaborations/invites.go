package trendlyCollabs

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/constants"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/myemail"
	"github.com/idivarts/backend-sls/templates"
)

func SendInvitation(c *gin.Context) {
	userType := middlewares.GetUserType(c)
	if userType != "manager" {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "Only Managers can call this endpoint"})
	}
	collabId := c.Param("collabId")
	userId := c.Param("userId")

	manager := middlewares.GetUserObject(c)
	managerName, _ := manager["name"].(string)

	user := &trendlymodels.User{}
	err := user.Get(userId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	collab := &trendlymodels.Collaboration{}
	err = collab.Get(collabId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	brand := &trendlymodels.Brand{}
	err = brand.Get(collab.BrandID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	log.Println("Got Brands, Collabs and User")

	notif := &trendlymodels.Notification{
		Title:       fmt.Sprintf("You have been invited to %s", collab.Name),
		Description: fmt.Sprintf("%s (from %s) has invited you to this collaboration. Apply Now!", managerName, brand.Name),
		IsRead:      false,
		Data: &trendlymodels.NotificationData{
			CollaborationID: &collabId,
			UserID:          &userId,
		},
		TimeStamp: time.Now().UnixMilli(),
		Type:      "invitation",
	}
	_, err = notif.Insert(trendlymodels.USER_COLLECTION, userId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	log.Println("Created Notifications... Now sending email")

	// {{.InfluencerName}}  => Name of the influencer receiving the invite
	// {{.BrandName}}       => Name of the brand sending the invitation
	// {{.CollabTitle}}     => Title of the collaboration
	// {{.ApplyLink}}       => Link to view/apply for the collaboration

	myemail.SendCustomHTMLEmail(*user.Email, templates.InfluencerInvitedToCollab, templates.SubjectBrandInvitedYouToCollab, map[string]interface{}{
		"InfluencerName": user.Name,
		"BrandName":      brand.Name,
		"CollabTitle":    collab.Name,
		"ApplyLink":      fmt.Sprintf("%s/collaboration/%s", constants.TRENDLY_CREATORS_FE, collabId),
	})

	c.JSON(http.StatusOK, gin.H{"message": "Successfully notified user for message"})
}
