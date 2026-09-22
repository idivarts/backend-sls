// Package socialtokens provides just-in-time refresh of brand-connected social
// access tokens. Short-lived OAuth tokens (YouTube ~1h, Twitter ~2h) expire long
// before the 6-hourly refresh cron runs, so publishing a video an hour after
// connecting would hit the platform with an expired token and 401. Callers on the
// publish path use EnsureFreshBrandToken to refresh-and-persist right before use.
package socialtokens

import (
	"fmt"
	"log"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/linkedin"
	"github.com/idivarts/backend-sls/pkg/twitter"
	"github.com/idivarts/backend-sls/pkg/youtube"
)

// expiryBufferSeconds — refresh if the token expires within this window, so a
// token that is valid now but dies mid-upload is renewed pre-emptively.
const expiryBufferSeconds = 120

// EnsureFreshBrandToken returns a token valid for the near future, refreshing and
// persisting it first when it is expired/expiring and refreshable. It mutates and
// returns the passed token so callers can use the result directly.
//
// Best-effort: platforms with no expiry, no refresh token, or an unsupported
// refresh flow are returned unchanged (the caller still attempts the publish; a
// genuinely dead token then surfaces as an auth failure prompting reconnect). A
// refresh error is logged and the (stale) token returned rather than aborting.
func EnsureFreshBrandToken(brandID string, acc *trendlymodels.SocialAccount, token *trendlymodels.SocialToken) *trendlymodels.SocialToken {
	if token == nil {
		return token
	}
	// No recorded expiry, or comfortably valid → nothing to do.
	if token.TokenExpiry == 0 || token.TokenExpiry > time.Now().Unix()+expiryBufferSeconds {
		return token
	}
	if token.RefreshToken == "" {
		return token // not refreshable (e.g. Meta long-lived, or no offline grant)
	}

	newAccess, newRefresh, newExpiry, err := refreshByPlatform(token.Platform, token.RefreshToken)
	if err != nil {
		log.Printf("socialtokens: refresh failed for brand %s account %s [%s]: %v", brandID, acc.ID, token.Platform, err)
		return token
	}
	if newAccess == "" {
		return token
	}

	// The token doc id follows acc.TokenRef when set (linkedin_page Pages share
	// one member token doc); otherwise it is the account id.
	tokenID := acc.ID
	if acc.TokenRef != "" {
		tokenID = acc.TokenRef
	}
	updates := []firestore.Update{
		{Path: "accessToken", Value: newAccess},
		{Path: "tokenExpiry", Value: newExpiry},
	}
	if newRefresh != "" {
		updates = append(updates, firestore.Update{Path: "refreshToken", Value: newRefresh})
	}
	if _, uerr := trendlymodels.UpdateBrandSocialToken(brandID, tokenID, updates); uerr != nil {
		// Persist failed, but the refresh itself worked — use the new token for
		// THIS publish so it still succeeds; the cron will re-persist later.
		log.Printf("socialtokens: refreshed but failed to persist for brand %s token %s: %v", brandID, tokenID, uerr)
	}

	token.AccessToken = newAccess
	token.TokenExpiry = newExpiry
	if newRefresh != "" {
		token.RefreshToken = newRefresh
	}
	return token
}

// refreshByPlatform calls the right platform refresh client and normalises the
// result to (accessToken, refreshToken, expiryUnix). refreshToken is empty when
// the platform doesn't rotate it (Google) — the caller keeps the stored one.
func refreshByPlatform(platform trendlymodels.Platform, refreshToken string) (string, string, int64, error) {
	switch platform {
	case trendlymodels.PlatformYouTube:
		r, err := youtube.RefreshAccessToken(refreshToken)
		if err != nil {
			return "", "", 0, err
		}
		return r.AccessToken, r.RefreshToken, r.ExpiresAt(), nil
	case trendlymodels.PlatformTwitter:
		r, err := twitter.RefreshAccessToken(refreshToken)
		if err != nil {
			return "", "", 0, err
		}
		return r.AccessToken, r.RefreshToken, r.ExpiresAt(), nil
	case trendlymodels.PlatformLinkedIn:
		r, err := linkedin.RefreshAccessToken(refreshToken)
		if err != nil {
			return "", "", 0, err
		}
		return r.AccessToken, r.RefreshToken, r.ExpiresAt(), nil
	case trendlymodels.PlatformLinkedInPage:
		// Company Pages use the dedicated Community Management API app.
		r, err := linkedin.RefreshAccessTokenCM(refreshToken)
		if err != nil {
			return "", "", 0, err
		}
		return r.AccessToken, r.RefreshToken, r.ExpiresAt(), nil
	default:
		// instagram / facebook / reddit — not refreshed on the publish path.
		return "", "", 0, fmt.Errorf("no publish-time refresh for platform %q", platform)
	}
}
