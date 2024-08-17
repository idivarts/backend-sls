package crowdychat

import (
	"net/http"
	"path/filepath"

	"github.com/gin-gonic/gin"
)

// Request structure to bind the incoming query parameters
type _GetOrganizationParams struct {
	Start int `form:"start" binding:"required"`
	Count int `form:"count" binding:"required"`
}

// _GetOrganizationsResponse structure to return the desired JSON response
type _GetOrganizationsResponse struct {
	Start    int                  `json:"start"`
	MoreData bool                 `json:"moreData"`
	Content  []_OrganizationShort `json:"content"`
}

// _OrganizationShort structure represents the content of each person in the response
type _OrganizationShort struct {
	ID    string `json:"id"`
	Image string `json:"image"`
	Name  string `json:"name"`
}

type _Organization struct {
	OrganizationToken string `json:"organizationToken,omitempty"`
	ID                string `json:"id"`
	Name              string `json:"name"`
	Image             string `json:"image"`
	Description       string `json:"description"`
	Industry          string `json:"industry"`
	Website           string `json:"website"`
	APIKeyAvailable   bool   `json:"apiKeyAvailable"`
}

func GetOrganizations(c *gin.Context) {
	// Bind query parameters
	var queryParams _GetOrganizationParams
	if err := c.ShouldBindQuery(&queryParams); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Mocked data for demonstration purposes
	content := []_OrganizationShort{
		{ID: "429", Image: "https://cdn.fakercloud.com/avatars/robbschiller_128.jpg", Name: "Connie Wisozk"},
		{ID: "724", Image: "https://cdn.fakercloud.com/avatars/BillSKenney_128.jpg", Name: "Cecelia Schmeler"},
		{ID: "960", Image: "https://cdn.fakercloud.com/avatars/gcmorley_128.jpg", Name: "Ignacio Zieme"},
	}

	// Determine if more data is available
	moreData := len(content) > queryParams.Count

	// Prepare response
	response := _GetOrganizationsResponse{
		Start:    queryParams.Start,
		MoreData: moreData,
		Content:  content,
	}

	// Send response
	c.JSON(http.StatusOK, Response{
		Data:    response,
		Message: "Successfully fetched data",
	})
}

func GetOrganizationByID(c *gin.Context) {
	// Extract the orgId from the path variables
	orgId := c.Param("orgId")

	// Mocked data for demonstration purposes
	organization := _Organization{
		OrganizationToken: "enim-commodi-excepturi",
		ID:                orgId,
		Name:              "D'Amore Inc",
		Image:             "https://xyz.com/image",
		Description:       "Omnis officia accusamus enim aut provident nulla et similique. Consequatur nobis et quos vero. Temporibus autem modi qui consequatur.",
		Industry:          "amira",
		Website:           "https://ariel.com",
		APIKeyAvailable:   false,
	}

	// Send the response
	c.JSON(http.StatusOK, Response{
		Data:    organization,
		Message: "Success",
	})
}

func CreateOrganization(c *gin.Context) {
	// Parse the form data
	name := c.PostForm("name")
	description := c.PostForm("description")
	industry := c.PostForm("industry")
	website := c.PostForm("website")

	// Handle the file upload
	file, err := c.FormFile("image")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Image upload failed"})
		return
	}

	// Save the uploaded file to a specific location (for this example, it's saved to ./uploads/)
	filePath := filepath.Join("uploads", filepath.Base(file.Filename))
	if err := c.SaveUploadedFile(file, filePath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save image"})
		return
	}

	// Mocked data for demonstration purposes
	organization := _Organization{
		OrganizationToken: "est-est-assumenda",
		ID:                "850",
		Name:              name,
		Image:             "https://xyz.com/image", // You can replace this with the actual file path or a URL
		Description:       description,
		Industry:          industry,
		Website:           website,
	}

	// Send the response
	c.JSON(http.StatusOK, Response{
		Data:    organization,
		Message: "Success",
	})
}

func UpdateOrganization(c *gin.Context) {
	// Parse the form data
	name := c.PostForm("name")
	description := c.PostForm("description")
	industry := c.PostForm("industry")
	website := c.PostForm("website")

	// Handle the file upload
	file, err := c.FormFile("image")
	var filePath string
	if err == nil {
		// Save the uploaded file to a specific location (for this example, it's saved to ./uploads/)
		filePath = filepath.Join("uploads", filepath.Base(file.Filename))
		if err := c.SaveUploadedFile(file, filePath); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save image"})
			return
		}
	} else {
		// Handle case where image is not provided
		filePath = "" // or retain the previous image path
	}

	// Mocked data for demonstration purposes
	organization := _Organization{
		ID:          "850",
		Name:        name,
		Image:       "https://xyz.com/image", // You can replace this with the actual file path or a URL
		Description: description,
		Industry:    industry,
		Website:     website,
	}

	// Send the response
	c.JSON(http.StatusOK, Response{
		Data:    organization,
		Message: "Success",
	})
}
