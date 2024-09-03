package sourcesapi

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
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
                "access_token": "EAAGDG5jzw5QBOzUwlcZBcNsLCiZAXO14Fs9imRhqjiZAMDUBBirvZB2lKE1wxhNmoETnee19WdJZCoVhHwazzfcCSDvdAO7spx9EiouCrZATV5v8m0fcZAoB5ZBxKC5hjKL51IpbpkDx9gVMzcC7ozqCQa1ZCexE8nOTFU750ofJjIg871aU9rP20pkyQmm4SxIadpZCu33SzmBfjAwUAogoJj2rEP",
                "id": "311133518746783",
                "instagram_business_account": {
                    "id": "17841466618151294"
                },
                "name": "Trends Hub"
            },
            {
                "access_token": "EAAGDG5jzw5QBOZCahR9AUT6X04JbwM0vQIrgItKZCJz9SPmzhIqnGKVrh5EZCbDTI4lHsGKE3ZAImj3ZC4aaTenliq1f9iqK7HZCQR5PXi2FReMBvSLr3e9KQdMOInZC4v0L0yEgJomJ6WNnLYQj5gSdjpnkhteRMswmQqCvxQ71I95hUnbwPH19LtEjX5x2geH4ZBA84IQZAOm8fc2obsDaYRdgZD",
                "id": "101101233043652",
                "instagram_business_account": {
                    "id": "17841460662344485"
                },
                "name": "Creato AI"
            }
        ],
        "paging": {
            "cursors": {
                "before": "QVFIUnVmY2tCVG0yNWRCa3M1bGNnNDh5d0pEQndRS21DdzRiTU9neXhhMENHSEVqbWVicVlFWXhRWHA2SmpkbU9taDdFSTJ3em9tZAEZAFOUNpM1RFaDFaNnl3",
                "after": "QVFIUmRSVTVCeHMwblV1bF9zUDBsMk1kdGNaNTh3amlNdmJRZAEtrZAFo4a0liNVpseS1Td0hJN28xZAU12X1llRmdtenJod1dPRE9qbXFraGt0LU5GeGNVQXVn"
            }
        }
    },
    "name": "Rahul Sinha",
    "id": "7574783585934388",
    "userID": "7574783585934388",
    "expiresIn": 7195,
    "accessToken": "EAAGDG5jzw5QBO7XI3B2Y5ZBe2X7cMxfHRc8xKLFAZCRC4xhNlRNE83CxJzZA1Vw27KetZA5mtiBoVqlQk6AjVoGexdCcLxTKZBpf4U7uKsHrtrxWFE4KVZAOpOSZBe67S4zLLhtAn7OFw6C897cZBnIS8L5I6ieIAn9om0e1p9I0ZAYt9oni82hTuO62X6RWtTUm5Lo5luzQ7d8fBNDFbhwZDZD",
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
	FacebookLogin(cont)

	if w.Result().StatusCode != http.StatusOK {
		log.Printf("Expected status code 200, but got: %v\n\n\n%s\n\n", w.Result().StatusCode, w.Body.String())
		t.Fail()
	} else {
		log.Println(w.Body.String())
	}
}
