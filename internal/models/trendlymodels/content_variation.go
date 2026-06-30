package trendlymodels

import (
	"context"
	"fmt"

	"cloud.google.com/go/firestore"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ContentVariation mirrors a per-platform override document at
// brands/{brandId}/contents/{contentId}/variations/{platform}. The document ID
// is the platform key, so there is at most one variation per platform.
//
// A variation is an OVERRIDE, not a snapshot: only the shared fields listed in
// OverriddenFields are taken from the variation; everything else inherits from
// the generic content. Platform-specific options always live on the variation.
type ContentVariation struct {
	ID       string `json:"id,omitempty" firestore:"-"`
	Platform string `json:"platform" firestore:"platform"`

	// Overridable copies of the generic content (only meaningful when the field
	// name is present in OverriddenFields).
	Caption     string              `json:"caption,omitempty" firestore:"caption,omitempty"`
	Hashtags    string              `json:"hashtags,omitempty" firestore:"hashtags,omitempty"`
	Attachments []ContentAttachment `json:"attachments,omitempty" firestore:"attachments,omitempty"`

	// Shared fields the user explicitly overrode ("caption" | "hashtags" |
	// "attachments"). Anything else inherits from the generic content.
	OverriddenFields []string `json:"overriddenFields,omitempty" firestore:"overriddenFields,omitempty"`

	PlatformOptions *ContentPlatformOptions `json:"platformOptions,omitempty" firestore:"platformOptions,omitempty"`

	CreatedAt int64 `json:"createdAt,omitempty" firestore:"createdAt"`
	UpdatedAt int64 `json:"updatedAt,omitempty" firestore:"updatedAt"`
}

func variationsCollection(brandID, contentID string) *firestore.CollectionRef {
	return firestoredb.Client.Collection(
		fmt.Sprintf("brands/%s/contents/%s/variations", brandID, contentID),
	)
}

// GetContentVariation reads the variation for one platform, or (nil, nil) when
// none exists (a missing variation is not an error — the platform falls back to
// the generic content).
func GetContentVariation(brandID, contentID, platform string) (*ContentVariation, error) {
	doc, err := variationsCollection(brandID, contentID).Doc(platform).Get(context.Background())
	if err != nil {
		// NotFound → no variation for this platform; treat as nil (not an error).
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, err
	}
	if !doc.Exists() {
		return nil, nil
	}
	var v ContentVariation
	if err := doc.DataTo(&v); err != nil {
		return nil, err
	}
	v.ID = doc.Ref.ID
	return &v, nil
}

// ListContentVariations returns all variations for a content, keyed by platform.
func ListContentVariations(brandID, contentID string) (map[string]*ContentVariation, error) {
	iter := variationsCollection(brandID, contentID).Documents(context.Background())
	defer iter.Stop()

	out := map[string]*ContentVariation{}
	for {
		doc, err := iter.Next()
		if err != nil {
			break
		}
		var v ContentVariation
		if err := doc.DataTo(&v); err != nil {
			continue
		}
		v.ID = doc.Ref.ID
		out[doc.Ref.ID] = &v
	}
	return out, nil
}

// EffectiveForPlatform returns a copy of the content with the variation's
// overrides applied for one platform. Shared fields are taken from the variation
// only when listed in OverriddenFields; platform options are taken from the
// variation whenever it has them. A nil variation returns the generic content
// unchanged (full backward-compat).
func (ct *Content) EffectiveForPlatform(v *ContentVariation) *Content {
	eff := *ct // shallow copy — slices/pointers are swapped wholesale below
	if v == nil {
		return &eff
	}
	overridden := map[string]bool{}
	for _, f := range v.OverriddenFields {
		overridden[f] = true
	}
	if overridden["caption"] {
		eff.Caption = v.Caption
	}
	if overridden["hashtags"] {
		eff.Hashtags = v.Hashtags
	}
	if overridden["attachments"] {
		eff.Attachments = v.Attachments
	}
	if v.PlatformOptions != nil {
		eff.PlatformOptions = v.PlatformOptions
	}
	return &eff
}
