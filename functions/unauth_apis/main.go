package main

import (
	"github.com/idivarts/backend-sls/internal/middlewares"
	trendlyunauth "github.com/idivarts/backend-sls/internal/trendlyapis/unauth_apis"
	apihandler "github.com/idivarts/backend-sls/pkg/api_handler"
)

func main() {
	instaApi := apihandler.GinEngine.Group("/instagram")

	// This is called by frontend. Purpose is to just redirect to insta auth url with needed params
	instaApi.GET("/", trendlyunauth.InstagramRedirect)

	// From insta server we get redirected here with code -> we inturn we redirect to frontend with code
	instaApi.GET("/auth/:redirect_type", trendlyunauth.InstagramAuthRedirect)

	// From frontend we call this api with code to complete the auth process and save tokens
	instaApi.POST("/auth", middlewares.ValidateSessionMiddleware(), trendlyunauth.InstagramAuth)

	// Insta calls this api to deauthorize our app for a user
	instaApi.GET("/deauth", trendlyunauth.InstagramDeAuth)

	// Insta calls this api to delete our app for a user
	instaApi.GET("/delete", trendlyunauth.InstagramDelete)

	firebaseApi := apihandler.GinEngine.Group("/firebase")

	firebaseApi.GET("/brands/members/add", trendlyunauth.ValidateFirebaseCallback)

	apihandler.StartLambda()
}
