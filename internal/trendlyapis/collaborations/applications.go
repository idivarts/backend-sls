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
	"github.com/idivarts/backend-sls/pkg/mytime"
	"github.com/idivarts/backend-sls/pkg/streamchat"
	"github.com/idivarts/backend-sls/templates"
)

func SendApplication(c *gin.Context) {
	userType := middlewares.GetUserType(c)
	if userType != "user" {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "Only Users can call this endpoint"})
		return
	}
	collabId := c.Param("collabId")
	userId := c.Param("userId")

	user := middlewares.GetUserObject(c)
	userName, _ := user["name"].(string)
	userEmail, _ := user["email"].(string)

	collab := &trendlymodels.Collaboration{}
	err := collab.Get(collabId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error fetching collaboration"})
		return
	}

	brand := &trendlymodels.Brand{}
	err = brand.Get(collab.BrandID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error fetching Brand"})
		return
	}

	application := &trendlymodels.Application{}
	err = application.Get(collabId, userId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error fetching application"})
		return
	}

	log.Println("Got Brands, Collabs and User")

	notif := &trendlymodels.Notification{
		Title:       fmt.Sprintf("Received new application : %s", userName),
		Description: fmt.Sprintf("You have received application on the collaboration %s", collab.Name),
		IsRead:      false,
		Data: &trendlymodels.NotificationData{
			CollaborationID: &collabId,
			UserID:          &userId,
		},
		TimeStamp: time.Now().UnixMilli(),
		Type:      "application",
	}
	_, emails, err := notif.Insert(trendlymodels.BRAND_COLLECTION, collab.BrandID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error inserting notification"})
		return
	}

	log.Println("Created Notifications... Now sending email")

	// 	<!--
	//   Dynamic Variables:
	//     {{.BrandName}}         => Name of the brand
	//     {{.CollabTitle}}       => Title of the collaboration
	//     {{.InfluencerName}}    => Full name of the influencer who applied
	//     {{.InfluencerEmail}}   => Email of the influencer
	//     {{.ApplicationTime}}   => Timestamp of the application
	//     {{.CollabLink}}        => Direct link to view the collaboration
	// -->

	data := map[string]interface{}{
		"BrandName":       brand.Name,
		"InfluencerName":  userName,
		"CollabTitle":     collab.Name,
		"InfluencerEmail": userEmail,
		"ApplicationTime": mytime.FormatPrettyIST(time.UnixMilli(application.TimeStamp)),
		"CollabLink":      fmt.Sprintf("%s/collaboration-details/%s", constants.TRENDLY_BRANDS_FE, collabId),
	}

	err = myemail.SendCustomHTMLEmailToMultipleRecipients(emails, templates.ApplicationSent, templates.SubjectInfluencerAppliedToCollab, data)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error sending email"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Successfully notified user for message"})

}

func EditApplication(c *gin.Context) {
	userType := middlewares.GetUserType(c)
	if userType != "user" {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "Only Users can call this endpoint"})
		return
	}
	collabId := c.Param("collabId")
	userId := c.Param("userId")

	user := middlewares.GetUserObject(c)
	userName, _ := user["name"].(string)
	// userEmail, _ := user["email"].(string)

	collab := &trendlymodels.Collaboration{}
	err := collab.Get(collabId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Collaboration fetch issue"})
		return
	}

	brand := &trendlymodels.Brand{}
	err = brand.Get(collab.BrandID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Brand fetch issue"})
		return
	}

	application := &trendlymodels.Application{}
	err = application.Get(collabId, userId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Application fetch issue"})
		return
	}

	log.Println("Got Brands, Collabs and User")

	notif := &trendlymodels.Notification{
		Title:       fmt.Sprintf("%s has changed their Quotation", userName),
		Description: fmt.Sprintf("You have received a new quotation on the collaboration %s", collab.Name),
		IsRead:      false,
		Data: &trendlymodels.NotificationData{
			CollaborationID: &collabId,
			UserID:          &userId,
		},
		TimeStamp: time.Now().UnixMilli(),
		Type:      "new-quotation",
	}
	_, emails, err := notif.Insert(trendlymodels.BRAND_COLLECTION, collab.BrandID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Notification creation issue"})
		return
	}

	log.Println("Created Notifications... Now sending email")

	// 	<!--
	//   Dynamic Variables:
	//     {{.BrandMemberName}}   => Name of the brand team member receiving the email
	//     {{.InfluencerName}}    => Name of the influencer who submitted the revised quotation
	//     {{.CollabTitle}}       => Title of the collaboration
	//     {{.SubmissionTime}}    => Time when the revised quotation was submitted
	//     {{.QuotationAmount}}   => New quotation amount
	//     {{.NewTimeline}}       => Stores the new timeline
	//     {{.ReviewLink}}        => Link for brand to view and review the quotation
	// -->

	data := map[string]interface{}{
		"BrandMemberName": brand.Name,
		"InfluencerName":  userName,
		"CollabTitle":     collab.Name,
		"SubmissionTime":  mytime.FormatPrettyIST(time.Now()),
		"QuotationAmount": application.Quotation,
		"NewTimeline":     mytime.FormatPrettyIST(time.UnixMilli(application.Timeline)),
		"ReviewLink":      fmt.Sprintf("%s/collaboration-details/%s", constants.TRENDLY_BRANDS_FE, collabId),
	}

	err = myemail.SendCustomHTMLEmailToMultipleRecipients(emails, templates.CollaborationQuotationResubmitted, templates.SubjectNewQuotationReceived, data)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "email sending issue"})
		return
	}

	contract := &trendlymodels.Contract{}
	err = contract.GetByCollab(collabId, userId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Collab fetch issue"})
		return
	}

	err = streamchat.SendSystemMessage(contract.StreamChannelID, fmt.Sprintf("Quotation for this collaboration has been updated by %s", userName))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Stream Send error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Successfully notified user for message"})
}
