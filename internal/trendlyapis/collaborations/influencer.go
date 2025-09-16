package trendlyCollabs

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/myutil"
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

	// contract := trendlymodels.Contract{}
	// err := contract.Get(contractId)
	// if err != nil {
	// 	c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error fetching contract"})
	// 	return
	// }

	// collab := trendlymodels.Collaboration{}
	// err = collab.Get(contract.CollaborationID)
	// if err != nil {
	// 	c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error fetching collaboration"})
	// 	return
	// }

	// brand := trendlymodels.Brand{}
	// err = brand.Get(contract.BrandID)
	// if err != nil {
	// 	c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error fetching Brand"})
	// 	return
	// }

	user := trendlymodels.User{}
	err := user.Get(influenerId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error fetching user"})
		return
	}

	// // Send Push Notification
	// notif := &trendlymodels.Notification{
	// 	Title:       fmt.Sprintf("You are connected with brand"),
	// 	Description: "You can now find the details on the contract's menu",
	// 	IsRead:      false,
	// 	Data: &trendlymodels.NotificationData{
	// 		UserID: &managerId,
	// 	},
	// 	TimeStamp: time.Now().UnixMilli(),
	// 	Type:      "contract-started",
	// }
	// _, emails, err := notif.Insert(trendlymodels.BRAND_COLLECTION, collab.BrandID)
	// if err != nil {
	// 	c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	// 	return
	// }
	// _, _, err = notif.Insert(trendlymodels.USER_COLLECTION, contract.UserID)
	// if err != nil {
	// 	c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	// 	return
	// }

	// // Send Email notification

	// // 	<!--
	// //   Dynamic Variables:
	// // {{.RecipientName}}     => Name of the recipient (Brand or Influencer)
	// // {{.CollabTitle}}       => Title of the collaboration
	// // {{.StartDate}}         => Date when the collaboration was started
	// // {{.ContractLink}}      => Link to view the created contract
	// // -->

	// if user.Email != nil {
	// 	data := map[string]interface{}{
	// 		"RecipientName": user.Name,
	// 		"CollabTitle":   collab.Name,
	// 		"StartDate":     mytime.FormatPrettyIST(time.Now()),
	// 		"ContractLink":  fmt.Sprintf("%s/contract-details/%s", constants.TRENDLY_CREATORS_FE, contractId),
	// 	}
	// 	err = myemail.SendCustomHTMLEmail(*user.Email, templates.CollaborationStarted, templates.SubjectCollaborationStarted, data)
	// 	if err != nil {
	// 		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	// 		return
	// 	}
	// }
	// if len(emails) > 0 {
	// 	data := map[string]interface{}{
	// 		"RecipientName": brand.Name,
	// 		"CollabTitle":   collab.Name,
	// 		"StartDate":     mytime.FormatPrettyIST(time.Now()),
	// 		"ContractLink":  fmt.Sprintf("%s/contract-details/%s", constants.TRENDLY_BRANDS_FE, contractId),
	// 	}
	// 	err = myemail.SendCustomHTMLEmailToMultipleRecipients(emails, templates.CollaborationStarted, templates.SubjectCollaborationStarted, data)
	// 	if err != nil {
	// 		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	// 		return
	// 	}
	// }

	// // Send Stream Notification
	// err = streamchat.SendSystemMessage(contract.StreamChannelID, "Congratulations!! The contract has been started!\nYou can find the contract details on the contract menu")
	// if err != nil {
	// 	c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Stream Error"})
	// 	return
	// }

	c.JSON(http.StatusOK, gin.H{"message": "Successfully Notified for starting contract", "managerId": managerId, "influenerId": influenerId, "userEmail": user.Email})
}
