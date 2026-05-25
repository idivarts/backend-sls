// Package main implements a scheduled Lambda function that refreshes
// expiring social platform access tokens stored in Firestore.
//
// Runs every 6 hours via EventBridge cron. For each user's socialTokens
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

const refreshWindowDays = 7

func main() {
	lambda.Start(handler)
}

func handler(ctx context.Context) error {
	log.Println("social_refresh: starting token refresh scan")

	now := time.Now().Unix()
	deadline := now + int64(refreshWindowDays*24*60*60)

	userDocs, err := firestoredb.Client.Collection("users").Documents(ctx).GetAll()
	if err != nil {
		return fmt.Errorf("social_refresh: failed to list users: %w", err)
	}

	refreshed, skipped, failed := 0, 0, 0

	for _, userDoc := range userDocs {
		userID := userDoc.Ref.ID

		// Use the model's ListSocialTokens equivalent by reading the sub-collection
		// directly — there is no list helper for tokens since it's an internal scan.
		tokenDocs, err := firestoredb.Client.
			Collection(fmt.Sprintf("users/%s/socialTokens", userID)).
			Documents(ctx).
			GetAll()
		if err != nil {
			log.Printf("social_refresh: failed to list socialTokens for %s: %v", userID, err)
			continue
		}

		for _, tokenDoc := range tokenDocs {
			var token trendlymodels.SocialToken
			if err := tokenDoc.DataTo(&token); err != nil {
				log.Printf("social_refresh: failed to decode token doc %s/%s: %v", userID, tokenDoc.Ref.ID, err)
				failed++
				continue
			}

			// Skip tokens with no expiry or plenty of time remaining
			if token.TokenExpiry > 0 && token.TokenExpiry > deadline {
				skipped++
				continue
			}

			if err := refreshToken(userID, tokenDoc.Ref.ID, &token); err != nil {
				log.Printf("social_refresh: failed to refresh %s/%s [%s]: %v",
					userID, tokenDoc.Ref.ID, token.Platform, err)
				failed++
			} else {
				refreshed++
			}
		}
	}

	log.Printf("social_refresh: done — refreshed=%d skipped=%d failed=%d", refreshed, skipped, failed)
	return nil
}

// refreshToken dispatches to the correct platform refresh handler.
func refreshToken(userID, socialID string, token *trendlymodels.SocialToken) error {
	switch token.Platform {
	case trendlymodels.PlatformInstagram:
		return refreshInstagram(userID, socialID, token)
	case trendlymodels.PlatformYouTube:
		return refreshYouTube(userID, socialID, token)
	case trendlymodels.PlatformLinkedIn:
		return refreshLinkedIn(userID, socialID, token)
	case trendlymodels.PlatformTwitter:
		return refreshTwitter(userID, socialID, token)
	case trendlymodels.PlatformFacebook:
		// Facebook long-lived tokens cannot be refreshed via API.
		log.Printf("social_refresh: Facebook token for %s/%s expiring soon — user must reconnect", userID, socialID)
		return nil
	default:
		return fmt.Errorf("unknown platform %q", token.Platform)
	}
}

func refreshInstagram(userID, socialID string, token *trendlymodels.SocialToken) error {
	newToken, err := instagram.RefreshLongLivedToken(token.AccessToken)
	if err != nil {
		return fmt.Errorf("instagram refresh: %w", err)
	}
	_, err = token.Update(userID, socialID, []firestore.Update{
		{Path: "accessToken", Value: newToken.AccessToken},
		{Path: "tokenExpiry", Value: time.Now().Add(time.Duration(newToken.ExpiresIn) * time.Second).Unix()},
	})
	return err
}

func refreshYouTube(userID, socialID string, token *trendlymodels.SocialToken) error {
	if token.RefreshToken == "" {
		return fmt.Errorf("youtube: no refresh token stored for %s/%s", userID, socialID)
	}
	newTokens, err := youtube.RefreshAccessToken(token.RefreshToken)
	if err != nil {
		return fmt.Errorf("youtube refresh: %w", err)
	}
	// Google doesn't rotate refresh tokens; only update access token + expiry
	_, err = token.Update(userID, socialID, []firestore.Update{
		{Path: "accessToken", Value: newTokens.AccessToken},
		{Path: "tokenExpiry", Value: newTokens.ExpiresAt()},
	})
	return err
}

func refreshLinkedIn(userID, socialID string, token *trendlymodels.SocialToken) error {
	if token.RefreshToken == "" {
		log.Printf("social_refresh: LinkedIn token for %s/%s expiring soon — no refresh token; user must reconnect", userID, socialID)
		return nil
	}
	newTokens, err := linkedin.RefreshAccessToken(token.RefreshToken)
	if err != nil {
		return fmt.Errorf("linkedin refresh: %w", err)
	}
	updates := []firestore.Update{
		{Path: "accessToken", Value: newTokens.AccessToken},
		{Path: "tokenExpiry", Value: newTokens.ExpiresAt()},
	}
	if newTokens.RefreshToken != "" {
		updates = append(updates, firestore.Update{Path: "refreshToken", Value: newTokens.RefreshToken})
	}
	_, err = token.Update(userID, socialID, updates)
	return err
}

func refreshTwitter(userID, socialID string, token *trendlymodels.SocialToken) error {
	if token.RefreshToken == "" {
		return fmt.Errorf("twitter: no refresh token stored for %s/%s", userID, socialID)
	}
	newTokens, err := twitter.RefreshAccessToken(token.RefreshToken)
	if err != nil {
		return fmt.Errorf("twitter refresh: %w", err)
	}
	updates := []firestore.Update{
		{Path: "accessToken", Value: newTokens.AccessToken},
		{Path: "tokenExpiry", Value: newTokens.ExpiresAt()},
	}
	if newTokens.RefreshToken != "" {
		updates = append(updates, firestore.Update{Path: "refreshToken", Value: newTokens.RefreshToken})
	}
	_, err = token.Update(userID, socialID, updates)
	return err
}
