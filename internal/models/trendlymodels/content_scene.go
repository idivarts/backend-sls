package trendlymodels

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
	"github.com/idivarts/backend-sls/pkg/scenegraph"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ContentSceneRef is the lightweight pointer stored on the parent content doc.
// It names the current scene revision and caches its rendered output + hit-box
// layout so the app can display and interact without loading the full graph.
type ContentSceneRef struct {
	RevisionID string                 `json:"revisionId" firestore:"revisionId"`
	DocType    string                 `json:"docType" firestore:"docType"` // image|video
	RenderURL  string                 `json:"renderUrl,omitempty" firestore:"renderUrl,omitempty"`
	ProxyURL   string                 `json:"proxyUrl,omitempty" firestore:"proxyUrl,omitempty"` // video preview proxy
	Boxes      []scenegraph.LayoutBox `json:"boxes,omitempty" firestore:"boxes,omitempty"`
	UpdatedAt  int64                  `json:"updatedAt" firestore:"updatedAt"`
}

// ContentAudio is the generated audio attached to a video content, muxed in at
// render time. References generatedAudio docs by id/url.
type ContentAudio struct {
	MusicID       string  `json:"musicId,omitempty" firestore:"musicId,omitempty"`
	MusicURL      string  `json:"musicUrl,omitempty" firestore:"musicUrl,omitempty"`
	MusicVolume   float64 `json:"musicVolume,omitempty" firestore:"musicVolume,omitempty"`
	VoiceoverID   string  `json:"voiceoverId,omitempty" firestore:"voiceoverId,omitempty"`
	VoiceoverURL  string  `json:"voiceoverUrl,omitempty" firestore:"voiceoverUrl,omitempty"`
	DuckMusic     bool    `json:"duckMusic,omitempty" firestore:"duckMusic,omitempty"`
	CaptionSource string  `json:"captionSource,omitempty" firestore:"captionSource,omitempty"`
}

// ContentSceneRevision is one immutable snapshot of a scene graph. Every AI
// apply / text edit creates a new revision so the user can revert (see the
// design critique). Stored at brands/{brandId}/contents/{contentId}/scenes/{id}.
type ContentSceneRevision struct {
	ID               string               `json:"id" firestore:"-"`
	Graph            *scenegraph.Document `json:"graph" firestore:"graph"`
	ParentRevisionID string               `json:"parentRevisionId,omitempty" firestore:"parentRevisionId,omitempty"`
	RenderURL        string               `json:"renderUrl,omitempty" firestore:"renderUrl,omitempty"`
	// Origin: "generate" | "edit" | "text" | "canva" | "revert".
	Origin    string `json:"origin,omitempty" firestore:"origin,omitempty"`
	CreatedAt int64  `json:"createdAt" firestore:"createdAt"`
}

func contentScenesCollection(brandID, contentID string) *firestore.CollectionRef {
	return firestoredb.Client.Collection(
		fmt.Sprintf("brands/%s/contents/%s/scenes", brandID, contentID))
}

// CreateSceneRevision writes a new immutable scene revision and returns its id.
func CreateSceneRevision(brandID, contentID string, rev *ContentSceneRevision) (string, error) {
	if rev.CreatedAt == 0 {
		rev.CreatedAt = time.Now().UnixMilli()
	}
	ref, _, err := contentScenesCollection(brandID, contentID).Add(context.Background(), rev)
	if err != nil {
		return "", fmt.Errorf("CreateSceneRevision: %w", err)
	}
	return ref.ID, nil
}

// GetSceneRevision reads a single revision, or (nil, nil) if absent.
func GetSceneRevision(brandID, contentID, revisionID string) (*ContentSceneRevision, error) {
	doc, err := contentScenesCollection(brandID, contentID).Doc(revisionID).Get(context.Background())
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, err
	}
	var r ContentSceneRevision
	if err := doc.DataTo(&r); err != nil {
		return nil, err
	}
	r.ID = doc.Ref.ID
	return &r, nil
}

// ListSceneRevisions returns a content's revisions newest-first (for the
// revision-history / revert UI). Capped by limit (0 = all).
func ListSceneRevisions(brandID, contentID string, limit int) ([]ContentSceneRevision, error) {
	q := contentScenesCollection(brandID, contentID).OrderBy("createdAt", firestore.Desc)
	if limit > 0 {
		q = q.Limit(limit)
	}
	iter := q.Documents(context.Background())
	defer iter.Stop()
	out := []ContentSceneRevision{}
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		var r ContentSceneRevision
		if err := doc.DataTo(&r); err != nil {
			return nil, err
		}
		r.ID = doc.Ref.ID
		out = append(out, r)
	}
	return out, nil
}

// SetContentSceneRef updates the parent content doc to point at a revision and
// caches its render + hit-box layout, and stamps the source as "ai".
func SetContentSceneRef(brandID, contentID string, ref *ContentSceneRef) error {
	ref.UpdatedAt = time.Now().UnixMilli()
	return UpdateContentFields(brandID, contentID, map[string]interface{}{
		"sceneRef": ref,
		"source":   "ai",
	})
}
