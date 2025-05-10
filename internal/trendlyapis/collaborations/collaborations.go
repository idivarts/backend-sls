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
	"github.com/idivarts/backend-sls/pkg/streamchat"
	"github.com/idivarts/backend-sls/templates"
)

// Starting a collab | Request to start
func StartContract(c *gin.Context) {
	userType := middlewares.GetUserType(c)
	if userType == "user" {
		requestToStart(c)
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
		Title:       fmt.Sprintf("The contract is started : %s", collab.Name),
		Description: "You can now find the details on the contract's menu",
		IsRead:      false,
		Data: &trendlymodels.NotificationData{
			CollaborationID: &contract.CollaborationID,
			UserID:          &contract.UserID,
			GroupID:         &contractId,
		},
		TimeStamp: time.Now().UnixMilli(),
		Type:      "contract-started",
	}
	_, emails, err := notif.Insert(trendlymodels.BRAND_COLLECTION, collab.BrandID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	_, _, err = notif.Insert(trendlymodels.USER_COLLECTION, contract.UserID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Send Email notification

	// 	<!--
	//   Dynamic Variables:
	// {{.RecipientName}}     => Name of the recipient (Brand or Influencer)
	// {{.CollabTitle}}       => Title of the collaboration
	// {{.StartDate}}         => Date when the collaboration was started
	// {{.ContractLink}}      => Link to view the created contract
	// -->

	if user.Email != nil {
		data := map[string]interface{}{
			"RecipientName": user.Name,
			"CollabTitle":   collab.Name,
			"StartDate":     time.Now().String(),
			"ContractLink":  fmt.Sprintf("%s/contract-details/%s", constants.TRENDLY_CREATORS_FE, contractId),
		}
		err = myemail.SendCustomHTMLEmail(*user.Email, templates.CollaborationStarted, templates.SubjectCollaborationStarted, data)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}
	if len(emails) > 0 {
		data := map[string]interface{}{
			"RecipientName": brand.Name,
			"CollabTitle":   collab.Name,
			"StartDate":     time.Now().String(),
			"ContractLink":  fmt.Sprintf("%s/contract-details/%s", constants.TRENDLY_BRANDS_FE, contractId),
		}
		err = myemail.SendCustomHTMLEmailToMultipleRecipients(emails, templates.CollaborationStarted, templates.SubjectCollaborationStarted, data)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}

	// Send Stream Notification
	err = streamchat.SendSystemMessage(contract.StreamChannelID, "Congratulations!! The contract has been started!\nYou can find the contract details on the contract menu")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Stream Error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Successfully Notified for starting contract"})
}

func requestToStart(c *gin.Context) {
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
		Title:       fmt.Sprintf("Please start the contract : %s", collab.Name),
		Description: fmt.Sprintf("%s has asked to start the contract. Please review that.", user.Name),
		IsRead:      false,
		Data: &trendlymodels.NotificationData{
			CollaborationID: &contract.CollaborationID,
			UserID:          &contract.UserID,
			GroupID:         &contractId,
		},
		TimeStamp: time.Now().UnixMilli(),
		Type:      "contract-start-request",
	}
	_, emails, err := notif.Insert(trendlymodels.BRAND_COLLECTION, collab.BrandID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Send Email notification

	// 	<!--
	//   Dynamic Variables:
	// {{.BrandMemberName}}    => Name of the brand team member receiving the email
	// {{.InfluencerName}}     => Name of the influencer who sent the poke
	// {{.CollabTitle}}        => Title of the collaboration
	// {{.PokeTime}}           => Timestamp when the poke was sent
	// {{.StartLink}}          => Link for the brand to start the collaboration
	// -->
	if len(emails) > 0 {
		data := map[string]interface{}{
			"BrandMemberName": brand.Name,
			"InfluencerName":  user.Name,
			"CollabTitle":     collab.Name,
			"PokeTime":        time.Now().String(),
			"StartLink":       fmt.Sprintf("%s/contract-details/%s", constants.TRENDLY_BRANDS_FE, contractId),
		}
		err = myemail.SendCustomHTMLEmailToMultipleRecipients(emails, templates.CollaborationStartRequested, templates.SubjectStartCollabReminderToBrand, data)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}

	// Send Stream Notification
	err = streamchat.SendSystemMessage(contract.StreamChannelID, fmt.Sprintf("To %s\nPlease start the contract if everything is discussed. %s is waiting on you to get started with his work", brand.Name, user.Name))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Stream Error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Successfully Notified for starting contract"})
}
