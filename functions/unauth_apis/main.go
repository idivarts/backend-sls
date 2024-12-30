package main

import (
	"github.com/idivarts/backend-sls/internal/trendlyapis"
	apihandler "github.com/idivarts/backend-sls/pkg/api_handler"
)

func main() {
	instaApi := apihandler.GinEngine.Group("/instagram")

	instaApi.GET("/", trendlyapis.InstagramRedirect)
	instaApi.GET("/auth/:redirect_type", trendlyapis.InstagramAuthRedirect)
	instaApi.POST("/auth", trendlyapis.InstagramAuth)
	instaApi.GET("/deauth", trendlyapis.InstagramDeAuth)
	instaApi.GET("/delete", trendlyapis.InstagramDelete)

	firebaseApi := apihandler.GinEngine.Group("/firebase")

	firebaseApi.GET("/brands/members/add", trendlyapis.ValidateFirebaseCallback)

	apihandler.StartLambda()
}
