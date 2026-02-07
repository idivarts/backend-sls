package monetize

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/constants"
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

type ShipmentDeliveredReq struct {
	ScreenshotURL string `json:"screenshotUrl" binding:"required"`
	Notes         string `json:"notes"`
}

func MarkShipmentDelivered(c *gin.Context) {
	var req ShipmentDeliveredReq
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
	data.Contract.Shipment.Status = "delivered"
	data.Contract.Shipment.Notes = req.Notes
	if req.ScreenshotURL != "" {
		data.Contract.Shipment.PackageScreenshots = append(data.Contract.Shipment.PackageScreenshots, req.ScreenshotURL)
	}
	data.Contract.Status = 5 // Marking as Delivered

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
		Title:       "Product Delivered! 🏠",
		Description: fmt.Sprintf("Proof of delivery uploaded for %s. Please confirm receipt.", collabName),
		TimeStamp:   time.Now().UnixMilli(),
		IsRead:      false,
		Type:        "shipment-delivered",
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
		emailData := map[string]interface{}{
			"InfluencerName": influencer.Name,
			"BrandName":      data.Brand.Name,
			"CollabTitle":    collabName,
			"ScreenshotURL":  req.ScreenshotURL,
			"Notes":          req.Notes,
			"ConfirmLink":    fmt.Sprintf("%s/contracts/%s", constants.TRENDLY_CREATORS_FE, data.ContractID),
		}
		err = myemail.SendCustomHTMLEmail(*influencer.Email, templates.ShipmentDelivered, templates.SubjectShipmentDelivered, emailData)
		if err != nil {
			log.Printf("Failed to send shipment delivered email: %v", err)
		}
	}

	// 5. Send Stream System Message
	streamMessage := fmt.Sprintf("🏠 **Product Delivered!**\n\nBrand has uploaded proof of delivery. Please confirm once you've received the package.")
	if req.Notes != "" {
		streamMessage += fmt.Sprintf("\n\n**Notes from Brand:** %s", req.Notes)
	}
	err = streamchat.SendSystemMessage(data.Contract.StreamChannelID, streamMessage)
	if err != nil {
		log.Printf("Failed to send stream message: %v", err)
	}

	c.JSON(http.StatusOK, gin.H{
		"message":  "Shipment marked as delivered successfully",
		"shipment": data.Contract.Shipment,
	})
}

func RequestShipment(c *gin.Context) {
	data, err := initializeData(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Failed to retrieve initialization data"})
		return
	}

	// 1. Fetch Influencer (Sender)
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

	// 2. Send Push Notification to Brand
	notif := &trendlymodels.Notification{
		Title:       "Shipment Requested 📦",
		Description: fmt.Sprintf("%s is waiting for the shipment for %s", influencer.Name, collabName),
		TimeStamp:   time.Now().UnixMilli(),
		IsRead:      false,
		Type:        "shipment-request",
		Data: &trendlymodels.NotificationData{
			CollaborationID: &data.Contract.CollaborationID,
			GroupID:         &data.ContractID,
		},
	}
	_, brandEmails, err := notif.Insert(trendlymodels.BRAND_COLLECTION, data.Contract.BrandID)
	if err != nil {
		log.Printf("Failed to insert notification: %v", err)
	}

	// 3. Send Email to Brand members
	if len(brandEmails) > 0 {
		emailData := map[string]interface{}{
			"BrandMemberName": data.Brand.Name,
			"InfluencerName":  influencer.Name,
			"CollabTitle":     collabName,
			"ShipmentLink":    fmt.Sprintf("%s/contracts/%s", constants.TRENDLY_BRANDS_FE, data.ContractID), // Example link, might need adjustment if constants available
		}
		err = myemail.SendCustomHTMLEmailToMultipleRecipients(brandEmails, templates.ShipmentRequested, templates.SubjectShipmentRequested, emailData)
		if err != nil {
			log.Printf("Failed to send shipment request email: %v", err)
		}
	}

	// 4. Send Stream System Message
	streamMessage := fmt.Sprintf("📢 **Update:** We've notified %s that you're waiting for the shipment for '**%s**'.\n\nWe'll keep you posted once they update the tracking details!", data.Brand.Name, collabName)
	err = streamchat.SendSystemMessage(data.Contract.StreamChannelID, streamMessage)
	if err != nil {
		log.Printf("Failed to send stream message: %v", err)
	}

	c.JSON(http.StatusOK, gin.H{"message": "Brand notified successfully for shipment"})
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
