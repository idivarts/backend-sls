package trendlymodels

import (
	"context"
	"time"

	"cloud.google.com/go/firestore"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
)

type ManagerSettings struct {
	Theme             string `json:"theme,omitempty" firestore:"theme,omitempty"` // "light" or "dark"
	EmailNotification bool   `json:"emailNotification,omitempty" firestore:"emailNotification,omitempty"`
	PushNotification  bool   `json:"pushNotification,omitempty" firestore:"pushNotification,omitempty"`
}

type Manager struct {
	Name                  string                `json:"name" firestore:"name"`
	Email                 string                `json:"email" firestore:"email"`
	IsAdmin               bool                  `json:"isAdmin" firestore:"isAdmin"`
	PhoneNumber           string                `json:"phoneNumber,omitempty" firestore:"phoneNumber,omitempty"`
	Location              string                `json:"location,omitempty" firestore:"location,omitempty"`
	IsChatConnected       bool                  `json:"isChatConnected,omitempty" firestore:"isChatConnected,omitempty"`
	ProfileImage          string                `json:"profileImage,omitempty" firestore:"profileImage,omitempty"`
	Settings              *ManagerSettings      `json:"settings,omitempty" firestore:"settings,omitempty"`
	PushNotificationToken PushNotificationToken `json:"pushNotificationToken" firestore:"pushNotificationToken"`

	Moderations struct {
		BlockedInfluencers  []string `json:"blockedInfluencers,omitempty" firestore:"blockedInfluencers,omitempty"`
		ReportedInfluencers []string `json:"reportedInfluencers,omitempty" firestore:"reportedInfluencers,omitempty"`
	} `json:"moderations,omitempty" firestore:"moderations,omitempty"`

	CreationTime int64 `json:"creationTime" firestore:"creationTime"`

	// LastSeenPlatform is the client the manager most recently used, one of
	// "ios" | "android" | "web-desktop" | "web-mobile". Captured from the
	// X-Client-Platform request header (see middlewares.TrendlyMiddleware) and
	// refreshed at most hourly, so it is a coarse "how do they use us" signal
	// for the admin CRM — not a session log. Empty for managers who have not
	// made a request since this shipped.
	LastSeenPlatform string `json:"lastSeenPlatform,omitempty" firestore:"lastSeenPlatform,omitempty"`
	// LastSeenAt is when LastSeenPlatform was last stamped (epoch ms).
	LastSeenAt int64 `json:"lastSeenAt,omitempty" firestore:"lastSeenAt,omitempty"`

	// DeletedAt soft-deletes the manager account (epoch ms). Non-nil means the
	// user deleted their account (Firebase auth + Stream user are also removed and
	// every brand/org membership is stripped). Kept as an audit marker.
	DeletedAt *int64 `json:"deletedAt,omitempty" firestore:"deletedAt,omitempty"`
}

func (u *Manager) Get(managerId string) error {
	res, err := firestoredb.Client.Collection("managers").Doc(managerId).Get((context.Background()))
	if err != nil {
		return err
	}
	err = res.DataTo(u)
	if err != nil {
		return err
	}
	return err
}

func (u *Manager) Insert(managerId string) (*firestore.WriteResult, error) {
	wr, err := firestoredb.Client.Collection("managers").Doc(managerId).Set(context.Background(), u)
	return wr, err
}

// LastSeenRefreshInterval is how stale LastSeenAt must be before the middleware
// bothers writing it again. This runs on every authenticated manager request,
// so the throttle is what keeps it from becoming a write per request.
const LastSeenRefreshInterval = int64(time.Hour / time.Millisecond)

// ShouldRefreshLastSeen reports whether the manager's stored device info is
// worth rewriting: the platform changed, or the last stamp has aged out.
func (u *Manager) ShouldRefreshLastSeen(platform string, now int64) bool {
	if platform == "" {
		return false
	}
	if u.LastSeenPlatform != platform {
		return true
	}
	return now-u.LastSeenAt >= LastSeenRefreshInterval
}

// TouchManagerLastSeen records which client the manager is currently using.
// Callers must gate this with ShouldRefreshLastSeen.
func TouchManagerLastSeen(ctx context.Context, managerID, platform string, at int64) error {
	_, err := firestoredb.Client.Collection("managers").Doc(managerID).
		Update(ctx, []firestore.Update{
			{Path: "lastSeenPlatform", Value: platform},
			{Path: "lastSeenAt", Value: at},
		})
	return err
}

// SoftDeleteManager stamps deletedAt on the manager doc. The DeleteManager
// handler also revokes Firebase auth, deletes the Stream user, and strips every
// brand/org membership.
func SoftDeleteManager(managerID string, deletedAt int64) error {
	_, err := firestoredb.Client.Collection("managers").Doc(managerID).
		Update(context.Background(), []firestore.Update{{Path: "deletedAt", Value: deletedAt}})
	return err
}
