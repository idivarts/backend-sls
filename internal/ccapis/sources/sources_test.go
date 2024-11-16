package sourcesapi_test

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	sourcesapi "github.com/idivarts/backend-sls/internal/ccapis/sources"
	"github.com/idivarts/backend-sls/pkg/messenger"
)

func TestFacebookLogin(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create a response recorder
	w := httptest.NewRecorder()

	// Create a Gin context
	cont, r := gin.CreateTestContext(w)

	// Define the request body
	requestBody := `{
    "accounts": {
        "data": [
            {
            "access_token": "EAAGDG5jzw5QBO1rZBzt2wo1sM84gxJyggCxnQxYvPJdjTRDZC6JuJFZCnlBLyQ48etyN1Pw9VYqDlZCTIdYj48ERGuClZBMrJ04NfsqiaaRVmZAI5t86LZBDVjvGK3DiChQECM9ZAvKGQLoc3Jo55fPqRZA7czZAdFnNmRf3ZAsgky4C0zaoYRAs6eoT9FJ4CZAQRs5P1rpvl8b8bsvMEFwIvLDs22ob",
            "name": "Crowdy chat",
            "id": "412067225321320",
            "instagram_business_account": {
                "id": "17841468570912696"
            }
            },
            {
            "access_token": "EAAGDG5jzw5QBOZCTDgnU5MJSHtSN61odW1xwPPRNQTm8dvTB2wP9GCTeD3X4UsKKLTu9yNGuQM3DoHMxZBBZAWz6rbv8ZB5X3klz4yz1TJdHOzRKBS63hNfJloZABBVhlhMQKq7rZBE9ak6tGI9z6foAXoa0Gobg1hjBMZCjZCMbyZBhg1jdMyihH0WIy9dZCCjWdSCKD8liKU46Xgv5opmk56nimC",
            "name": "Trendly",
            "id": "311133518746783",
            "instagram_business_account": {
                "id": "17841466618151294"
            }
            },
            {
            "access_token": "EAAGDG5jzw5QBOyjNiWpuCmY0ZBUKhcT04Vtc5o1HUwjgMhrelwQ1pA98z1tjaUkMbR3N6yp5wOAV0b86vlHA7HQnH6EcGCxxqti4wIabohoq0EEU1xajBljoeD59AnT9Ya0SGjTUMhgHuAZCAoK9omQDK3ZAMB2zLnkERoWraa8rxbvqlApb5wo3LEZBIvB1k31BWGAIRTAIDZC1u7Hw51i8ZD",
            "name": "Creato AI",
            "id": "101101233043652",
            "instagram_business_account": {
                "id": "17841460662344485"
            }
            }
        ],
        "paging": {
            "cursors": {
            "before": "QVFIUkRZAeEtjZAlVPWDBPLVYxTlJvU0RodW1uYWp5ZATBZASFRVdHZAGdEFfUEdJOUNVeS1wUXhNajh5VTNvbEpseTJIUWRFMHYySDhkRlhSaVBIQW5WX3VSakVR",
            "after": "QVFIUmRSVTVCeHMwblV1bF9zUDBsMk1kdGNaNTh3amlNdmJRZAEtrZAFo4a0liNVpseS1Td0hJN28xZAU12X1llRmdtenJod1dPRE9qbXFraGt0LU5GeGNVQXVn"
            }
        }
        },
    "name": "Rahul Sinha",
    "id": "7574783585934388",
    "userID": "7574783585934388",
    "expiresIn": 7195,
    "accessToken": "EAAGDG5jzw5QBOzqnjmo3Be74N8XFx0pQiATadcKmVGNyfTak3EGIDIJP2ZCrhD6RwoNjbEMP1DZA5pZCpdwqIcZBBhn8jTogvyP2BvUoVhWBZA6SIxV8Y3yKmoQLkpJzLjieZByPqVuZBs6OhRxIK5hbJoBODqikAkLjKw36UdvZCwHnnoGzbv5ceoY8tvajrdv7czkRyUXCZBVh4L2mbVwZDZD",
    "signedRequest": "i-paIaLknfJ5mMfkPPLApjVv5vnXdXzc6R7HsN96Uqk.eyJ1c2VyX2lkIjoiNzU3NDc4MzU4NTkzNDM4OCIsImNvZGUiOiJBUUJqZktuV05zWWxhMlpwRXdpRk9WOG9VZUZwelQ2bE1RaXVwYjBDVnFuNU5seEpiVEx2S3VhdXFKWkZwekJPbGw1VWEtQ0k5djFQeDZ4VUtaNGRGbWQ5SV9vY1pwRndxemtMZDgtdjRmNDhtTy1ZelVrT3BYVjFwVWUzVDhRM2Z0OXVlMENLSGlYN3Jpb3VSdVZrVlJoT2FxQTF3UVl1MWhuVXUtTEppN3MwNk94OEFOSzhKUmFad1ZqLTNxbEVfRGc4d2hYSndXQWRtbFJaVndYQzdndktDemhfUnI4M0dUc0dsVE9XVW5ETVcycTBad25tQldtYnhQQzk2N1RMUEU3VUg0aGktUTNyTDN2U0VYaXdpcUFtMWdFcDJvT2V4dGU0aU1aYy1iMVhHMnBnM25fT3I3R3gxWHJzbmxiaGlncE9ER25ZTFJaNHV4cHYtR1RRQm1aYlc2NmZPeVA5MV9JUlZxb0dxNGVMc1EiLCJhbGdvcml0aG0iOiJITUFDLVNIQTI1NiIsImlzc3VlZF9hdCI6MTcyNTM4MjgwNn0",
    "graphDomain": "facebook",
    "data_access_expiration_time": 1733158805
}`
	req, err := http.NewRequest(http.MethodPost, "/facebook/login", bytes.NewBuffer([]byte(requestBody)))
	if err != nil {
		t.Fatalf("Could not create a request: %v", err)
	}

	// Set request header for JSON
	req.Header.Set("Content-Type", "application/json")

	// Assign the request to the context
	r.ServeHTTP(w, req)

	cont.Request = req
	cont.Set("firebaseUID", "0rdPB7B5q3cUvbu1Ewarp4Xg2AD3")
	cont.Set("organizationID", "jJLOC1LfG8WLgmAs5Ka7")

	// Call the function you want to test
	sourcesapi.FacebookLogin(cont)

	if w.Result().StatusCode != http.StatusOK {
		log.Printf("Expected status code 200, but got: %v\n\n\n%s\n\n", w.Result().StatusCode, w.Body.String())
		t.Fail()
	} else {
		log.Println(w.Body.String())
	}
}

func TestFetchMessages(t *testing.T) {
	data := messenger.FetchAllConversations(nil, "EAAGDG5jzw5QBOwqvavtNVZCa9CoxgvzWWCk7PhPKPmxPMeZAKnVRLO3FUWIeOU7mxLVZBzXUUG6uuhvHHQZBeCkKfSOEBzcyec0UJni2fvYZBY5g1bsRSHDrDD9s633ZB4ljUPhfQZAK9UAUg0jVZBbqANeWZAKpe2UxdzaHns7QoSur2yZAw6C0J7DYmp")
	log.Println(data)
}
