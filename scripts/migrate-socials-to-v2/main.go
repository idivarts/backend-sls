package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"cloud.google.com/go/firestore"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/facebook"

	_ "github.com/idivarts/backend-sls/pkg/firebase"
)

func main() {
	migrated, skipped, failed := 0, 0, 0

	ctx := context.Background()
	iter := firestoredb.Client.CollectionGroup("socials").Documents(ctx)
	defer iter.Stop()

	for {
		doc, err := iter.Next()
		if err != nil {
			if err.Error() == "iterator: done" {
				break
			}
			log.Fatalf("collection group iteration error: %v", err)
		}

		// Path: users/{userId}/socials/{socialId}
		userID := doc.Ref.Parent.Parent.ID
		socialID := doc.Ref.ID

		var old trendlymodels.Socials
		if err := doc.DataTo(&old); err != nil {
			log.Printf("[SKIP] %s/%s: failed to decode Socials: %v", userID, socialID, err)
			failed++
			continue
		}

		// Determine platform and username before computing the new ID.
		platform, username, displayName, profileImageURL, bio, profileURL,
			followerCount, followingCount, mediaCount :=
			extractProfileFields(&old)

		if username == "" {
			log.Printf("[SKIP] %s/%s: could not determine username, skipping", userID, socialID)
			skipped++
			continue
		}

		newID := trendlymodels.SocialAccountID(platform, username)

		// Idempotency check — skip if the socialAccount already exists.
		existRef := firestoredb.Client.
			Collection(fmt.Sprintf("users/%s/socialAccounts", userID)).
			Doc(newID)
		existSnap, err := existRef.Get(ctx)
		if err == nil && existSnap.Exists() {
			log.Printf("[SKIP] %s/%s → %s already exists", userID, socialID, newID)
			skipped++
			continue
		}

		// Fetch matching socialsPrivate for the access token.
		var priv trendlymodels.SocialsPrivate
		privErr := priv.Get(userID, socialID)
		if privErr != nil {
			log.Printf("[WARN] %s/%s: socialsPrivate not found (%v); token will be empty", userID, socialID, privErr)
		}

		accessToken := ""
		if priv.AccessToken != nil {
			accessToken = *priv.AccessToken
		}

		now := time.Now().Unix()

		account := &trendlymodels.SocialAccount{
			ID:              newID,
			Platform:        platform,
			UserID:          userID,
			Username:        username,
			DisplayName:     displayName,
			ProfileImageURL: profileImageURL,
			Bio:             bio,
			ProfileURL:      profileURL,
			FollowerCount:   followerCount,
			FollowingCount:  followingCount,
			MediaCount:      mediaCount,
			ConnectedAt:     now,
			UpdatedAt:       now,
			RawProfile:      buildRawProfile(&old),
		}

		token := &trendlymodels.SocialToken{
			Platform:    platform,
			AccessToken: accessToken,
			TokenExpiry: 0, // unknown for migrated tokens
		}

		if err := trendlymodels.SaveSocialAccount(userID, account, token); err != nil {
			log.Printf("[FAIL] %s/%s: SaveSocialAccount: %v", userID, socialID, err)
			failed++
			continue
		}

		log.Printf("[OK] %s/%s → socialAccounts/%s (%s @%s)", userID, socialID, newID, platform, username)
		migrated++
	}

	log.Printf("\nDone — migrated: %d  skipped: %d  failed: %d", migrated, skipped, failed)
}

// extractProfileFields derives the canonical profile fields from the legacy Socials doc.
// Priority: typed profile structs (InstaProfile / FBProfile) > flat Socials fields.
func extractProfileFields(s *trendlymodels.Socials) (
	platform trendlymodels.Platform,
	username, displayName, profileImageURL, bio, profileURL string,
	followerCount, followingCount, mediaCount int64,
) {
	if s.IsInstagram {
		platform = trendlymodels.PlatformInstagram
	} else {
		platform = trendlymodels.PlatformFacebook
	}

	// Fallback values from flat fields.
	username = s.OwnerName
	displayName = s.Name
	profileImageURL = s.Image

	if s.InstaProfile != nil {
		p := s.InstaProfile
		if p.Username != "" {
			username = p.Username
		}
		if p.Name != "" {
			displayName = p.Name
		}
		if p.ProfilePictureURL != "" {
			profileImageURL = p.ProfilePictureURL
		}
		bio = p.Biography
		profileURL = p.Website
		followerCount = int64(p.FollowersCount)
		followingCount = int64(p.FollowsCount)
		mediaCount = int64(p.MediaCount)
	} else if s.FBProfile != nil {
		p := s.FBProfile
		if p.Name != "" {
			displayName = p.Name
		}
		if p.Picture.Data.URL != "" {
			profileImageURL = p.Picture.Data.URL
		}
		bio = p.About
		profileURL = p.Website
		followerCount = int64(p.FollowersCount)
	}

	return
}

// buildRawProfile serialises the typed profile structs into a plain map so
// SocialAccount.RawProfile can store the original API payload without
// importing the messenger package in the model layer.
func buildRawProfile(s *trendlymodels.Socials) map[string]interface{} {
	var src interface{}
	if s.InstaProfile != nil {
		src = s.InstaProfile
	} else if s.FBProfile != nil {
		src = s.FBProfile
	}
	if src == nil {
		return nil
	}

	b, err := json.Marshal(src)
	if err != nil {
		return nil
	}
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		return nil
	}
	return m
}

// Ensure messenger import is used (profile types are embedded in trendlymodels.Socials).
var _ = facebook.InstagramProfile{}
var _ = firestore.Update{}
