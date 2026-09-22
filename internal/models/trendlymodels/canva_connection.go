package trendlymodels

import (
	"context"
	"fmt"
	"time"

	firestorepkg "cloud.google.com/go/firestore"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// CanvaConnection stores a brand-member's Canva Connect OAuth tokens. It is
// backend-only: the Firestore rules deny ALL client access (tokens must never
// reach the app). Keyed by managerId (one connection per brand-member).
type CanvaConnection struct {
	ID           string `json:"id" firestore:"-"`
	ManagerID    string `json:"managerId" firestore:"managerId"`
	BrandID      string `json:"brandId,omitempty" firestore:"brandId,omitempty"`
	AccessToken  string `json:"-" firestore:"accessToken"`  // secret — never serialized to clients
	RefreshToken string `json:"-" firestore:"refreshToken"` // secret, single-use rotated
	Scope        string `json:"scope,omitempty" firestore:"scope,omitempty"`
	// ExpiresAt is the access-token expiry (epoch ms). Refresh when near.
	ExpiresAt int64 `json:"expiresAt" firestore:"expiresAt"`
	CanvaUser string `json:"canvaUser,omitempty" firestore:"canvaUser,omitempty"`
	Connected bool   `json:"connected" firestore:"connected"`
	CreatedAt int64  `json:"createdAt" firestore:"createdAt"`
	UpdatedAt int64  `json:"updatedAt" firestore:"updatedAt"`
}

const canvaConnectionsCollection = "canvaConnections"

// UpsertCanvaConnection creates or overwrites a brand-member's connection.
func UpsertCanvaConnection(conn *CanvaConnection) error {
	now := time.Now().UnixMilli()
	if conn.CreatedAt == 0 {
		conn.CreatedAt = now
	}
	conn.UpdatedAt = now
	_, err := firestoredb.Client.Collection(canvaConnectionsCollection).
		Doc(conn.ManagerID).Set(context.Background(), conn)
	if err != nil {
		return fmt.Errorf("UpsertCanvaConnection: %w", err)
	}
	return nil
}

// GetCanvaConnection reads a brand-member's connection, or (nil, nil) if none.
func GetCanvaConnection(managerID string) (*CanvaConnection, error) {
	doc, err := firestoredb.Client.Collection(canvaConnectionsCollection).
		Doc(managerID).Get(context.Background())
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, err
	}
	var c CanvaConnection
	if err := doc.DataTo(&c); err != nil {
		return nil, err
	}
	c.ID = doc.Ref.ID
	return &c, nil
}

// UpdateCanvaTokens rotates the stored tokens after a refresh (single-use
// refresh tokens mean we must persist the new pair atomically).
func UpdateCanvaTokens(managerID, accessToken, refreshToken string, expiresAt int64) error {
	_, err := firestoredb.Client.Collection(canvaConnectionsCollection).Doc(managerID).Set(
		context.Background(),
		map[string]interface{}{
			"accessToken":  accessToken,
			"refreshToken": refreshToken,
			"expiresAt":    expiresAt,
			"connected":    true,
			"updatedAt":    time.Now().UnixMilli(),
		},
		firestorepkg.MergeAll,
	)
	if err != nil {
		return fmt.Errorf("UpdateCanvaTokens: %w", err)
	}
	return nil
}

// DeleteCanvaConnection removes a brand-member's connection (disconnect).
func DeleteCanvaConnection(managerID string) error {
	_, err := firestoredb.Client.Collection(canvaConnectionsCollection).
		Doc(managerID).Delete(context.Background())
	return err
}
