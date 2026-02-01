package monetize

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/payments"
)

func CreateOrder(c *gin.Context) {
	// var req struct{}
	// if err := c.ShouldBind(&req); err != nil {
	// 	c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Invalid request payload"})
	// 	return
	// }
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

	oData := map[string]interface{}{
		"amount":   application.Quotation * 100, // amount in smallest currency unit (e.g. 50000 paise = ₹500)
		"currency": "INR",                       // ISO currency code
		"notes": map[string]interface{}{
			"contractId":      data.ContractID,
			"brandId":         data.Contract.BrandID,
			"collaborationId": data.Contract.CollaborationID,
			"userId":          data.Contract.UserID,
		},
	}

	order, err := payments.Client.Order.Create(oData, nil)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Failed to Create Order"})
		return
	}

	data.Contract.Payment = &trendlymodels.Payment{
		OrderID:   order["id"].(string),
		Status:    "",
		PaymentID: "",
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Order Created successfully",
		"orderId": data.Contract.Payment.OrderID,
		"order":   order,
	})
}

func GetOrder(c *gin.Context) {
	var req struct{}
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Invalid request payload"})
		return
	}

	// The real implementation will go here in the future

	c.JSON(http.StatusOK, gin.H{"message": "This is a placeholder endpoint for Trendly Monetize APIs."})
}
