package middlewares

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
)

// RequireBrandMembership loads the caller's membership in brandID. On failure it
// writes the HTTP error response and returns ok=false.
func RequireBrandMembership(c *gin.Context, brandID string) (*trendlymodels.BrandMember, bool) {
	userId, b := GetUserId(c)
	if !b {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "User not found"})
		return nil, false
	}
	member := &trendlymodels.BrandMember{}
	if err := member.Get(brandID, userId); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"message": "You are not a member of this brand", "error": err.Error()})
		return nil, false
	}
	return member, true
}

// RequireFeaturePrivilege loads the caller's membership, resolves their team,
// and enforces that the team grants priv under feature. Members not yet migrated
// to the team-privilege model (no team assigned) are allowed through during the
// transition — remove the fallback once scripts/migrate-teams-v2 has run.
func RequireFeaturePrivilege(c *gin.Context, brandID string, feature trendlymodels.Feature, priv trendlymodels.Privilege) (*trendlymodels.BrandMember, bool) {
	member, ok := RequireBrandMembership(c, brandID)
	if !ok {
		return nil, false
	}
	team, err := member.ResolveTeam(brandID)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"message": "Unable to resolve your team", "error": err.Error()})
		return nil, false
	}
	// Legacy member (pre-migration, no team) — allow through during transition.
	if team == nil {
		return member, true
	}
	if !team.HasPrivilege(feature, priv) {
		c.JSON(http.StatusForbidden, gin.H{"message": "You don't have permission to perform this action"})
		return nil, false
	}
	return member, true
}
