package trendlymodels

import "strings"

// ─── ContentFormat constants ─────────────────────────────────────────────────

// ContentFormat identifies the format of a content piece. A content has exactly
// ONE format but may target MANY platforms (see FormatPlatformSupport).
//
// Mirrors the frontend ContentFormatEnum in
// trendly-brands/shared-libs/firestore/trendly-pro/constants/content-format.ts
// — keep the two in sync.
//
// Video formats are split by aspect ratio:
//   - reel  = portrait / short-form vertical video (IG & FB Reels, YouTube Shorts)
//   - video = landscape / long-form 16:9 (YouTube, Facebook, LinkedIn, X, and
//     Instagram as a feed video — not a Reel)
type ContentFormat = string

const (
	ContentFormatPost     ContentFormat = "post"
	ContentFormatReel     ContentFormat = "reel"
	ContentFormatVideo    ContentFormat = "video"
	ContentFormatStory    ContentFormat = "story"
	ContentFormatCarousel ContentFormat = "carousel"
	ContentFormatLive     ContentFormat = "live"
	ContentFormatText     ContentFormat = "text"
)

// AllContentFormats lists every valid content format, in canonical order.
var AllContentFormats = []ContentFormat{
	ContentFormatPost, ContentFormatReel, ContentFormatVideo,
	ContentFormatStory, ContentFormatCarousel, ContentFormatLive, ContentFormatText,
}

// FormatPlatformSupport maps each content format to the platforms that support
// it — the single source of truth for the platform↔format restriction. A
// content's targeted platforms must all support its chosen format.
//
// Confirmed matrix (2026-06-17):
//   - post:     all except YouTube
//   - reel:     all (YouTube = Shorts)
//   - video:    all (Instagram = feed video, not a Reel)
//   - story:    Instagram + Facebook only
//   - carousel: Instagram, Facebook, LinkedIn
//   - live:     all except X/Twitter
//   - text:     Facebook, LinkedIn, X/Twitter (Instagram cannot do a text post)
var FormatPlatformSupport = map[ContentFormat][]Platform{
	ContentFormatPost:     {PlatformInstagram, PlatformFacebook, PlatformLinkedIn, PlatformTwitter},
	ContentFormatReel:     {PlatformInstagram, PlatformFacebook, PlatformYouTube, PlatformLinkedIn, PlatformTwitter},
	ContentFormatVideo:    {PlatformInstagram, PlatformFacebook, PlatformYouTube, PlatformLinkedIn, PlatformTwitter},
	ContentFormatStory:    {PlatformInstagram, PlatformFacebook},
	ContentFormatCarousel: {PlatformInstagram, PlatformFacebook, PlatformLinkedIn},
	ContentFormatLive:     {PlatformInstagram, PlatformFacebook, PlatformYouTube, PlatformLinkedIn},
	ContentFormatText:     {PlatformFacebook, PlatformLinkedIn, PlatformTwitter},
}

// IsValidContentFormat reports whether f is a known content format.
func IsValidContentFormat(f ContentFormat) bool {
	_, ok := FormatPlatformSupport[f]
	return ok
}

// NormalizeContentFormat lowercases/validates a free-form format string,
// falling back to "post" for unknown values.
func NormalizeContentFormat(v string) ContentFormat {
	f := strings.ToLower(strings.TrimSpace(v))
	if IsValidContentFormat(f) {
		return f
	}
	return ContentFormatPost
}

// PlatformsForFormat returns the platforms that support the given format.
func PlatformsForFormat(f ContentFormat) []Platform {
	return FormatPlatformSupport[f]
}

// IsFormatPlatformCompatible reports whether a (format, platform) pair is allowed.
func IsFormatPlatformCompatible(f ContentFormat, p Platform) bool {
	for _, allowed := range FormatPlatformSupport[f] {
		if allowed == p {
			return true
		}
	}
	return false
}

// IncompatiblePlatforms returns the subset of platforms that do NOT support the
// given format. An empty result means the whole selection is valid.
func IncompatiblePlatforms(f ContentFormat, platforms []Platform) []Platform {
	var bad []Platform
	for _, p := range platforms {
		if !IsFormatPlatformCompatible(f, p) {
			bad = append(bad, p)
		}
	}
	return bad
}

// ─── Platform normalization ──────────────────────────────────────────────────

// NormalizePlatform coerces a free-form / legacy platform string (e.g. the old
// capitalised "Instagram", or the "X / Twitter" label) into a canonical
// Platform key. The bool reports whether the value was recognised.
func NormalizePlatform(v string) (Platform, bool) {
	s := strings.ToLower(strings.TrimSpace(v))
	switch s {
	case "x", "x / twitter", "twitter/x":
		return PlatformTwitter, true
	case PlatformInstagram, PlatformFacebook, PlatformYouTube, PlatformLinkedIn, PlatformTwitter:
		return s, true
	default:
		return "", false
	}
}

// NormalizePlatforms coerces a list of legacy/mixed platform strings, dropping
// unknowns and de-duplicating while preserving order.
func NormalizePlatforms(vals []string) []Platform {
	out := []Platform{}
	seen := map[Platform]bool{}
	for _, raw := range vals {
		if p, ok := NormalizePlatform(raw); ok && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}
