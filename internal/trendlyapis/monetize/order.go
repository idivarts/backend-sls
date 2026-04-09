package monetize

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/myemail"
	"github.com/idivarts/backend-sls/pkg/myutil"
	"github.com/idivarts/backend-sls/pkg/payments"
	"github.com/idivarts/backend-sls/pkg/streamchat"
	"github.com/idivarts/backend-sls/templates"
)

func CreateOrder(c *gin.Context) {
	data, err := initializeData(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Failed to retrieve initialization data"})
		return
	}

	application := &trendlymodels.Application{}
	err = application.Get(data.Contract.CollaborationID, data.Contract.UserID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Cant find Application"})
		return
	}

	if application.Quotation <= 0 {
		_ = startBarterContract(c, data, application)
		return
	}

	user := &trendlymodels.User{}
	err = user.Get(data.Contract.UserID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Cant find User"})
		return
	}

	if !user.IsKYCDone || user.KYC == nil || user.KYC.AccountID == "" || user.KYC.Status != trendlymodels.KYCStatusActivated {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Influencer is not KYC verified", "message": "Influencer is not KYC verified"})
		return
	}

	var orderID string
	var transferID string
	var shortURL string
	reuseOrder := false

	// 1. Check if an order already exists and is still valid
	if data.Contract.Payment != nil && data.Contract.Payment.OrderID != "" {
		existingOrder, err := payments.FetchOrder(data.Contract.Payment.OrderID)
		if err == nil {
			// Statuses: created, attempted, paid
			if existingOrder.Status == "created" || existingOrder.Status == "attempted" {
				orderID = data.Contract.Payment.OrderID
				shortURL = data.Contract.Payment.ShortURL
				reuseOrder = true
			}
		}
	}

	// 2. Prepare Notification to get Brand Emails
	notif := &trendlymodels.Notification{
		Title:       "Payment Order Created",
		Description: "A payment order has been created for your collaboration. Please complete the pre-payment.",
		TimeStamp:   time.Now().UnixMilli(),
		IsRead:      false,
		Type:        "payment-order-created",
		Data: &trendlymodels.NotificationData{
			CollaborationID: &data.Contract.CollaborationID,
			GroupID:         &data.ContractID,
		},
	}
	_, brandEmails, err := notif.Insert(trendlymodels.BRAND_COLLECTION, data.Contract.BrandID)
	if err != nil {
		log.Printf("Failed to insert notification: %v", err)
	}

	if !reuseOrder {
		// 3. Create a new Order
		oNotes := map[string]interface{}{
			"contractId":      data.ContractID,
			"brandId":         data.Contract.BrandID,
			"collaborationId": data.Contract.CollaborationID,
			"userId":          data.Contract.UserID,
		}

		order, err := payments.CreateOrder(application.Quotation, oNotes, []payments.OrderTransfer{
			{
				Account:    user.KYC.AccountID,
				AmountInRs: application.Quotation,
				Currency:   "INR",
				OnHold:     myutil.BoolPtr(true),
			},
		})
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Failed to Create Order"})
			return
		}
		if len(order.Transfers) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "No transfers found", "message": "Failed to Create Order with Transfers"})
			return
		}

		orderTransfer := order.Transfers[0]
		if orderTransfer.Error != nil && (orderTransfer.Error.ID != nil || orderTransfer.Error.Code != nil) {
			c.JSON(http.StatusBadRequest, gin.H{"error": orderTransfer.Error, "transfer": orderTransfer, "message": "Failed to Create Transfer"})
			return
		}

		orderID = order.ID
		transferID = orderTransfer.ID

		// 4. Create a Payment Link for the email
		customerEmail := ""
		if len(brandEmails) > 0 {
			customerEmail = brandEmails[0]
		}
		customerPhone := ""
		if data.Brand.Profile != nil && data.Brand.Profile.PhoneNumber != nil {
			customerPhone = *data.Brand.Profile.PhoneNumber
		}

		customer := payments.Customer{
			Name:        data.Brand.Name,
			Email:       customerEmail,
			PhoneNumber: customerPhone,
		}
		_, shortURL, err = payments.CreatePaymentLink(application.Quotation, customer, oNotes)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Failed to generate payment link"})
			return
		}
	}

	// 5. Update the Contract with Order/Payment details
	data.Contract.Payment = &trendlymodels.Payment{
		OrderID:    orderID,
		TransferID: transferID,
		Status:     trendlymodels.PaymentStatusWaitingForPayment,
		ShortURL:   shortURL,
		Amount:     application.Quotation,
	}
	data.Contract.Status = trendlymodels.ContractStatusOrderCreated
	err = data.Contract.Update(data.ContractID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Failed to update Contract with Order details"})
		return
	}

	// 6. Send email to the Brand members
	collab := &trendlymodels.Collaboration{}
	err = collab.Get(data.Contract.CollaborationID)
	collabTitle := "Your Collaboration"
	if err == nil {
		collabTitle = collab.Name
	}

	emailData := map[string]interface{}{
		"RecipientName": data.Brand.Name,
		"CollabTitle":   collabTitle,
		"Amount":        application.Quotation,
		"PaymentLink":   shortURL,
	}

	if len(brandEmails) > 0 {
		err = myemail.SendCustomHTMLEmailToMultipleRecipients(brandEmails, templates.PaymentOrderCreated, templates.SubjectPaymentOrderCreated, emailData)
		if err != nil {
			log.Printf("Failed to send payment email: %v", err)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"message":  "Order Created successfully",
		"orderId":  orderID,
		"shortUrl": shortURL,
	})
}

// contractStatusAfterFunding matches handleOrderPaid in webhook_orders (post-payment workflow).
func contractStatusAfterFunding(collab *trendlymodels.Collaboration) trendlymodels.ContractStatus {
	if collab.PromotionSubject == trendlymodels.PromotionSubjectPhysicalProduct {
		return trendlymodels.ContractStatusShipmentPending
	}
	return trendlymodels.ContractStatusDeliverablePending
}

// startBarterContract skips Razorpay for zero-quotation (barter) collabs and moves the contract forward
// as if payment succeeded, without charging the brand.
func startBarterContract(c *gin.Context, data *struct {
	ContractID string
	Contract   *trendlymodels.Contract
	Brand      *trendlymodels.Brand
}, _ *trendlymodels.Application) error {
	collab := &trendlymodels.Collaboration{}
	if err := collab.Get(data.Contract.CollaborationID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Cant find Collaboration"})
		return err
	}

	nextStatus := contractStatusAfterFunding(collab)
	data.Contract.Payment = &trendlymodels.Payment{
		Status: trendlymodels.PaymentStatusPaid,
		Amount: 0,
	}
	data.Contract.Status = nextStatus
	if err := data.Contract.Update(data.ContractID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Failed to update contract"})
		return err
	}

	influencer := &trendlymodels.User{}
	_ = influencer.Get(data.Contract.UserID)
	influencerName := influencer.Name
	if influencerName == "" {
		influencerName = "Creator"
	}
	collabName := collab.Name
	if collabName == "" {
		collabName = "Your Collaboration"
	}

	notifBrand := &trendlymodels.Notification{
		Title:       "Collaboration is live",
		Description: fmt.Sprintf("%s is now active. No payment was required (barter). The creator can start the next steps.", collabName),
		TimeStamp:   time.Now().UnixMilli(),
		IsRead:      false,
		Type:        "barter-collaboration-started",
		Data: &trendlymodels.NotificationData{
			CollaborationID: &data.Contract.CollaborationID,
			GroupID:         &data.ContractID,
		},
	}
	if _, _, err := notifBrand.Insert(trendlymodels.BRAND_COLLECTION, data.Contract.BrandID); err != nil {
		log.Printf("barter start: brand notification: %v", err)
	}

	notifInfluencer := &trendlymodels.Notification{
		Title:       "Collaboration is live",
		Description: fmt.Sprintf("Your barter collaboration %s with %s is active. You can start working on the next steps.", collabName, data.Brand.Name),
		TimeStamp:   time.Now().UnixMilli(),
		IsRead:      false,
		Type:        "barter-collaboration-started",
		Data: &trendlymodels.NotificationData{
			CollaborationID: &data.Contract.CollaborationID,
			GroupID:         &data.ContractID,
		},
	}
	if _, _, err := notifInfluencer.Insert(trendlymodels.USER_COLLECTION, data.Contract.UserID); err != nil {
		log.Printf("barter start: influencer notification: %v", err)
	}

	streamMessage := fmt.Sprintf("🤝 **Barter collaboration is live**\n\nNo payment was required. **%s** and **%s** can move forward with the collaboration. 🚀", data.Brand.Name, influencerName)
	if err := streamchat.SendSystemMessage(data.Contract.StreamChannelID, streamMessage); err != nil {
		log.Printf("barter start: stream message: %v", err)
	}

	c.JSON(http.StatusOK, gin.H{
		"message":  "Barter collaboration started",
		"barter":   true,
		"status":   int(nextStatus),
		"orderId":  "",
		"shortUrl": "",
	})
	return nil
}

func GetOrder(c *gin.Context) {
	data, err := initializeData(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Failed to retrieve initialization data"})
		return
	}

	if data.Contract.Payment == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No order found", "message": "No payment order has been created for this contract yet"})
		return
	}

	if data.Contract.Payment.OrderID == "" && data.Contract.Payment.Status == trendlymodels.PaymentStatusPaid && data.Contract.Payment.Amount == 0 {
		c.JSON(http.StatusOK, gin.H{
			"message": "Barter collaboration — no Razorpay order",
			"barter":  true,
			"payment": data.Contract.Payment,
		})
		return
	}

	if data.Contract.Payment.OrderID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No order found", "message": "No payment order has been created for this contract yet"})
		return
	}

	order, err := payments.FetchOrder(data.Contract.Payment.OrderID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Failed to fetch order details from Razorpay"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Order found",
		"order":   order,
		"payment": data.Contract.Payment,
	})
}
