package trendlyCollabs

import (
	"fmt"
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

// End | request to end Contract
func EndContract(c *gin.Context) {
	userType := middlewares.GetUserType(c)
	if userType == "user" {
		requestToEndContract(c)
		return
	}

	contractId := c.Param(("contractId"))

	contract := trendlymodels.Contract{}
	err := contract.Get(contractId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error fetching contract"})
		return
	}

	collab := trendlymodels.Collaboration{}
	err = collab.Get(contract.CollaborationID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error fetching collaboration"})
		return
	}

	brand := trendlymodels.Brand{}
	err = brand.Get(contract.BrandID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error fetching Brand"})
		return
	}

	user := trendlymodels.User{}
	err = user.Get(contract.UserID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error fetching user"})
		return
	}

	// Send Push Notification
	notif := &trendlymodels.Notification{
		Title:       fmt.Sprintf("The contract has ended : %s", collab.Name),
		Description: "Thanks for working with trendly. We hope you had a great exprience",
		IsRead:      false,
		Data: &trendlymodels.NotificationData{
			CollaborationID: &contract.CollaborationID,
			UserID:          &contract.UserID,
			GroupID:         &contractId,
		},
		TimeStamp: time.Now().UnixMilli(),
		Type:      "contract-ended",
	}
	_, emails, err := notif.Insert(trendlymodels.BRAND_COLLECTION, collab.BrandID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	notif.Title = fmt.Sprintf("Contract ended! Please provide your feedback : %s", collab.Name)
	notif.Description = "We are waiting for you to provide feedback. Note: You wont be able to see your rating till you provide your feedback to brand"
	_, _, err = notif.Insert(trendlymodels.USER_COLLECTION, contract.UserID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Send Email notification

	if user.Email != nil {
		// 	{{.InfluencerName}}     => Name of the influencer
		// {{.BrandName}}          => Name of the brand that ended the contract
		// {{.CollabTitle}}        => Title of the collaboration
		// {{.EndDate}}            => Date when the contract was ended
		// {{.RatingLink}}         => Link for the influencer to rate the brand
		data := map[string]interface{}{
			"InfluencerName": user.Name,
			"BrandName":      brand.Name,
			"CollabTitle":    collab.Name,
			"EndDate":        mytime.FormatPrettyIST(time.Now()),
			"RatingLink":     fmt.Sprintf("%s/contract-details/%s", constants.TRENDLY_CREATORS_FE, contractId),
		}
		err = myemail.SendCustomHTMLEmail(*user.Email, templates.CollaborationEndedInfluencer, templates.SubjectContractEndedForInfluencer, data)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}
	if len(emails) > 0 {
		// 	{{.BrandMemberName}}   => Name of the brand team member receiving the email
		// {{.InfluencerName}}    => Name of the influencer whose contract was ended
		// {{.CollabTitle}}       => Title of the collaboration
		// {{.EndDate}}           => Date the contract was ended
		data := map[string]interface{}{
			"BrandMemberName": brand.Name,
			"InfluencerName":  user.Name,
			"CollabTitle":     collab.Name,
			"EndDate":         mytime.FormatPrettyIST(time.Now()),
		}
		err = myemail.SendCustomHTMLEmailToMultipleRecipients(emails, templates.CollaborationEndedBrand, templates.SubjectContractEndedForBrand, data)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}

	// Send Stream Notification
	err = streamchat.SendSystemMessage(contract.StreamChannelID, "Congratulations!! The contract has been ended!\nWe hope you had a great experience collaborating on Trendly")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Stream Error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Successfully Notified for ending contract"})
}
func requestToEndContract(c *gin.Context) {
	contractId := c.Param(("contractId"))

	contract := trendlymodels.Contract{}
	err := contract.Get(contractId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error fetching contract"})
		return
	}

	collab := trendlymodels.Collaboration{}
	err = collab.Get(contract.CollaborationID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error fetching collaboration"})
		return
	}

	brand := trendlymodels.Brand{}
	err = brand.Get(contract.BrandID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error fetching Brand"})
		return
	}

	user := trendlymodels.User{}
	err = user.Get(contract.UserID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error fetching user"})
		return
	}

	// Send Push Notification
	notif := &trendlymodels.Notification{
		Title:       fmt.Sprintf("Please end the contract : %s", collab.Name),
		Description: "Influencer has requested you to end the contract. Please review that",
		IsRead:      false,
		Data: &trendlymodels.NotificationData{
			CollaborationID: &contract.CollaborationID,
			UserID:          &contract.UserID,
			GroupID:         &contractId,
		},
		TimeStamp: time.Now().UnixMilli(),
		Type:      "contract-end-request",
	}
	_, emails, err := notif.Insert(trendlymodels.BRAND_COLLECTION, collab.BrandID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Send Email notification

	// 	<!--
	//   Dynamic Variables:
	// {{.BrandMemberName}}   => Name of the brand team member receiving the email
	// {{.InfluencerName}}    => Name of the influencer who sent the nudge
	// {{.CollabTitle}}       => Title of the collaboration
	// {{.PokeTime}}          => Timestamp when the poke was sent
	// {{.EndLink}}           => Link to end the contract and rate the influencer
	// -->

	if len(emails) > 0 {
		data := map[string]interface{}{
			"BrandMemberName": brand.Name,
			"InfluencerName":  user.Name,
			"CollabTitle":     collab.Name,
			"PokeTime":        mytime.FormatPrettyIST(time.Now()),
			"EndLink":         fmt.Sprintf("%s/contract-details/%s", constants.TRENDLY_BRANDS_FE, contractId),
		}
		err = myemail.SendCustomHTMLEmailToMultipleRecipients(emails, templates.CollaborationEndNudged, templates.SubjectNudgeToEndContract, data)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}

	// Send Stream Notification
	err = streamchat.SendSystemMessage(contract.StreamChannelID, fmt.Sprintf("To %s,\n%s has asked to end contract. If all contract deliverable is done, please end the contract.", brand.Name, user.Name))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Stream Error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Successfully requested to end contract"})
}

func GiveContractFeedback(c *gin.Context) {
	userType := middlewares.GetUserType(c)
	if userType != "user" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Only users can request this endpoint"})
		return
	}

	contractId := c.Param(("contractId"))

	contract := trendlymodels.Contract{}
	err := contract.Get(contractId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error fetching contract"})
		return
	}

	collab := trendlymodels.Collaboration{}
	err = collab.Get(contract.CollaborationID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error fetching collaboration"})
		return
	}

	brand := trendlymodels.Brand{}
	err = brand.Get(contract.BrandID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error fetching Brand"})
		return
	}

	user := trendlymodels.User{}
	err = user.Get(contract.UserID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error fetching user"})
		return
	}

	// Send Push Notification
	notif := &trendlymodels.Notification{
		Title:       fmt.Sprintf("%s has given you a rating", user.Name),
		Description: fmt.Sprintf("You have received a new rating for the collaboration %s", collab.Name),
		IsRead:      false,
		Data: &trendlymodels.NotificationData{
			CollaborationID: &contract.CollaborationID,
			UserID:          &contract.UserID,
			GroupID:         &contractId,
		},
		TimeStamp: time.Now().UnixMilli(),
		Type:      "feedback-given",
	}
	_, emails, err := notif.Insert(trendlymodels.BRAND_COLLECTION, collab.BrandID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Send Email notification

	// 	<!--
	//   Dynamic Variables:
	// {{.BrandMemberName}} => Name of the brand member receiving the email
	// {{.InfluencerName}}  => Name of the influencer who submitted the rating
	// {{.CollabTitle}}     => Title of the collaboration
	// {{.ContractLink}}    => Link to view the contract details including rating
	// -->

	if len(emails) > 0 {
		data := map[string]interface{}{
			"BrandMemberName": brand.Name,
			"InfluencerName":  user.Name,
			"CollabTitle":     collab.Name,
			"ContractLink":    fmt.Sprintf("%s/contract-details/%s", constants.TRENDLY_BRANDS_FE, contractId),
		}
		err = myemail.SendCustomHTMLEmailToMultipleRecipients(emails, templates.CollaborationRatedByInfluencer, templates.SubjectInfluencerRatedYou, data)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "Successfully given feedback"})
}
