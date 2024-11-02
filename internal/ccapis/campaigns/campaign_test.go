package campaignsapi_test

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	campaignsapi "github.com/idivarts/backend-sls/internal/ccapis/campaigns"
)

func TestCreateCampaign(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create a response recorder
	w := httptest.NewRecorder()

	// Create a Gin context
	cont, r := gin.CreateTestContext(w)

	// Define the request body
	requestBody := ``
	req, err := http.NewRequest(http.MethodPost, "/facebook/login", bytes.NewBuffer([]byte(requestBody)))
	if err != nil {
		t.Fatalf("Could not create a request: %v", err)
	}

	// Set request header for JSON
	req.Header.Set("Content-Type", "application/json")

	// Assign the request to the context
	r.ServeHTTP(w, req)

	cont.Request = req
	// Set URL parameter
	cont.Params = gin.Params{
		{Key: "campaignId", Value: "Y1smxj5ZKVWRF3ea5Fvv"},
	}

	cont.Set("firebaseUID", "0rdPB7B5q3cUvbu1Ewarp4Xg2AD3")
	cont.Set("organizationID", "jJLOC1LfG8WLgmAs5Ka7")

	// Call the function you want to test
	campaignsapi.CreateOrUpdateCampaign(cont)

	if w.Result().StatusCode != http.StatusOK {
		log.Printf("Expected status code 200, but got: %v\n\n\n%s\n\n", w.Result().StatusCode, w.Body.String())
		t.Fail()
	} else {
		log.Println(w.Body.String())
	}
}
