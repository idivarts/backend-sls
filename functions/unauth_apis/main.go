package main

import (
	"github.com/idivarts/backend-sls/internal/trendlyapis/mediajobs"
	"github.com/idivarts/backend-sls/internal/trendlyapis/social_connect"
	trendlyunauth "github.com/idivarts/backend-sls/internal/trendlyapis/unauth_apis"
	apihandler "github.com/idivarts/backend-sls/pkg/api_handler"
)

func main() {
	onboard := apihandler.GinEngine.Group("/onboard")

	onboard.POST("/signup", trendlyunauth.Signup)
	onboard.GET("/email-redirection", trendlyunauth.EmailRedirection)
	onboard.POST("/reset-password", trendlyunauth.ResetPassword)
	onboard.POST("/check-email", trendlyunauth.CheckEmail)

	instaApi := apihandler.GinEngine.Group("/instagram")

	// Legacy: called by frontend to redirect to IG auth url (pre-connect-portal flow)
	instaApi.GET("/redirect", trendlyunauth.InstagramRedirect)
	instaApi.GET("/auth/:redirect_type", trendlyunauth.InstagramAuthRedirect)
	instaApi.GET("/deauth", trendlyunauth.InstagramDeAuth)
	instaApi.GET("/delete", trendlyunauth.InstagramDelete)

	// Generative-media callbacks. Unauthenticated because the provider calls
	// us, and the provider is taken from the ROUTE rather than the body — a
	// parser chosen by sniffing a payload is a parser chosen by whoever sent
	// it. The handler only ever resolves a job it already has a record of, and
	// the polling sweep is what actually guarantees resolution, so a spoofed
	// callback can at worst re-resolve a job that was already terminal.
	mediaApi := apihandler.GinEngine.Group("/media")
	mediaApi.POST("/webhook/:provider", mediajobs.Webhook)

	firebaseApi := apihandler.GinEngine.Group("/firebase")
	firebaseApi.GET("/brands/members/add", trendlyunauth.ValidateFirebaseCallback)

	// Public, view-only share links (no auth). Resolves a shareLinks/{token} and
	// returns the calendar-month payload. CORS handled globally.
	publicApi := apihandler.GinEngine.Group("/public")
	publicApi.GET("/shares/:token", trendlyunauth.PublicShareResolve)

	// Public free AI tools for the marketing website (no auth).
	// CORS + OPTIONS preflight are handled globally by middlewares.CORSMiddleware.
	// ⚠️ Unauthenticated + AI-cost-incurring — needs rate limiting before prod (see tools.go).
	toolsApi := apihandler.GinEngine.Group("/tools")
	toolsApi.POST("/generate", trendlyunauth.GenerateToolContent)

	// ── Social Connect Portal (V2 OAuth flow) ─────────────────────────────────
	// Init routes: browser redirect from connect portal — token in query param.
	connectInit := apihandler.GinEngine.Group("/connect", social_connect.ValidateQueryTokenMiddleware())
	connectInit.GET("/instagram", social_connect.InstagramInit)
	connectInit.GET("/facebook", social_connect.FacebookInit)
	connectInit.GET("/youtube", social_connect.YouTubeInit)
	connectInit.GET("/linkedin", social_connect.LinkedInInit)
	connectInit.GET("/twitter", social_connect.TwitterInit)
	connectInit.GET("/reddit", social_connect.RedditInit)
	connectInit.GET("/linkedin_page", social_connect.LinkedInPageInit)

	// Callback routes: called by OAuth provider — no auth header; userId in state.
	connectCallback := apihandler.GinEngine.Group("/connect")
	connectCallback.GET("/instagram/callback", social_connect.InstagramCallback)
	connectCallback.GET("/facebook/callback", social_connect.FacebookCallback)
	connectCallback.GET("/youtube/callback", social_connect.YouTubeCallback)
	connectCallback.GET("/linkedin/callback", social_connect.LinkedInCallback)
	connectCallback.GET("/twitter/callback", social_connect.TwitterCallback)
	connectCallback.GET("/reddit/callback", social_connect.RedditCallback)
	connectCallback.GET("/linkedin_page/callback", social_connect.LinkedInPageCallback)
	// Page-picker support (public, guarded by the random session id):
	connectCallback.GET("/linkedin_page/session", social_connect.LinkedInPageSessionInfo)
	connectCallback.POST("/linkedin_page/select", social_connect.LinkedInPageSelect)

	apihandler.StartLambda()
}
