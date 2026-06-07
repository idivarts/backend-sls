package trendlyapis

import (
	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
)

// requireBrandMembership / requireFeaturePrivilege are thin package-local aliases
// over the canonical guards in middlewares, kept so handlers in this package
// read cleanly.
func requireBrandMembership(c *gin.Context, brandID string) (*trendlymodels.BrandMember, bool) {
	return middlewares.RequireBrandMembership(c, brandID)
}

func requireFeaturePrivilege(c *gin.Context, brandID string, feature trendlymodels.Feature, priv trendlymodels.Privilege) (*trendlymodels.BrandMember, bool) {
	return middlewares.RequireFeaturePrivilege(c, brandID, feature, priv)
}

// countMembersWithPrivilege returns how many of the brand's members sit on a
// team that grants priv under feature. Members not yet migrated (no team) are
// counted as holding the privilege, so transition state can't trip the guard.
// Used to prevent a brand from locking itself out of administration.
func countMembersWithPrivilege(brandID string, feature trendlymodels.Feature, priv trendlymodels.Privilege) (int, error) {
	members, err := trendlymodels.GetAllBrandMembers(brandID)
	if err != nil {
		return 0, err
	}
	teamCache := map[string]*trendlymodels.Team{}
	count := 0
	for i := range members {
		teamID := members[i].TeamID
		if teamID == "" {
			count++
			continue
		}
		team, ok := teamCache[teamID]
		if !ok {
			t := &trendlymodels.Team{}
			if err := t.Get(brandID, teamID); err != nil {
				continue
			}
			team = t
			teamCache[teamID] = t
		}
		if team.HasPrivilege(feature, priv) {
			count++
		}
	}
	return count, nil
}
