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

// RequireBrandCapability loads the caller's membership and enforces that they
// hold cap. Legacy members (pre-RBAC backfill, invalid role) are allowed through
// during the transition — remove the fallback once the role migration has run.
func RequireBrandCapability(c *gin.Context, brandID string, cap trendlymodels.Capability) (*trendlymodels.BrandMember, bool) {
	member, ok := RequireBrandMembership(c, brandID)
	if !ok {
		return nil, false
	}
	if member.Role.IsValid() && !member.HasCapability(cap) {
		c.JSON(http.StatusForbidden, gin.H{"message": "You don't have permission to perform this action"})
		return nil, false
	}
	return member, true
}
