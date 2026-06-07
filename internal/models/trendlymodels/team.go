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

// EnsureDefaultTeam returns the brand's default team ID, creating it if none
// exists yet. It is idempotent and safe to call on every brand-setup path.
func EnsureDefaultTeam(brandID, createdBy string, now int64) (string, error) {
	teams, err := GetAllTeams(brandID)
	if err != nil {
		return "", err
	}
	for _, t := range teams {
		if t.IsDefault {
			return t.ID, nil
		}
	}

	ref := teamsCol(brandID).NewDoc()
	team := Team{
		ID:           ref.ID,
		Name:         "Default",
		IsDefault:    true,
		CreatedBy:    createdBy,
		CreationTime: now,
		// The default team always holds full access.
		Privileges: AllFeaturePrivilegesMap(),
	}
	if _, err := team.Set(brandID); err != nil {
		return "", err
	}
	return ref.ID, nil
}
