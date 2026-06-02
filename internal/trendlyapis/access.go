package trendlyapis

import (
	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
)

// requireBrandMembership / requireBrandCapability are thin package-local aliases
// over the canonical guards in middlewares, kept so handlers in this package
// read cleanly.
func requireBrandMembership(c *gin.Context, brandID string) (*trendlymodels.BrandMember, bool) {
	return middlewares.RequireBrandMembership(c, brandID)
}

func requireBrandCapability(c *gin.Context, brandID string, cap trendlymodels.Capability) (*trendlymodels.BrandMember, bool) {
	return middlewares.RequireBrandCapability(c, brandID, cap)
}

// countBrandOwners returns how many members of the brand hold the Owner role.
func countBrandOwners(brandID string) (int, error) {
	members, err := trendlymodels.GetAllBrandMembers(brandID)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, m := range members {
		if m.Role == trendlymodels.RoleOwner {
			count++
		}
	}
	return count, nil
}
