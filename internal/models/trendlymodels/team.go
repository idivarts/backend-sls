package trendlymodels

import (
	"context"

	"cloud.google.com/go/firestore"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
	"google.golang.org/api/iterator"
)

// Team is an access unit within a brand. A team holds, per feature, the set of
// privileges its members inherit. Every brand has exactly one default team that
// always holds full access (all features + all privileges) so the brand can
// never be locked out of its own settings.
// Stored at brands/{brandId}/teams/{teamId}.
type Team struct {
	ID           string `json:"id" firestore:"id"`
	Name         string `json:"name" firestore:"name"`
	IsDefault    bool   `json:"isDefault" firestore:"isDefault"`
	CreatedBy    string `json:"createdBy,omitempty" firestore:"createdBy,omitempty"`
	CreationTime int64  `json:"creationTime" firestore:"creationTime"`
	// Privileges maps each granted feature to the set of privilege keys the team
	// holds for it. A feature absent from the map (or with an empty list) grants
	// no access to that feature. Keys/values mirror Feature / Privilege.
	Privileges map[string][]string `json:"privileges,omitempty" firestore:"privileges,omitempty"`
}

// HasPrivilege reports whether the team grants priv under feature.
func (t *Team) HasPrivilege(feature Feature, priv Privilege) bool {
	if t.Privileges == nil {
		return false
	}
	for _, p := range t.Privileges[string(feature)] {
		if p == string(priv) {
			return true
		}
	}
	return false
}

// HasFeature reports whether the team grants any privilege under feature (used
// to decide whether a feature's navigation/area is visible at all).
func (t *Team) HasFeature(feature Feature) bool {
	return len(t.Privileges[string(feature)]) > 0
}

func teamsCol(brandID string) *firestore.CollectionRef {
	return firestoredb.Client.Collection("brands").Doc(brandID).Collection("teams")
}

// NewTeamRef allocates a new team document ref with a generated ID.
func NewTeamRef(brandID string) *firestore.DocumentRef {
	return teamsCol(brandID).NewDoc()
}

func (t *Team) Set(brandID string) (*firestore.WriteResult, error) {
	return teamsCol(brandID).Doc(t.ID).Set(context.Background(), t)
}

func (t *Team) Get(brandID, teamID string) error {
	doc, err := teamsCol(brandID).Doc(teamID).Get(context.Background())
	if err != nil {
		return err
	}
	return doc.DataTo(t)
}

func GetAllTeams(brandID string) ([]Team, error) {
	var teams []Team
	iter := teamsCol(brandID).Documents(context.Background())
	defer iter.Stop()

	for {
		doc, err := iter.Next()
		if err != nil {
			if err == iterator.Done {
				break
			}
			return nil, err
		}

		var team Team
		if err := doc.DataTo(&team); err != nil {
			return nil, err
		}
		teams = append(teams, team)
	}
	return teams, nil
}

func DeleteTeam(brandID, teamID string) error {
	_, err := teamsCol(brandID).Doc(teamID).Delete(context.Background())
	return err
}

// defaultTeamSpec describes one of the teams seeded for every new brand. The
// first spec (Admin) is the team used wherever a single team is needed for
// auto-assignment (the brand creator, invited members, etc.).
type defaultTeamSpec struct {
	name       string
	privileges map[string][]string
}

// defaultTeamSpecs is the ordered set of default teams a new brand receives:
//   - Admin  — full access (all features + privileges).
//   - Editor — full access except brand administration.
//   - Viewer — view-only access.
//
// Admin must stay first; callers rely on teams[0] for auto-assignment.
func defaultTeamSpecs() []defaultTeamSpec {
	return []defaultTeamSpec{
		{name: "Admin", privileges: AllFeaturePrivilegesMap()},
		{name: "Editor", privileges: FeaturePrivilegesMapExcept(FeatureBrandAdmin)},
		{name: "Viewer", privileges: ViewOnlyFeaturePrivilegesMap()},
	}
}

// EnsureDefaultTeam returns the brand's default teams, creating them if none
// exist yet. New brands get three default teams — Admin, Editor and Viewer (see
// defaultTeamSpecs) — and the Admin team is always returned first, so callers
// that need a single team for auto-assignment should use teams[0].
//
// It is idempotent and safe to call on every brand-setup path: if any default
// team already exists (e.g. older brands seeded with a single default team), the
// existing default teams are returned untouched and nothing new is created — no
// backfill happens here.
func EnsureDefaultTeam(brandID, createdBy string, now int64) ([]Team, error) {
	teams, err := GetAllTeams(brandID)
	if err != nil {
		return nil, err
	}
	var existing []Team
	for _, t := range teams {
		if t.IsDefault {
			existing = append(existing, t)
		}
	}
	if len(existing) > 0 {
		// Keep the Admin team first so callers can rely on teams[0] for
		// auto-assignment, regardless of Firestore iteration order.
		for i := range existing {
			if existing[i].Name == "Admin" && i != 0 {
				existing[0], existing[i] = existing[i], existing[0]
				break
			}
		}
		return existing, nil
	}

	var created []Team
	for _, spec := range defaultTeamSpecs() {
		ref := teamsCol(brandID).NewDoc()
		team := Team{
			ID:           ref.ID,
			Name:         spec.name,
			IsDefault:    true,
			CreatedBy:    createdBy,
			CreationTime: now,
			Privileges:   spec.privileges,
		}
		if _, err := team.Set(brandID); err != nil {
			return nil, err
		}
		created = append(created, team)
	}
	return created, nil
}
