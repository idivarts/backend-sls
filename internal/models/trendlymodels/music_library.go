package trendlymodels

import (
	"context"
	"fmt"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// MusicTrack is one curated, commercially-licensed track in the shared music
// catalog (`musicLibrary`). Written only by the seed script (Admin SDK); read by
// any authenticated user for the soundtrack browser.
type MusicTrack struct {
	ID         string   `json:"id" firestore:"-"`
	Title      string   `json:"title" firestore:"title"`
	Moods      []string `json:"moods" firestore:"moods"` // upbeat|cinematic|corporate|chill|dramatic…
	URL        string   `json:"url" firestore:"url"`
	DurationMs int64    `json:"durationMs" firestore:"durationMs"`
	Provider   string   `json:"provider" firestore:"provider"` // elevenlabs
	CreatedAt  int64    `json:"createdAt" firestore:"createdAt"`
}

const musicLibraryCollection = "musicLibrary"

// CreateMusicTrack adds a catalog track (seed script).
func CreateMusicTrack(t *MusicTrack) (string, error) {
	if t.CreatedAt == 0 {
		t.CreatedAt = time.Now().UnixMilli()
	}
	ref, _, err := firestoredb.Client.Collection(musicLibraryCollection).Add(context.Background(), t)
	if err != nil {
		return "", fmt.Errorf("CreateMusicTrack: %w", err)
	}
	return ref.ID, nil
}

// GetMusicTrack reads one catalog track, or (nil, nil) if absent.
func GetMusicTrack(id string) (*MusicTrack, error) {
	doc, err := firestoredb.Client.Collection(musicLibraryCollection).Doc(id).Get(context.Background())
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, err
	}
	var t MusicTrack
	if err := doc.DataTo(&t); err != nil {
		return nil, err
	}
	t.ID = doc.Ref.ID
	return &t, nil
}

// ListMusicLibrary returns catalog tracks, optionally filtered by mood, then
// text-filtered by query on the title (client-grade search over a small catalog).
func ListMusicLibrary(mood, query string, limit int) ([]MusicTrack, error) {
	q := firestoredb.Client.Collection(musicLibraryCollection).OrderBy("createdAt", firestore.Desc)
	if mood != "" {
		q = firestoredb.Client.Collection(musicLibraryCollection).Where("moods", "array-contains", mood)
	}
	if limit > 0 {
		q = q.Limit(limit)
	}
	iter := q.Documents(context.Background())
	defer iter.Stop()
	out := []MusicTrack{}
	needle := strings.ToLower(strings.TrimSpace(query))
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		var t MusicTrack
		if err := doc.DataTo(&t); err != nil {
			return nil, err
		}
		t.ID = doc.Ref.ID
		if needle != "" && !strings.Contains(strings.ToLower(t.Title), needle) &&
			!containsFold(t.Moods, needle) {
			continue
		}
		out = append(out, t)
	}
	return out, nil
}

func containsFold(items []string, needle string) bool {
	for _, i := range items {
		if strings.Contains(strings.ToLower(i), needle) {
			return true
		}
	}
	return false
}
