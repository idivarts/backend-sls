package social_connect

// OAuth permission scopes requested during social-account connection.
// Centralized here as the single source of truth so every connect flow
// references the same list instead of inlining scope strings.

const (
	// FacebookScopes are the Graph API permissions requested in the Facebook
	// Login flow: basic profile (public_profile, email), Page access, and
	// business_management (to surface Pages owned via Business Manager).
	//
	// NOTE: pages_read_user_content and business_management are Advanced Access
	// permissions — they require Meta App Review before regular users are
	// granted them in production (App admins/developers/testers get them
	// immediately). See facebookReview.md.
	FacebookScopes = "public_profile," +
		"email," +
		"pages_show_list," +
		"pages_read_engagement," +
		"pages_read_user_content," +
		"pages_messaging," +
		"pages_manage_engagement," +
		"pages_manage_metadata," +
		"pages_manage_posts," +
		"business_management"

	// InstagramScopes are the permissions requested in the Business Login for
	// Instagram flow (instagram.com/oauth/authorize). Only instagram_business_*
	// scopes are valid here — Facebook scopes (public_profile, email, pages_*)
	// are NOT accepted by this endpoint.
	InstagramScopes = "instagram_business_basic," +
		"instagram_business_manage_comments," +
		"instagram_business_content_publish," +
		"instagram_business_manage_insights," +
		"instagram_business_manage_messages"
)
