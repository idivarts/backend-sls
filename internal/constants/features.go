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
