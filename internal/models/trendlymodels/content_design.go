package trendlymodels

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ── HTML design model (supersedes the scene-graph JSON approach) ─────────────
// A design is a single self-contained HTML document (inline CSS + embedded font)
// sized to the exact post dimensions. LLMs author HTML/CSS far more reliably
// than layout JSON, and a real browser (WebView on device) renders it perfectly.
// The app captures the WebView to PNG on approve, so preview == export.
//
// Revisions live at brands/{brandId}/contents/{contentId}/designs/{revisionId}.
// This subcollection is CLIENT-WRITABLE (HTML is not sensitive) so deterministic
// text edits + render-capture happen frontend-side; the AI also writes via the
// Admin SDK (bypasses rules).

// ContentDesignRef is the lightweight pointer cached on the content doc.
type ContentDesignRef struct {
	RevisionID string `json:"revisionId" firestore:"revisionId"`
	DocType    string `json:"docType" firestore:"docType"` // image|video
	Width      int    `json:"width" firestore:"width"`     // per-slide width
	Height     int    `json:"height" firestore:"height"`   // per-slide height
	// SlideCount is the number of carousel slides (1 for a single post).
	SlideCount int `json:"slideCount" firestore:"slideCount"`
	// DurationMs is the animation length for a video design (0 for images).
	DurationMs int `json:"durationMs,omitempty" firestore:"durationMs,omitempty"`
	// RenderURL is the first frontend-captured PNG (cover) used for publish/Canva.
	RenderURL string `json:"renderUrl,omitempty" firestore:"renderUrl,omitempty"`
	UpdatedAt int64  `json:"updatedAt" firestore:"updatedAt"`
}

// ContentDesignRevision is one immutable HTML snapshot.
type ContentDesignRevision struct {
	ID     string `json:"id" firestore:"-"`
	HTML       string `json:"html" firestore:"html"`
	Width      int    `json:"width" firestore:"width"`   // per-slide width
	Height     int    `json:"height" firestore:"height"` // per-slide height
	SlideCount int    `json:"slideCount" firestore:"slideCount"`
	DurationMs int    `json:"durationMs,omitempty" firestore:"durationMs,omitempty"`
	DocType    string `json:"docType" firestore:"docType"` // image|video
	// Origin: "generate" | "edit" | "text" | "revert".
	Origin           string `json:"origin,omitempty" firestore:"origin,omitempty"`
	ParentRevisionID string `json:"parentRevisionId,omitempty" firestore:"parentRevisionId,omitempty"`
	RenderURL        string `json:"renderUrl,omitempty" firestore:"renderUrl,omitempty"`
	CreatedAt        int64  `json:"createdAt" firestore:"createdAt"`
}

func contentDesignsCollection(brandID, contentID string) *firestore.CollectionRef {
	return firestoredb.Client.Collection(
		fmt.Sprintf("brands/%s/contents/%s/designs", brandID, contentID))
}

// CreateDesignRevision writes a new immutable HTML design revision.
func CreateDesignRevision(brandID, contentID string, rev *ContentDesignRevision) (string, error) {
	if rev.CreatedAt == 0 {
		rev.CreatedAt = time.Now().UnixMilli()
	}
	ref, _, err := contentDesignsCollection(brandID, contentID).Add(context.Background(), rev)
	if err != nil {
		return "", fmt.Errorf("CreateDesignRevision: %w", err)
	}
	return ref.ID, nil
}

// GetDesignRevision reads one revision, or (nil, nil) if absent.
func GetDesignRevision(brandID, contentID, revisionID string) (*ContentDesignRevision, error) {
	doc, err := contentDesignsCollection(brandID, contentID).Doc(revisionID).Get(context.Background())
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, err
	}
	var r ContentDesignRevision
	if err := doc.DataTo(&r); err != nil {
		return nil, err
	}
	r.ID = doc.Ref.ID
	return &r, nil
}

// ListDesignRevisions returns a content's design revisions newest-first.
func ListDesignRevisions(brandID, contentID string, limit int) ([]ContentDesignRevision, error) {
	q := contentDesignsCollection(brandID, contentID).OrderBy("createdAt", firestore.Desc)
	if limit > 0 {
		q = q.Limit(limit)
	}
	iter := q.Documents(context.Background())
	defer iter.Stop()
	out := []ContentDesignRevision{}
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		var r ContentDesignRevision
		if err := doc.DataTo(&r); err != nil {
			return nil, err
		}
		r.ID = doc.Ref.ID
		out = append(out, r)
	}
	return out, nil
}

// SetContentDesignRef points the content at a design revision and stamps source.
func SetContentDesignRef(brandID, contentID string, ref *ContentDesignRef) error {
	ref.UpdatedAt = time.Now().UnixMilli()
	return UpdateContentFields(brandID, contentID, map[string]interface{}{
		"designRef": ref,
		"source":    "ai",
	})
}
