package trendlymodels

import (
	"context"
	"time"

	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// CanvaOAuthState is a short-lived record correlating an in-flight Canva OAuth
// attempt (the PKCE verifier + which brand-member started it) to the `state`
// returned on the public callback, which carries no Firebase session. Fully
// backend-only. Cleaned up on use.
type CanvaOAuthState struct {
	State     string `json:"state" firestore:"-"`
	ManagerID string `json:"managerId" firestore:"managerId"`
	BrandID   string `json:"brandId,omitempty" firestore:"brandId,omitempty"`
	Verifier  string `json:"-" firestore:"verifier"`
	CreatedAt int64  `json:"createdAt" firestore:"createdAt"`
}

const canvaOAuthStatesCollection = "canvaOauthStates"

// CreateCanvaOAuthState persists an in-flight auth attempt keyed by state.
func CreateCanvaOAuthState(s *CanvaOAuthState) error {
	s.CreatedAt = time.Now().UnixMilli()
	_, err := firestoredb.Client.Collection(canvaOAuthStatesCollection).
		Doc(s.State).Set(context.Background(), s)
	return err
}

// ConsumeCanvaOAuthState reads and deletes an auth attempt by state (single use).
// Returns (nil, nil) when absent/expired.
func ConsumeCanvaOAuthState(state string) (*CanvaOAuthState, error) {
	ref := firestoredb.Client.Collection(canvaOAuthStatesCollection).Doc(state)
	doc, err := ref.Get(context.Background())
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, err
	}
	var s CanvaOAuthState
	if err := doc.DataTo(&s); err != nil {
		return nil, err
	}
	s.State = doc.Ref.ID
	_, _ = ref.Delete(context.Background())
	return &s, nil
}
