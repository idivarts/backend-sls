package constants

// Feature flags for in-progress integrations.
//
// RedditEnabled gates the entire Reddit integration (connect, posting, inbox
// DMs/comments, analytics). It is built end-to-end but PAUSED until the Reddit
// app + commercial Data API access are set up — see
// docs/reddit-integration-setup.md. While false, the connect endpoint 404s,
// Reddit is excluded from all inbox channels, and publish/analytics dispatch
// treat it as unsupported, so nothing Reddit-related is reachable.
//
// To enable: flip this to true AND the frontend flags
// (trendly-brands `constants/features.ts` REDDIT_ENABLED, trendly-connect
// `lib/config.ts` REDDIT_ENABLED).
const RedditEnabled = false

// LinkedInPageEnabled gates only the LinkedIn Page (Company/Showcase Page)
// integration via the Community Management API — connect-init, publish
// dispatch, inbox comment channel, and analytics fetch/snapshot. Personal
// LinkedIn (PlatformLinkedIn) is NOT gated by this flag and stays available.
// LinkedIn's CMA app review is taking too long, so this is PAUSED until that
// review clears.
//
// To enable: flip this to true AND the frontend flags
// (trendly-brands `constants/features.ts` LINKEDIN_PAGE_ENABLED, trendly-connect
// `lib/config.ts` LINKEDIN_PAGE_ENABLED).
const LinkedInPageEnabled = false
