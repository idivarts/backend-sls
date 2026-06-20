package trendlymodels

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"cloud.google.com/go/firestore"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
)

// ─── Platform constants ────────────────────────────────────────────────────────

// Platform identifies a social media platform using a stable string key.
// Using string rather than int avoids remapping issues as platforms are added.
type Platform = string

const (
	PlatformInstagram Platform = "instagram"
	PlatformFacebook  Platform = "facebook"
	PlatformYouTube   Platform = "youtube"
	PlatformLinkedIn  Platform = "linkedin"
	PlatformTwitter   Platform = "twitter"
)

// ─── SocialAccount (users/{userId}/socialAccounts/{id}) ───────────────────────

// SocialAccount is the public social account document.
// It lives alongside the legacy `socials` sub-collection; the old collection
// is never written to for new connections.
type SocialAccount struct {
	// Core identity
	ID       string   `json:"id" firestore:"id"`             // deterministic: sha256(platform:username)
	Platform Platform `json:"platform" firestore:"platform"` // "instagram", "facebook", etc.
	UserID   string   `json:"userId" firestore:"userId"`

	// PlatformAccountID is the platform's own account id — the Instagram
	// Business Account id or the Facebook Page id. It is the key Meta webhooks
	// arrive under (entry.id), so it is what the inbox webhook router uses to
	// resolve an event back to a connected account. Distinct from `ID`, which
	// is an opaque hash of platform:username.
	PlatformAccountID string `json:"platformAccountId,omitempty" firestore:"platformAccountId,omitempty"`

	// InstagramBusinessID is the IG Business Account id linked to a Facebook
	// Page (only set on facebook page accounts). Page-linked IG message/comment
	// webhooks arrive under this id and are served using the page access token.
	InstagramBusinessID string `json:"instagramBusinessId,omitempty" firestore:"instagramBusinessId,omitempty"`

	// Profile info (refreshed on each reconnect / token refresh)
	Username        string `json:"username" firestore:"username"`
	DisplayName     string `json:"displayName" firestore:"displayName"`
	ProfileImageURL string `json:"profileImageURL" firestore:"profileImageURL"`
	Bio             string `json:"bio,omitempty" firestore:"bio"`
	ProfileURL      string `json:"profileURL,omitempty" firestore:"profileURL"` // e.g. https://instagram.com/{handle}

	// Metrics (updated by social sync job)
	FollowerCount  int64 `json:"followerCount" firestore:"followerCount"`
	FollowingCount int64 `json:"followingCount" firestore:"followingCount"`
	MediaCount     int64 `json:"mediaCount" firestore:"mediaCount"` // posts/videos

	// Connection metadata
	ConnectedAt int64 `json:"connectedAt" firestore:"connectedAt"` // Unix timestamp
	UpdatedAt   int64 `json:"updatedAt" firestore:"updatedAt"`     // last profile sync

	// Platform-specific raw profile blob (optional, for future use)
	// Stored as map so we don't need to import platform packages here.
	RawProfile map[string]interface{} `json:"rawProfile,omitempty" firestore:"rawProfile"`
}

// SocialAccountID returns the canonical document ID for a social account.
// It is the SHA-256 hex digest of "{platform}:{username}" — deterministic and opaque.
func SocialAccountID(platform Platform, username string) string {
	h := sha256.Sum256([]byte(platform + ":" + username))
	return hex.EncodeToString(h[:])
}

// Insert creates or overwrites the public social account document in Firestore.
func (s *SocialAccount) Insert(userID string) (*firestore.WriteResult, error) {
	res, err := firestoredb.Client.
		Collection(fmt.Sprintf("users/%s/socialAccounts", userID)).
		Doc(s.ID).
		Set(context.Background(), s)
	if err != nil {
		return nil, err
	}
	return res, nil
}

// Get reads the social account document into the receiver.
func (s *SocialAccount) Get(userID, id string) error {
	res, err := firestoredb.Client.
		Collection(fmt.Sprintf("users/%s/socialAccounts", userID)).
		Doc(id).
		Get(context.Background())
	if err != nil {
		return err
	}
	return res.DataTo(s)
}

// Update performs a partial update on the social account document.
func (s *SocialAccount) Update(userID, id string, fields []firestore.Update) (*firestore.WriteResult, error) {
	res, err := firestoredb.Client.
		Collection(fmt.Sprintf("users/%s/socialAccounts", userID)).
		Doc(id).
		Update(context.Background(), fields)
	if err != nil {
		return nil, err
	}
	return res, nil
}

// ListSocialAccounts returns all social accounts for a given user.
func ListSocialAccounts(userID string) ([]SocialAccount, error) {
	docs, err := firestoredb.Client.
		Collection(fmt.Sprintf("users/%s/socialAccounts", userID)).
		Documents(context.Background()).
		GetAll()
	if err != nil {
		return nil, err
	}

	accounts := make([]SocialAccount, 0, len(docs))
	for _, doc := range docs {
		var a SocialAccount
		if err := doc.DataTo(&a); err != nil {
			return nil, fmt.Errorf("ListSocialAccounts: failed to decode %s: %w", doc.Ref.ID, err)
		}
		accounts = append(accounts, a)
	}
	return accounts, nil
}

// ─── SocialToken (users/{userId}/socialTokens/{id}) ───────────────────────────

// SocialToken holds sensitive token data in a separate sub-collection.
// Access rules should restrict reads to server-side only (Firebase security rules).
type SocialToken struct {
	Platform     Platform `json:"platform" firestore:"platform"`
	AccessToken  string   `json:"accessToken" firestore:"accessToken"`
	RefreshToken string   `json:"refreshToken,omitempty" firestore:"refreshToken"` // empty for non-refreshable tokens
	TokenExpiry  int64    `json:"tokenExpiry" firestore:"tokenExpiry"`             // Unix timestamp; 0 = no expiry
	Scopes       []string `json:"scopes,omitempty" firestore:"scopes"`
	// PKCE verifier is only stored transiently during the OAuth flow (not persisted here).
}

// Set creates or overwrites the token document in Firestore.
func (t *SocialToken) Set(userID, id string) (*firestore.WriteResult, error) {
	res, err := firestoredb.Client.
		Collection(fmt.Sprintf("users/%s/socialTokens", userID)).
		Doc(id).
		Set(context.Background(), t)
	if err != nil {
		return nil, err
	}
	return res, nil
}

// Get reads the token document into the receiver.
func (t *SocialToken) Get(userID, id string) error {
	res, err := firestoredb.Client.
		Collection(fmt.Sprintf("users/%s/socialTokens", userID)).
		Doc(id).
		Get(context.Background())
	if err != nil {
		return err
	}
	return res.DataTo(t)
}

// Update performs a partial update on the token document.
func (t *SocialToken) Update(userID, id string, fields []firestore.Update) (*firestore.WriteResult, error) {
	res, err := firestoredb.Client.
		Collection(fmt.Sprintf("users/%s/socialTokens", userID)).
		Doc(id).
		Update(context.Background(), fields)
	if err != nil {
		return nil, err
	}
	return res, nil
}

// ─── Batch helpers ────────────────────────────────────────────────────────────

// SaveSocialAccount atomically writes both the public SocialAccount and its
// SocialToken in a single Firestore batch commit.
func SaveSocialAccount(userID string, account *SocialAccount, token *SocialToken) error {
	ctx := context.Background()

	pubRef := firestoredb.Client.
		Collection(fmt.Sprintf("users/%s/socialAccounts", userID)).
		Doc(account.ID)
	privRef := firestoredb.Client.
		Collection(fmt.Sprintf("users/%s/socialTokens", userID)).
		Doc(account.ID)

	batch := firestoredb.Client.Batch()
	batch.Set(pubRef, account)
	batch.Set(privRef, token)

	if _, err := batch.Commit(ctx); err != nil {
		return fmt.Errorf("SaveSocialAccount: batch commit failed: %w", err)
	}
	return nil
}

// ─── Brand-level helpers (brands/{brandId}/socialAccounts) ───────────────────

// SaveBrandSocialAccount atomically writes the SocialAccount and its SocialToken
// under the brand's Firestore sub-collections.
func SaveBrandSocialAccount(brandID string, account *SocialAccount, token *SocialToken) error {
	ctx := context.Background()

	pubRef := firestoredb.Client.
		Collection(fmt.Sprintf("brands/%s/socialAccounts", brandID)).
		Doc(account.ID)
	privRef := firestoredb.Client.
		Collection(fmt.Sprintf("brands/%s/socialTokens", brandID)).
		Doc(account.ID)

	batch := firestoredb.Client.Batch()
	batch.Set(pubRef, account)
	batch.Set(privRef, token)

	if _, err := batch.Commit(ctx); err != nil {
		return fmt.Errorf("SaveBrandSocialAccount: batch commit failed: %w", err)
	}
	return nil
}

// ListBrandSocialAccounts returns all social accounts connected to a given brand.
func ListBrandSocialAccounts(brandID string) ([]SocialAccount, error) {
	docs, err := firestoredb.Client.
		Collection(fmt.Sprintf("brands/%s/socialAccounts", brandID)).
		Documents(context.Background()).
		GetAll()
	if err != nil {
		return nil, err
	}

	accounts := make([]SocialAccount, 0, len(docs))
	for _, doc := range docs {
		var a SocialAccount
		if err := doc.DataTo(&a); err != nil {
			return nil, fmt.Errorf("ListBrandSocialAccounts: failed to decode %s: %w", doc.Ref.ID, err)
		}
		accounts = append(accounts, a)
	}
	return accounts, nil
}

// GetBrandSocialToken reads the (sensitive) token document for a brand-connected
// social account. Server-side only.
func GetBrandSocialToken(brandID, id string) (*SocialToken, error) {
	doc, err := firestoredb.Client.
		Collection(fmt.Sprintf("brands/%s/socialTokens", brandID)).
		Doc(id).
		Get(context.Background())
	if err != nil {
		return nil, err
	}
	var t SocialToken
	if err := doc.DataTo(&t); err != nil {
		return nil, err
	}
	return &t, nil
}

// GetBrandSocialAccount reads a single brand-connected social account.
func GetBrandSocialAccount(brandID, id string) (*SocialAccount, error) {
	doc, err := firestoredb.Client.
		Collection(fmt.Sprintf("brands/%s/socialAccounts", brandID)).
		Doc(id).
		Get(context.Background())
	if err != nil {
		return nil, err
	}
	var a SocialAccount
	if err := doc.DataTo(&a); err != nil {
		return nil, err
	}
	return &a, nil
}

// DeleteBrandSocialAccount atomically removes both the SocialAccount and its
// SocialToken documents for the given brand.
func DeleteBrandSocialAccount(brandID, socialID string) error {
	ctx := context.Background()

	pubRef := firestoredb.Client.
		Collection(fmt.Sprintf("brands/%s/socialAccounts", brandID)).
		Doc(socialID)
	privRef := firestoredb.Client.
		Collection(fmt.Sprintf("brands/%s/socialTokens", brandID)).
		Doc(socialID)

	batch := firestoredb.Client.Batch()
	batch.Delete(pubRef)
	batch.Delete(privRef)

	if _, err := batch.Commit(ctx); err != nil {
		return fmt.Errorf("DeleteBrandSocialAccount: batch commit failed: %w", err)
	}
	return nil
}

// DeleteSocialAccount atomically removes both the SocialAccount and its
// SocialToken documents in a single Firestore batch commit.
func DeleteSocialAccount(userID, socialID string) error {
	ctx := context.Background()

	pubRef := firestoredb.Client.
		Collection(fmt.Sprintf("users/%s/socialAccounts", userID)).
		Doc(socialID)
	privRef := firestoredb.Client.
		Collection(fmt.Sprintf("users/%s/socialTokens", userID)).
		Doc(socialID)

	batch := firestoredb.Client.Batch()
	batch.Delete(pubRef)
	batch.Delete(privRef)

	if _, err := batch.Commit(ctx); err != nil {
		return fmt.Errorf("DeleteSocialAccount: batch commit failed: %w", err)
	}
	return nil
}
