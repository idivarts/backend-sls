package monetize

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/payments"
)

func CreateAccount(c *gin.Context) {
	var req struct {
		Name    string              `json:"name" binding:"required"`
		PAN     string              `json:"pan" binding:"required"`
		Address payments.AddressReq `json:"address" binding:"required"`
		Bank    payments.BankReq    `json:"bank" binding:"required"`
	}

	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Invalid request payload"})
		return
	}

	userId, b := middlewares.GetUserId(c)
	if !b {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized", "message": "User not authenticated"})
		return
	}
	user := middlewares.GetUserModel(c)

	// The real implementation will go here in the future
	account, stakeholder, err := payments.CreateLinkedAccount(payments.CreateAccountReq{
		UserId:  userId,
		Name:    req.Name,
		Email:   *user.Email,
		Phone:   *user.PhoneNumber,
		Address: req.Address,
		PAN:     req.PAN,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Failed to create account"})
		return
	}

	product, err := payments.CreataOrUpdateProduct(account.ID, req.Bank)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Failed to create/update bank details"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":       "Account and Bank Details created successfully",
		"accountId":     account.ID,
		"stakeholderId": stakeholder.ID,
		"product":       product,
	})
}

func GetAccountStatus(c *gin.Context) {
	var req struct{}
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Invalid request payload"})
		return
	}

	user := middlewares.GetUserModel(c)

	product, err := payments.GetProduct(user.Name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Failed to fetch account status"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"product": product, "message": "Account status fetched successfully"})
}

func UpdateBankDetails(c *gin.Context) {
	var req payments.BankReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Invalid request payload"})
		return
	}

	userId, _ := middlewares.GetUserId(c)
	user := middlewares.GetUserModel(c)

	if user.KYC == nil || user.KYC.AccountID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Razorpay account not found for this user"})
		return
	}

	// 1. Update Bank Details in Razorpay Product
	product, err := payments.CreataOrUpdateProduct(user.KYC.AccountID, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Failed to update bank details in Razorpay"})
		return
	}

	// 2. Update user.KYC.BankDetails in Firestore
	user.KYC.BankDetails = &trendlymodels.BankDetails{
		AccountNumber:   req.AccountNumber,
		IFSC:            req.IFSC,
		BeneficiaryName: req.BenificiaryName,
	}

	_, err = user.Insert(userId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Failed to update user records"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Bank details updated successfully",
		"product": product,
	})
}

func UpdateAddress(c *gin.Context) {
	var req struct{}
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Invalid request payload"})
		return
	}

	// The real implementation will go here in the future

	c.JSON(http.StatusOK, gin.H{"message": "This is a placeholder endpoint for Trendly Monetize APIs."})
}
