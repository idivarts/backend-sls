package main

import (
	trendlyunauth "github.com/idivarts/backend-sls/internal/trendlyapis/unauth_apis"
	apihandler "github.com/idivarts/backend-sls/pkg/api_handler"
)

func main() {
	instaApi := apihandler.GinEngine.Group("/instagram")

	instaApi.GET("/", trendlyunauth.InstagramRedirect)
	instaApi.GET("/auth/:redirect_type", trendlyunauth.InstagramAuthRedirect)
	instaApi.POST("/auth", trendlyunauth.InstagramAuth)
	instaApi.GET("/deauth", trendlyunauth.InstagramDeAuth)
	instaApi.GET("/delete", trendlyunauth.InstagramDelete)

	firebaseApi := apihandler.GinEngine.Group("/firebase")

	firebaseApi.GET("/brands/members/add", trendlyunauth.ValidateFirebaseCallback)

	apihandler.StartLambda()
}
