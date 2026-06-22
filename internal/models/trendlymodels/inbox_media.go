package trendlymodels

import (
	"context"
	"fmt"

	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
)

// ─── Inbox media (brands/{brandId}/inboxMedia/{mediaId}) ──────────────────────
//
// The brand's published posts/reels across connected Meta accounts, populated
// asynchronously by the social_sqs worker (OpMedia) so the Media tab reads it
// live from Firestore instead of waiting on a slow multi-account Graph fetch.
// Written server-side only (Admin SDK); the frontend subscribes read-only.

type InboxMediaDoc struct {
	ID            string `json:"id" firestore:"id"`
	Channel       string `json:"channel" firestore:"channel"` // "instagram" | "facebook"
	SocialID      string `json:"socialId" firestore:"socialId"`
	ThumbnailURL  string `json:"thumbnailUrl,omitempty" firestore:"thumbnailUrl,omitempty"`
	Caption       string `json:"caption,omitempty" firestore:"caption,omitempty"`
	Permalink     string `json:"permalink,omitempty" firestore:"permalink,omitempty"`
	Timestamp     int64  `json:"timestamp" firestore:"timestamp"` // epoch ms
	CommentsCount int    `json:"commentsCount" firestore:"commentsCount"`
	LikeCount     int    `json:"likeCount" firestore:"likeCount"`
	UpdatedAt     int64  `json:"updatedAt" firestore:"updatedAt"`
}

func brandInboxMediaCollection(brandID string) string {
	return fmt.Sprintf("brands/%s/inboxMedia", brandID)
}

// Upsert writes (creates or overwrites) a media doc keyed by its platform id.
func (m *InboxMediaDoc) Upsert(brandID string) error {
	if m.ID == "" {
		return fmt.Errorf("InboxMediaDoc.Upsert: empty ID")
	}
	_, err := firestoredb.Client.
		Collection(brandInboxMediaCollection(brandID)).
		Doc(m.ID).
		Set(context.Background(), m)
	return err
}
