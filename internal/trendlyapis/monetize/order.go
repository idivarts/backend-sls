package monetize

import (
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/myemail"
	"github.com/idivarts/backend-sls/pkg/payments"
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

	var orderID string
	var shortURL string
	reuseOrder := false

	// 1. Check if an order already exists and is still valid
	if data.Contract.Payment != nil && data.Contract.Payment.OrderID != "" {
		existingOrder, err := payments.FetchOrder(data.Contract.Payment.OrderID)
		if err == nil {
			status := existingOrder["status"].(string)
			// Statuses: created, attempted, paid
			if status == "created" || status == "attempted" {
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

		order, err := payments.CreateOrder(application.Quotation, oNotes)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Failed to Create Order"})
			return
		}
		orderID = order["id"].(string)

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
		OrderID:  orderID,
		Status:   "waiting_for_payment",
		ShortURL: shortURL,
		Amount:   application.Quotation,
	}
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

func GetOrder(c *gin.Context) {
	data, err := initializeData(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Failed to retrieve initialization data"})
		return
	}

	if data.Contract.Payment == nil || data.Contract.Payment.OrderID == "" {
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
