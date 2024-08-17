package crowdychat

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Struct for form data
type ProfileUpdateRequest struct {
	Name        string `form:"name" binding:"required"`
	Image       string `form:"image"`
	OldPassword string `form:"oldPassword"`
	NewPassword string `form:"newPassword"`
}

// Struct for the response structure
type ProfileUpdateResponse struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Image string `json:"image"`
	Email string `json:"email"`
}

func UpdateProfile(c *gin.Context) {
	// Initialize the request struct
	var request ProfileUpdateRequest

	// Bind form data to the struct and validate
	if err := c.ShouldBind(&request); err != nil {
		// Return validation error if any field is missing or invalid
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Additional custom validation for passwords
	if request.OldPassword != "" && request.NewPassword == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "New password is required if old password is provided"})
		return
	}

	// Process image file upload if provided
	imageFile, err := c.FormFile("image")
	var imageURL string
	if err == nil {
		// Here you would save the image file and generate the URL
		imageURL = "path_to_saved_image" // Replace with actual path
	} else {
		// Handle the case where no image is uploaded
		imageURL = imageFile.Filename
	}

	// Placeholder for actual data update logic
	response := ProfileUpdateResponse{
		ID:    "942",
		Name:  request.Name,
		Image: imageURL,
		Email: "Antonetta25@hotmail.com",
	}

	c.JSON(http.StatusOK, Response{
		Data:    response,
		Message: "Successfully edited profile",
	})
}
