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

// GeneratedAudio is an ElevenLabs-generated music bed or voiceover, owned by the
// backend (Admin SDK writes only) and stored under a brand for brand-scoped
// reads. The bytes live in S3/CloudFront; this doc holds metadata + the URL so
// the app can preview and the render worker can mux.
type GeneratedAudio struct {
	ID       string `json:"id" firestore:"-"`
	BrandID  string `json:"brandId" firestore:"brandId"`
	Kind     string `json:"kind" firestore:"kind"`               // music|voiceover
	Provider string `json:"provider" firestore:"provider"`       // elevenlabs
	Prompt   string `json:"prompt,omitempty" firestore:"prompt"` // music prompt / voiceover text
	VoiceID  string `json:"voiceId,omitempty" firestore:"voiceId,omitempty"`
	Language string `json:"language,omitempty" firestore:"language,omitempty"`
	URL      string `json:"url" firestore:"url"`
	DurationMs int64 `json:"durationMs,omitempty" firestore:"durationMs,omitempty"`
	// Status: "ready" (URL populated) | "failed".
	Status    string `json:"status" firestore:"status"`
	Error     string `json:"error,omitempty" firestore:"error,omitempty"`
	CreatedAt int64  `json:"createdAt" firestore:"createdAt"`
}

func generatedAudioCollection(brandID string) *firestore.CollectionRef {
	return firestoredb.Client.Collection(fmt.Sprintf("brands/%s/generatedAudio", brandID))
}

// CreateGeneratedAudio writes a generated-audio doc and returns its id.
func CreateGeneratedAudio(a *GeneratedAudio) (string, error) {
	if a.CreatedAt == 0 {
		a.CreatedAt = time.Now().UnixMilli()
	}
	ref, _, err := generatedAudioCollection(a.BrandID).Add(context.Background(), a)
	if err != nil {
		return "", fmt.Errorf("CreateGeneratedAudio: %w", err)
	}
	return ref.ID, nil
}

// GetGeneratedAudio reads one audio doc, or (nil, nil) if absent.
func GetGeneratedAudio(brandID, audioID string) (*GeneratedAudio, error) {
	doc, err := generatedAudioCollection(brandID).Doc(audioID).Get(context.Background())
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, err
	}
	var a GeneratedAudio
	if err := doc.DataTo(&a); err != nil {
		return nil, err
	}
	a.ID = doc.Ref.ID
	return &a, nil
}

// ListGeneratedAudio returns a brand's generated audio newest-first, optionally
// filtered by kind ("" = all). Capped by limit (0 = all).
func ListGeneratedAudio(brandID, kind string, limit int) ([]GeneratedAudio, error) {
	q := generatedAudioCollection(brandID).OrderBy("createdAt", firestore.Desc)
	if kind != "" {
		q = generatedAudioCollection(brandID).Where("kind", "==", kind).OrderBy("createdAt", firestore.Desc)
	}
	if limit > 0 {
		q = q.Limit(limit)
	}
	iter := q.Documents(context.Background())
	defer iter.Stop()
	out := []GeneratedAudio{}
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		var a GeneratedAudio
		if err := doc.DataTo(&a); err != nil {
			return nil, err
		}
		a.ID = doc.Ref.ID
		out = append(out, a)
	}
	return out, nil
}
