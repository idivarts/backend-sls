package trendlyapis

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
)

// Brand members are read directly from Firestore by the apps
// (brands/{brandId}/members); only mutations live here.

type IUpdateMember struct {
	// TeamID is the single team to move the member to. Must be a team of this brand.
	TeamID *string `json:"teamId"`
}

// UpdateBrandMember moves a member to a different team. The member inherits that
// team's feature privileges. Requires brand_admin:members. The brand must always
// retain at least one member who can manage members.
// PATCH /api/v2/brands/:brandId/members/:managerId
func UpdateBrandMember(c *gin.Context) {
	brandID := c.Param("brandId")
	targetID := c.Param("managerId")
	if _, ok := requireFeaturePrivilege(c, brandID, trendlymodels.FeatureBrandAdmin, trendlymodels.PrivAdminMembers); !ok {
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

	if req.TeamID != nil {
		newTeam := &trendlymodels.Team{}
		if err := newTeam.Get(brandID, *req.TeamID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Target team not found"})
			return
		}
		// Guard: don't strip the last members-admin by moving them to a team that
		// can't manage members.
		if !newTeam.HasPrivilege(trendlymodels.FeatureBrandAdmin, trendlymodels.PrivAdminMembers) {
			admins, err := countMembersWithPrivilege(brandID, trendlymodels.FeatureBrandAdmin, trendlymodels.PrivAdminMembers)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			if admins <= 1 {
				c.JSON(http.StatusBadRequest, gin.H{"message": "Cannot move the last member who can manage members to a team without that access"})
				return
			}
		}
		member.TeamID = *req.TeamID
	}

	if _, err := member.Set(brandID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Unable to update member"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Member updated", "member": member})
}

// RemoveBrandMember removes a member from the brand. Requires brand_admin:members.
// The last member who can manage members cannot be removed.
// DELETE /api/v2/brands/:brandId/members/:managerId
func RemoveBrandMember(c *gin.Context) {
	brandID := c.Param("brandId")
	targetID := c.Param("managerId")
	if _, ok := requireFeaturePrivilege(c, brandID, trendlymodels.FeatureBrandAdmin, trendlymodels.PrivAdminMembers); !ok {
		return
	}

	member := &trendlymodels.BrandMember{}
	if err := member.Get(brandID, targetID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Member not found"})
		return
	}
	// Guard: the brand must always keep at least one member who can manage members.
	team, terr := member.ResolveTeam(brandID)
	if terr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": terr.Error(), "message": "Unable to resolve member's team"})
		return
	}
	targetIsAdmin := team == nil || team.HasPrivilege(trendlymodels.FeatureBrandAdmin, trendlymodels.PrivAdminMembers)
	if targetIsAdmin {
		admins, err := countMembersWithPrivilege(brandID, trendlymodels.FeatureBrandAdmin, trendlymodels.PrivAdminMembers)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if admins <= 1 {
			c.JSON(http.StatusBadRequest, gin.H{"message": "Cannot remove the last member who can manage members"})
			return
		}
	}

	if err := trendlymodels.DeleteBrandMember(brandID, targetID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Unable to remove member"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Member removed"})
}
