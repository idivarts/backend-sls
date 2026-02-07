package monetize

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/myemail"
	"github.com/idivarts/backend-sls/pkg/mytime"
	"github.com/idivarts/backend-sls/pkg/streamchat"
	"github.com/idivarts/backend-sls/templates"
)

type ShipmentReq struct {
	TrackingID       string `json:"trackingId" binding:"required"`
	ShipmentProvider string `json:"shipmentProvider" binding:"required"`
	ExpectedDate     int64  `json:"expectedDate" binding:"required"`
}

func MarkShipment(c *gin.Context) {
	var req ShipmentReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Invalid request payload"})
		return
	}

	data, err := initializeData(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Failed to retrieve initialization data"})
		return
	}

	// 1. Update Contract Status and Shipment Info
	if data.Contract.Shipment == nil {
		data.Contract.Shipment = &trendlymodels.Shipment{}
	}
	data.Contract.Shipment.TrackingID = req.TrackingID
	data.Contract.Shipment.ShipmentProvider = req.ShipmentProvider
	data.Contract.Shipment.ExpectedDate = req.ExpectedDate
	data.Contract.Shipment.Status = "shipped"
	data.Contract.Status = 4 // Marking as Shipped

	err = data.Contract.Update(data.ContractID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Failed to update contract"})
		return
	}

	// 2. Fetch Influencer for notification
	influencer := &trendlymodels.User{}
	err = influencer.Get(data.Contract.UserID)
	if err != nil {
		log.Printf("Failed to get influencer: %v", err)
	}

	collab := &trendlymodels.Collaboration{}
	err = collab.Get(data.Contract.CollaborationID)
	collabName := "Your Collaboration"
	if err == nil {
		collabName = collab.Name
	}

	// 3. Send Push Notification to Influencer
	notif := &trendlymodels.Notification{
		Title:       "Package Shipped! 📦",
		Description: fmt.Sprintf("%s has shipped your package for %s", data.Brand.Name, collabName),
		TimeStamp:   time.Now().UnixMilli(),
		IsRead:      false,
		Type:        "shipment-marked",
		Data: &trendlymodels.NotificationData{
			CollaborationID: &data.Contract.CollaborationID,
			GroupID:         &data.ContractID,
		},
	}
	_, _, err = notif.Insert(trendlymodels.USER_COLLECTION, data.Contract.UserID)
	if err != nil {
		log.Printf("Failed to send push notification: %v", err)
	}

	// 4. Send Email to Influencer
	if influencer.Email != nil {
		expectedDateStr := mytime.FormatPrettyIST(time.UnixMilli(req.ExpectedDate))
		emailData := map[string]interface{}{
			"InfluencerName":   influencer.Name,
			"BrandName":        data.Brand.Name,
			"CollabTitle":      collabName,
			"TrackingID":       req.TrackingID,
			"ShipmentProvider": req.ShipmentProvider,
			"ExpectedDate":     expectedDateStr,
		}
		err = myemail.SendCustomHTMLEmail(*influencer.Email, templates.ShipmentMarked, templates.SubjectShipmentMarked, emailData)
		if err != nil {
			log.Printf("Failed to send shipment email: %v", err)
		}
	}

	// 5. Send Stream System Message
	streamMessage := fmt.Sprintf("📦 **Package Shipped!**\n\n**Agent:** %s\n**Tracking ID:** %s\n**Expected Delivery:** %s",
		req.ShipmentProvider, req.TrackingID, mytime.FormatPrettyIST(time.UnixMilli(req.ExpectedDate)))
	err = streamchat.SendSystemMessage(data.Contract.StreamChannelID, streamMessage)
	if err != nil {
		log.Printf("Failed to send stream message: %v", err)
	}

	c.JSON(http.StatusOK, gin.H{
		"message":  "Shipment marked successfully",
		"shipment": data.Contract.Shipment,
	})
}

func MarkShipmentDelivered(c *gin.Context) {
	var req struct{}
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Invalid request payload"})
		return
	}

	// The real implementation will go here in the future

	c.JSON(http.StatusOK, gin.H{"message": "This is a placeholder endpoint for Trendly Monetize APIs."})
}

func RequestShipment(c *gin.Context) {
	var req struct{}
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Invalid request payload"})
		return
	}

	// The real implementation will go here in the future

	c.JSON(http.StatusOK, gin.H{"message": "This is a placeholder endpoint for Trendly Monetize APIs."})
}

func MarkShipmentReceived(c *gin.Context) {
	var req struct{}
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Invalid request payload"})
		return
	}

	// The real implementation will go here in the future

	c.JSON(http.StatusOK, gin.H{"message": "This is a placeholder endpoint for Trendly Monetize APIs."})
}
