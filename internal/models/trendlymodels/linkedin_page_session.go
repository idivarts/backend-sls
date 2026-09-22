package trendlymodels

import (
	"context"
	"fmt"
	"time"

	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
)

// linkedInPageSessionTTL is how long a pending page-connect session stays valid
// between the OAuth callback and the user picking pages in the portal.
const linkedInPageSessionTTL = 10 * 60 // seconds

// LinkedInPageSessionOrg is one Company/Showcase Page the connecting member
// administers, offered in the portal picker. Token-free (safe to expose).
type LinkedInPageSessionOrg struct {
	URN        string `json:"urn" firestore:"urn"` // urn:li:organization:{id}
	ID         string `json:"id" firestore:"id"`
	Name       string `json:"name" firestore:"name"`
	VanityName string `json:"vanityName,omitempty" firestore:"vanityName,omitempty"`
	LogoURL    string `json:"logoUrl,omitempty" firestore:"logoUrl,omitempty"`
}

// LinkedInPageSession is a short-lived doc bridging the linkedin_page OAuth
// callback and the portal page-picker. It holds the shared member token (so the
// select step can create accounts) and the admin-org list (so the picker can
// render). The random doc id is the only capability needed to complete the flow,
// so it is single-use + TTL-bound. Server-only (tokens are sensitive).
type LinkedInPageSession struct {
	ID             string                   `json:"id" firestore:"-"`
	BrandID        string                   `json:"brandId" firestore:"brandId"`
	App            string                   `json:"app" firestore:"app"`
	CallbackScheme string                   `json:"callbackScheme" firestore:"callbackScheme"`
	UserID         string                   `json:"userId" firestore:"userId"`
	MemberID       string                   `json:"memberId" firestore:"memberId"`
	AccessToken    string                   `json:"-" firestore:"accessToken"`
	RefreshToken   string                   `json:"-" firestore:"refreshToken,omitempty"`
	TokenExpiry    int64                    `json:"-" firestore:"tokenExpiry"`
	Scopes         []string                 `json:"-" firestore:"scopes,omitempty"`
	Orgs           []LinkedInPageSessionOrg `json:"orgs" firestore:"orgs"`
	CreatedAt      int64                    `json:"createdAt" firestore:"createdAt"`
}

func linkedInPageSessionsCollection() string {
	return "linkedinPageSessions"
}

// CreateLinkedInPageSession writes a new pending session and returns its id.
func CreateLinkedInPageSession(id string, s *LinkedInPageSession) error {
	s.CreatedAt = time.Now().Unix()
	_, err := firestoredb.Client.
		Collection(linkedInPageSessionsCollection()).
		Doc(id).
		Set(context.Background(), s)
	if err != nil {
		return fmt.Errorf("CreateLinkedInPageSession: %w", err)
	}
	return nil
}

// GetLinkedInPageSession reads a pending session, erroring if it is missing or
// older than the TTL.
func GetLinkedInPageSession(id string) (*LinkedInPageSession, error) {
	doc, err := firestoredb.Client.
		Collection(linkedInPageSessionsCollection()).
		Doc(id).
		Get(context.Background())
	if err != nil {
		return nil, err
	}
	var s LinkedInPageSession
	if err := doc.DataTo(&s); err != nil {
		return nil, err
	}
	s.ID = doc.Ref.ID
	if time.Now().Unix()-s.CreatedAt > linkedInPageSessionTTL {
		return nil, fmt.Errorf("linkedin page session expired")
	}
	return &s, nil
}

// DeleteLinkedInPageSession removes a session (single-use cleanup).
func DeleteLinkedInPageSession(id string) error {
	_, err := firestoredb.Client.
		Collection(linkedInPageSessionsCollection()).
		Doc(id).
		Delete(context.Background())
	return err
}
