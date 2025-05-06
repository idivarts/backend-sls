package trendlyCollabs

import (
	"net/http"

	"github.com/gin-gonic/gin"
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
	userId := c.Param("collabId")

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

	// {{.InfluencerName}}  => Name of the influencer receiving the invite
	// {{.BrandName}}       => Name of the brand sending the invitation
	// {{.CollabTitle}}     => Title of the collaboration
	// {{.ApplyLink}}       => Link to view/apply for the collaboration

	myemail.SendCustomHTMLEmail(*user.Email, templates.InfluencerInvitedToCollab, templates.SubjectBrandInvitedYouToCollab, map[string]interface{}{
		"InfluencerName": user.Name,
		"CollabTitle":    collab.Name,
	})
}
