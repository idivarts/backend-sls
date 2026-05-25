// Package main implements a scheduled Lambda function that refreshes
// expiring social platform access tokens stored in Firestore.
//
// Runs every 6 hours via EventBridge cron. For each user's socialsV2Private
// sub-collection, it finds tokens expiring within the next 7 days and
// attempts a refresh using the appropriate platform client.
//
// Token refresh support matrix:
//   - Instagram  : long-lived tokens (60d); refresh by calling the IG refresh endpoint
//   - Facebook   : long-lived tokens (60d); no refresh API — user must reconnect
//   - YouTube    : access token (1h) + refresh token (no expiry); Google refresh API
//   - LinkedIn   : access token (60d); refresh token if offline_access was granted
//   - Twitter/X  : access token (2h) + rotating refresh tokens; Twitter refresh API
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
	"github.com/idivarts/backend-sls/pkg/instagram"
	"github.com/idivarts/backend-sls/pkg/linkedin"
	"github.com/idivarts/backend-sls/pkg/twitter"
	"github.com/idivarts/backend-sls/pkg/youtube"
)

const refreshWindowDays = 7 // Refresh tokens expiring within this many days

func main() {
	lambda.Start(handler)
}

func handler(ctx context.Context) error {
	log.Println("social_refresh: starting token refresh scan")

	now := time.Now().Unix()
	deadline := now + int64(refreshWindowDays*24*60*60)

	// Iterate over all users
	userDocs, err := firestoredb.Client.Collection("users").Documents(ctx).GetAll()
	if err != nil {
		return fmt.Errorf("social_refresh: failed to list users: %w", err)
	}

	refreshed, skipped, failed := 0, 0, 0

	for _, userDoc := range userDocs {
		userID := userDoc.Ref.ID

		privDocs, err := firestoredb.Client.
			Collection(fmt.Sprintf("users/%s/socialsV2Private", userID)).
			Documents(ctx).
			GetAll()
		if err != nil {
			log.Printf("social_refresh: failed to list socialsV2Private for %s: %v", userID, err)
			continue
		}

		for _, privDoc := range privDocs {
			var priv trendlymodels.SocialV2Private
			if err := privDoc.DataTo(&priv); err != nil {
				log.Printf("social_refresh: failed to decode private doc %s/%s: %v", userID, privDoc.Ref.ID, err)
				failed++
				continue
			}

			// Skip tokens with no expiry (e.g. non-expiring tokens) or plenty of time left
			if priv.TokenExpiry > 0 && priv.TokenExpiry > deadline {
				skipped++
				continue
			}

			if err := refreshToken(ctx, userID, privDoc.Ref.ID, &priv); err != nil {
				log.Printf("social_refresh: failed to refresh %s/%s [%s]: %v",
					userID, privDoc.Ref.ID, priv.Platform, err)
				failed++
			} else {
				refreshed++
			}
		}
	}

	log.Printf("social_refresh: done — refreshed=%d skipped=%d failed=%d", refreshed, skipped, failed)
	return nil
}

// refreshToken attempts to refresh the access token for a single social account.
func refreshToken(ctx context.Context, userID, socialID string, priv *trendlymodels.SocialV2Private) error {
	switch priv.Platform {
	case trendlymodels.PlatformInstagram:
		return refreshInstagram(ctx, userID, socialID, priv)
	case trendlymodels.PlatformYouTube:
		return refreshYouTube(ctx, userID, socialID, priv)
	case trendlymodels.PlatformLinkedIn:
		return refreshLinkedIn(ctx, userID, socialID, priv)
	case trendlymodels.PlatformTwitter:
		return refreshTwitter(ctx, userID, socialID, priv)
	case trendlymodels.PlatformFacebook:
		// Facebook long-lived tokens cannot be refreshed via API.
		// Log a warning; the user will need to reconnect when the token expires.
		log.Printf("social_refresh: Facebook token for %s/%s expiring soon — user must reconnect", userID, socialID)
		return nil
	default:
		return fmt.Errorf("unknown platform %q", priv.Platform)
	}
}

// refreshInstagram refreshes an Instagram long-lived token (valid 60 days).
// Instagram's refresh endpoint simply extends the expiry of an existing long-lived token.
func refreshInstagram(ctx context.Context, userID, socialID string, priv *trendlymodels.SocialV2Private) error {
	newToken, err := instagram.RefreshLongLivedToken(priv.AccessToken)
	if err != nil {
		return fmt.Errorf("instagram refresh: %w", err)
	}
	return persistPrivate(ctx, userID, socialID, priv.Platform, map[string]interface{}{
		"accessToken": newToken.AccessToken,
		"tokenExpiry": time.Now().Add(time.Duration(newToken.ExpiresIn) * time.Second).Unix(),
	})
}

// refreshYouTube uses the stored refresh token to get a new access token.
func refreshYouTube(ctx context.Context, userID, socialID string, priv *trendlymodels.SocialV2Private) error {
	if priv.RefreshToken == "" {
		return fmt.Errorf("youtube: no refresh token stored for %s/%s", userID, socialID)
	}
	newTokens, err := youtube.RefreshAccessToken(priv.RefreshToken)
	if err != nil {
		return fmt.Errorf("youtube refresh: %w", err)
	}
	return persistPrivate(ctx, userID, socialID, priv.Platform, map[string]interface{}{
		"accessToken": newTokens.AccessToken,
		"tokenExpiry": newTokens.ExpiresAt(),
		// Google doesn't rotate refresh tokens; keep the existing one
	})
}

// refreshLinkedIn uses the stored refresh token (if available).
func refreshLinkedIn(ctx context.Context, userID, socialID string, priv *trendlymodels.SocialV2Private) error {
	if priv.RefreshToken == "" {
		// LinkedIn only issues refresh tokens with offline_access scope.
		// Without it, the user must reconnect when the token expires.
		log.Printf("social_refresh: LinkedIn token for %s/%s expiring soon — no refresh token; user must reconnect", userID, socialID)
		return nil
	}
	newTokens, err := linkedin.RefreshAccessToken(priv.RefreshToken)
	if err != nil {
		return fmt.Errorf("linkedin refresh: %w", err)
	}
	updates := map[string]interface{}{
		"accessToken": newTokens.AccessToken,
		"tokenExpiry": newTokens.ExpiresAt(),
	}
	if newTokens.RefreshToken != "" {
		updates["refreshToken"] = newTokens.RefreshToken
	}
	return persistPrivate(ctx, userID, socialID, priv.Platform, updates)
}

// refreshTwitter uses the stored refresh token (Twitter rotates refresh tokens).
func refreshTwitter(ctx context.Context, userID, socialID string, priv *trendlymodels.SocialV2Private) error {
	if priv.RefreshToken == "" {
		return fmt.Errorf("twitter: no refresh token stored for %s/%s", userID, socialID)
	}
	newTokens, err := twitter.RefreshAccessToken(priv.RefreshToken)
	if err != nil {
		return fmt.Errorf("twitter refresh: %w", err)
	}
	updates := map[string]interface{}{
		"accessToken": newTokens.AccessToken,
		"tokenExpiry": newTokens.ExpiresAt(),
	}
	// Twitter always rotates refresh tokens
	if newTokens.RefreshToken != "" {
		updates["refreshToken"] = newTokens.RefreshToken
	}
	return persistPrivate(ctx, userID, socialID, priv.Platform, updates)
}

// persistPrivate applies a partial update to a socialsV2Private document.
func persistPrivate(ctx context.Context, userID, socialID string, platform trendlymodels.Platform, fields map[string]interface{}) error {
	ref := firestoredb.Client.
		Collection(fmt.Sprintf("users/%s/socialsV2Private", userID)).
		Doc(socialID)

	updates := make([]firestore.Update, 0, len(fields))
	for k, v := range fields {
		updates = append(updates, firestore.Update{Path: k, Value: v})
	}

	if _, err := ref.Update(ctx, updates); err != nil {
		return fmt.Errorf("persistPrivate [%s/%s/%s]: %w", userID, socialID, platform, err)
	}
	log.Printf("social_refresh: refreshed token for %s/%s [%s]", userID, socialID, platform)
	return nil
}
