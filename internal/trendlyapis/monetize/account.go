package monetize

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/payments"
)

func CreateAccount(c *gin.Context) {
	var req struct {
		Name            string              `json:"name" binding:"required"`
		PAN             string              `json:"pan" binding:"required"`
		Address         payments.AddressReq `json:"address" binding:"required"`
		Bank            payments.BankReq    `json:"bank" binding:"required"`
		ReCreateAccount bool                `json:"reCreateAccount"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Invalid request payload"})
		return
	}

	userId, b := middlewares.GetUserId(c)
	if !b {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized", "message": "User not authenticated"})
		return
	}
	user := middlewares.GetUserModel(c)

	// 1. Handle Re-creation logic (Start Fresh)
	if req.ReCreateAccount && user.KYC != nil && user.KYC.AccountID != "" {
		log.Printf("Re-creating account for user %s. Deleting old account: %s", userId, user.KYC.AccountID)

		// Attempt to delete old account in Razorpay (Best effort)
		_, err := payments.DeleteAccount(user.KYC.AccountID)
		if err != nil {
			log.Printf("Warning: Failed to delete old Razorpay account %s: %v", user.KYC.AccountID, err)
		}

		// Reset KYC records in internal state
		user.KYC.AccountID = ""
		user.KYC.StakeHolderID = ""
		user.KYC.ProductID = ""
		user.KYC.Status = "not_started"
		user.KYC.Reason = nil

		// Persist the reset before continuing
		_, err = user.Insert(userId)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Failed to reset KYC records for re-creation"})
			return
		}
	}

	// 2. Create Linked Account
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

	// 3. Create/Update Product
	product, err := payments.CreataOrUpdateProduct(account.ID, req.Bank)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Failed to create/update bank details"})
		return
	}

	// 4. Update user model with new credentials
	if user.KYC == nil {
		user.KYC = &trendlymodels.KYC{}
	}
	user.KYC.AccountID = account.ID
	user.KYC.StakeHolderID = stakeholder.ID
	user.KYC.ProductID = product.ID
	user.KYC.Status = "in_progress" // Or based on Razorpay status
	user.KYC.BankDetails = &trendlymodels.BankDetails{
		AccountNumber:   req.Bank.AccountNumber,
		IFSC:            req.Bank.IFSC,
		BeneficiaryName: req.Bank.BenificiaryName,
	}
	user.KYC.PANDetails = &trendlymodels.PANDetails{
		PANNumber:    req.PAN,
		NameAsPerPAN: req.Name,
	}
	user.KYC.CurrentAddress = &trendlymodels.CurrentAddress{
		Street:     req.Address.Street,
		City:       req.Address.City,
		State:      req.Address.State,
		PostalCode: req.Address.PostalCode,
	}

	_, err = user.Insert(userId)
	if err != nil {
		log.Printf("Failed to update user KYC records after creation: %v", err)
	}

	c.JSON(http.StatusOK, gin.H{
		"message":       "Account and Bank Details created successfully",
		"accountId":     account.ID,
		"stakeholderId": stakeholder.ID,
		"product":       product,
	})
}

func GetAccountStatus(c *gin.Context) {
	user := middlewares.GetUserModel(c)

	// 1. Check if the user has even started the process
	if user.KYC == nil || user.KYC.AccountID == "" {
		c.JSON(http.StatusOK, gin.H{
			"message":   "Account onboarding has not been started.",
			"kycStatus": "not_started",
		})
		return
	}

	// 2. Fetch comprehensive details from Razorpay
	// This helps developers debug and users understand verification bottlenecks
	account, err := payments.FetchLinkedAccount(user.KYC.AccountID)
	if err != nil {
		log.Printf("Error fetching Razorpay account (%s): %v", user.KYC.AccountID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Failed to fetch account status from Razorpay"})
		return
	}

	product, err := payments.GetProduct(user.KYC.AccountID)
	if err != nil {
		log.Printf("Warning: Failed to fetch product status for %s: %v", user.KYC.AccountID, err)
	}

	var stakeholder *payments.RPStakeholder
	if user.KYC.StakeHolderID != "" {
		stakeholder, err = payments.FetchStakeholder(user.KYC.AccountID, user.KYC.StakeHolderID)
		if err != nil {
			log.Printf("Warning: Failed to fetch stakeholder status for %s: %v", user.KYC.StakeHolderID, err)
		}
	}

	// 3. Keep Firestore status in sync with Razorpay (Self-healing)
	if account.Status != "" && account.Status != user.KYC.Status {
		log.Printf("Syncing KYC status for user %s: Razorpay(%s) vs Firestore(%s)", user.Name, account.Status, user.KYC.Status)
		user.KYC.Status = account.Status

		userId, _ := middlewares.GetUserId(c)
		_, updateErr := user.Insert(userId)
		if updateErr != nil {
			log.Printf("Failed to sync KYC status to Firestore: %v", updateErr)
		}
	}

	// 4. Return detailed response
	c.JSON(http.StatusOK, gin.H{
		"message":     "Account status fetched successfully",
		"account":     account,
		"stakeholder": stakeholder,
		"product":     product,
		"kycStatus":   user.KYC.Status,
	})
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
	var req payments.AddressReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Invalid request payload"})
		return
	}

	userId, _ := middlewares.GetUserId(c)
	user := middlewares.GetUserModel(c)

	if user.KYC == nil || user.KYC.AccountID == "" || user.KYC.StakeHolderID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Razorpay account or stakeholder not found for this user"})
		return
	}

	// 1. Prepare data for Razorpay Update
	pan := ""
	if user.KYC.PANDetails != nil {
		pan = user.KYC.PANDetails.PANNumber
	}

	email := ""
	if user.Email != nil {
		email = *user.Email
	}

	phone := ""
	if user.PhoneNumber != nil {
		phone = *user.PhoneNumber
	}

	updateReq := payments.CreateAccountReq{
		Name:    user.Name,
		Email:   email,
		Phone:   phone,
		UserId:  userId,
		Address: req,
		PAN:     pan,
	}

	// 2. Sync with Razorpay
	account, stakeholder, err := payments.UpdateAccountAndStakeHolderAddress(user.KYC.AccountID, user.KYC.StakeHolderID, updateReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Failed to update address in Razorpay"})
		return
	}

	// 3. Update user.KYC.CurrentAddress in Firestore
	user.KYC.CurrentAddress = &trendlymodels.CurrentAddress{
		Street:     req.Street,
		City:       req.City,
		State:      req.State,
		PostalCode: req.PostalCode,
	}

	_, err = user.Insert(userId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Failed to update user records"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":     "Address updated successfully",
		"account":     account,
		"stakeholder": stakeholder,
	})
}
