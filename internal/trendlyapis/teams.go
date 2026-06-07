package trendlyapis

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
)

// ListTeams returns all teams for a brand. Any brand member may read.
// GET /api/v2/brands/:brandId/teams
func ListTeams(c *gin.Context) {
	brandID := c.Param("brandId")
	if _, ok := requireBrandMembership(c, brandID); !ok {
		return
	}
	teams, err := trendlymodels.GetAllTeams(brandID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Unable to list teams"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"teams": teams})
}

type ITeamUpsert struct {
	Name string `json:"name" binding:"required"`
	// Privileges maps feature → granted privilege keys. Validated against the
	// feature/privilege taxonomy; unknown features/privileges are dropped.
	Privileges map[string][]string `json:"privileges"`
}

// sanitizePrivileges keeps only known features and, within each, only valid
// privileges for that feature.
func sanitizePrivileges(in map[string][]string) map[string][]string {
	out := map[string][]string{}
	for feature, privs := range in {
		f := trendlymodels.Feature(feature)
		if !f.IsValid() {
			continue
		}
		valid := trendlymodels.FilterValidPrivileges(f, privs)
		if len(valid) > 0 {
			out[feature] = valid
		}
	}
	return out
}

// CreateTeam creates a new (non-default) team with its feature privileges.
// Requires brand_admin:teams.
// POST /api/v2/brands/:brandId/teams
func CreateTeam(c *gin.Context) {
	brandID := c.Param("brandId")
	member, ok := requireFeaturePrivilege(c, brandID, trendlymodels.FeatureBrandAdmin, trendlymodels.PrivAdminTeams)
	if !ok {
		return
	}
	var req ITeamUpsert
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ref := trendlymodels.NewTeamRef(brandID)
	team := trendlymodels.Team{
		ID:           ref.ID,
		Name:         req.Name,
		IsDefault:    false,
		CreatedBy:    member.ManagerID,
		CreationTime: time.Now().UnixMilli(),
		Privileges:   sanitizePrivileges(req.Privileges),
	}
	if _, err := team.Set(brandID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Unable to create team"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Team created", "team": team})
}

// UpdateTeam renames a team and/or updates its feature privileges.
// Requires brand_admin:teams. The default team always retains full access — its
// privileges cannot be reduced.
// PATCH /api/v2/brands/:brandId/teams/:teamId
func UpdateTeam(c *gin.Context) {
	brandID := c.Param("brandId")
	teamID := c.Param("teamId")
	if _, ok := requireFeaturePrivilege(c, brandID, trendlymodels.FeatureBrandAdmin, trendlymodels.PrivAdminTeams); !ok {
		return
	}
	var req ITeamUpsert
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	team := trendlymodels.Team{}
	if err := team.Get(brandID, teamID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Team not found"})
		return
	}
	team.Name = req.Name
	// The default team always keeps full access; only non-default teams can have
	// their privileges edited.
	if !team.IsDefault && req.Privileges != nil {
		team.Privileges = sanitizePrivileges(req.Privileges)
	} else if team.IsDefault {
		team.Privileges = trendlymodels.AllFeaturePrivilegesMap()
	}
	if _, err := team.Set(brandID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Unable to update team"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Team updated", "team": team})
}

// DeleteTeam removes a non-default team. Requires brand_admin:teams.
// DELETE /api/v2/brands/:brandId/teams/:teamId
func DeleteTeam(c *gin.Context) {
	brandID := c.Param("brandId")
	teamID := c.Param("teamId")
	if _, ok := requireFeaturePrivilege(c, brandID, trendlymodels.FeatureBrandAdmin, trendlymodels.PrivAdminTeams); !ok {
		return
	}

	team := trendlymodels.Team{}
	if err := team.Get(brandID, teamID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Team not found"})
		return
	}
	if team.IsDefault {
		c.JSON(http.StatusBadRequest, gin.H{"message": "The default team cannot be deleted"})
		return
	}

	// Reassign any members on this team to the default team so they aren't
	// orphaned (an orphaned teamId would lock the member out).
	defTeamID, derr := trendlymodels.EnsureDefaultTeam(brandID, "", time.Now().UnixMilli())
	if derr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": derr.Error(), "message": "Unable to resolve default team"})
		return
	}
	members, merr := trendlymodels.GetAllBrandMembers(brandID)
	if merr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": merr.Error(), "message": "Unable to list members"})
		return
	}
	for i := range members {
		if members[i].TeamID == teamID {
			members[i].TeamID = defTeamID
			if _, err := members[i].Set(brandID); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Unable to reassign team members"})
				return
			}
		}
	}

	if err := trendlymodels.DeleteTeam(brandID, teamID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Unable to delete team"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Team deleted"})
}
