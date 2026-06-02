package trendlyapis

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
)

// ListBrandMembers returns all members of a brand with their roles. Any brand
// member may read.
// GET /api/v2/brands/:brandId/members
func ListBrandMembers(c *gin.Context) {
	brandID := c.Param("brandId")
	if _, ok := requireBrandMembership(c, brandID); !ok {
		return
	}
	members, err := trendlymodels.GetAllBrandMembers(brandID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Unable to list members"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"members": members})
}

type IUpdateMember struct {
	Role      *string          `json:"role"`
	TeamIDs   *[]string        `json:"teamIds"`
	Overrides *map[string]bool `json:"overrides"`
}

// UpdateBrandMember updates a member's role, teams, and/or override toggles.
// Requires manage_members. The last Owner cannot be downgraded.
// PATCH /api/v2/brands/:brandId/members/:managerId
func UpdateBrandMember(c *gin.Context) {
	brandID := c.Param("brandId")
	targetID := c.Param("managerId")
	if _, ok := requireBrandCapability(c, brandID, trendlymodels.CapManageMembers); !ok {
		return
	}
	var req IUpdateMember
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	member := &trendlymodels.BrandMember{}
	if err := member.Get(brandID, targetID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Member not found"})
		return
	}

	if req.Role != nil {
		newRole := trendlymodels.BrandRole(*req.Role)
		if !newRole.IsValid() {
			c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid role"})
			return
		}
		// Guard the last Owner from being downgraded.
		if member.Role == trendlymodels.RoleOwner && newRole != trendlymodels.RoleOwner {
			owners, err := countBrandOwners(brandID)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			if owners <= 1 {
				c.JSON(http.StatusBadRequest, gin.H{"message": "Cannot remove the last owner — transfer ownership first"})
				return
			}
		}
		member.Role = newRole
	}

	if req.TeamIDs != nil {
		member.TeamIDs = *req.TeamIDs
	}

	if req.Overrides != nil {
		overrides := map[string]bool{}
		for k, v := range *req.Overrides {
			if trendlymodels.Capability(k).IsOverridable() {
				overrides[k] = v
			}
		}
		member.Overrides = overrides
	}

	if _, err := member.Set(brandID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Unable to update member"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Member updated", "member": member})
}

// RemoveBrandMember removes a member from the brand. Requires manage_members.
// The last Owner cannot be removed.
// DELETE /api/v2/brands/:brandId/members/:managerId
func RemoveBrandMember(c *gin.Context) {
	brandID := c.Param("brandId")
	targetID := c.Param("managerId")
	if _, ok := requireBrandCapability(c, brandID, trendlymodels.CapManageMembers); !ok {
		return
	}

	member := &trendlymodels.BrandMember{}
	if err := member.Get(brandID, targetID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Member not found"})
		return
	}
	if member.Role == trendlymodels.RoleOwner {
		owners, err := countBrandOwners(brandID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if owners <= 1 {
			c.JSON(http.StatusBadRequest, gin.H{"message": "Cannot remove the last owner — transfer ownership first"})
			return
		}
	}

	if err := trendlymodels.DeleteBrandMember(brandID, targetID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Unable to remove member"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Member removed"})
}
