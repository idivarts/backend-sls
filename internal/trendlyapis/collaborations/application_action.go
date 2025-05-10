package trendlyCollabs

import (
	"fmt"
	"net/http"
	"time"

	stream_chat "github.com/GetStream/stream-chat-go/v5"
	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/constants"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/internal/trendlyapis"
	"github.com/idivarts/backend-sls/pkg/myemail"
	"github.com/idivarts/backend-sls/pkg/mytime"
	"github.com/idivarts/backend-sls/pkg/streamchat"
	"github.com/idivarts/backend-sls/templates"
)

// accept | reject | revise
func ApplicationAction(c *gin.Context) {
	userType := middlewares.GetUserType(c)
	if userType != "manager" {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "Only Managers can call this endpoint"})
		return
	}
	managerId, b := middlewares.GetUserId(c)
	if !b {
		c.JSON(http.StatusBadRequest, gin.H{"message": "User not found"})
		return
	}

	collabId := c.Param("collabId")
	userId := c.Param("userId")
	action := c.Param("action")

	collab := &trendlymodels.Collaboration{}
	err := collab.Get(collabId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	if action == "accept" {
		channelName := collab.Name
		res, err := trendlyapis.CreateChannel(managerId, trendlyapis.ICreateChannel{
			Name:            &channelName,
			UserID:          userId,
			CollaborationID: collabId,
		})
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
			return
		}

		err = notifyApplicationAccepted(userId, *collab, res.Contract, res.Channel)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "Channel Created", "channel": res.Channel, "contractId": res.Contract.StreamChannelID, "contract": res.Contract})
		return
	} else if action == "reject" {
		c.JSON(http.StatusOK, gin.H{"message": "Successfully rejected application"})
		return
	} else if action == "revise" {
		err = notifyToReviseApplication(userId, collabId, *collab)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Successfully sent revision request"})
		return
	}
	c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid request. Please provide correct action"})
}
func notifyApplicationAccepted(userId string, collab trendlymodels.Collaboration, contract trendlymodels.Contract, channel stream_chat.Channel) error {
	user := trendlymodels.User{}
	err := user.Get(userId)
	if err != nil {
		return err
	}
	brand := trendlymodels.Brand{}
	err = brand.Get(collab.BrandID)
	if err != nil {
		return err
	}

	// Push Notification
	notif := &trendlymodels.Notification{
		Title:       fmt.Sprintf("Your application for %s is accepted", collab.Name),
		Description: "Start messaging the brand to get your contract started.",
		IsRead:      false,
		Data: &trendlymodels.NotificationData{
			CollaborationID: &collab.Name,
			UserID:          &userId,
			GroupID:         &contract.StreamChannelID,
		},
		TimeStamp: time.Now().UnixMilli(),
		Type:      "application-accepted",
	}
	_, _, err = notif.Insert(trendlymodels.USER_COLLECTION, userId)
	if err != nil {
		return err
	}

	// Email Notification
	// 	<!--
	//   Dynamic Variables:
	//     {{.InfluencerName}}    => Name of the influencer
	//     {{.BrandName}}         => Name of the brand that accepted the application
	//     {{.CollabTitle}}       => Title of the collaboration
	//     {{.AcceptanceTime}}    => Timestamp of acceptance
	//     {{.CollabLink}}        => Link to view the collaboration details
	// -->

	data := map[string]interface{}{
		"InfluencerName": user.Name,
		"BrandName":      brand.Name,
		"CollabTitle":    collab.Name,
		"AcceptanceTime": mytime.FormatPrettyIST(time.Now()),
		"CollabLink":     fmt.Sprintf("%s/messages?channelId=%s", constants.TRENDLY_CREATORS_FE, contract.StreamChannelID),
	}

	if user.Email != nil {
		err = myemail.SendCustomHTMLEmail(*user.Email, templates.ApplicationAccepted, templates.SubjectApplicationAccepted, data)
		if err != nil {
			return err
		}
	}

	// Stream Notification
	err = streamchat.SendSystemMessage(channel.ID, fmt.Sprintf("Use this channel to get to understand each other before you are a new contract for this collaboration : %s", collab.Name))
	if err != nil {
		return err
	}

	return nil
}

func notifyToReviseApplication(userId, collabId string, collab trendlymodels.Collaboration) error {
	user := trendlymodels.User{}
	err := user.Get(userId)
	if err != nil {
		return err
	}
	brand := trendlymodels.Brand{}
	err = brand.Get(collab.BrandID)
	if err != nil {
		return err
	}

	contract := trendlymodels.Contract{}
	err = contract.GetByCollab(collabId, userId)
	if err != nil {
		return err
	}

	// Push Notification
	notif := &trendlymodels.Notification{
		Title:       fmt.Sprintf("Revise your quotation for %s", collab.Name),
		Description: "Please open your contract details and revise your quotation",
		IsRead:      false,
		Data: &trendlymodels.NotificationData{
			CollaborationID: &collab.Name,
			UserID:          &userId,
			GroupID:         &contract.StreamChannelID,
		},
		TimeStamp: time.Now().UnixMilli(),
		Type:      "revise-quotation",
	}
	_, _, err = notif.Insert(trendlymodels.USER_COLLECTION, userId)
	if err != nil {
		return err
	}

	// 	<!--
	//   Dynamic Variables:
	//     {{.InfluencerName}}      => Name of the influencer
	//     {{.BrandName}}           => Name of the brand requesting revision
	//     {{.CollabTitle}}         => Title of the collaboration
	//     {{.RevisionRequestTime}} => Timestamp of the revision request
	//     {{.RevisionNote}}        => Optional note or reason provided by the brand
	//     {{.ReviseLink}}          => Link for the influencer to review and revise the quotation
	// -->

	data := map[string]interface{}{
		"InfluencerName":      user.Name,
		"BrandName":           brand.Name,
		"CollabTitle":         collab.Name,
		"RevisionRequestTime": mytime.FormatPrettyIST(time.Now()),
		// "RevisionNote":""
		"CollabLink": fmt.Sprintf("%s/contract-details/%s", constants.TRENDLY_CREATORS_FE, contract.StreamChannelID),
	}

	if user.Email != nil {
		err = myemail.SendCustomHTMLEmail(*user.Email, templates.CollaborationQuotationRequested, templates.SubjectQuotationRevisionRequested, data)
		if err != nil {
			return err
		}
	}

	// Stream Notification
	err = streamchat.SendSystemMessage(contract.StreamChannelID, fmt.Sprintf("Hello %s,\nPlease revise your quotation from the application info screen so that a contract can be started", user.Name))
	if err != nil {
		return err
	}

	return nil
}
