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
}

// CreateTeam creates a new (non-default) team. Requires manage_teams.
// POST /api/v2/brands/:brandId/teams
func CreateTeam(c *gin.Context) {
	brandID := c.Param("brandId")
	member, ok := requireBrandCapability(c, brandID, trendlymodels.CapManageTeams)
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
	}
	if _, err := team.Set(brandID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Unable to create team"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Team created", "team": team})
}

// UpdateTeam renames a team. Requires manage_teams.
// PATCH /api/v2/brands/:brandId/teams/:teamId
func UpdateTeam(c *gin.Context) {
	brandID := c.Param("brandId")
	teamID := c.Param("teamId")
	if _, ok := requireBrandCapability(c, brandID, trendlymodels.CapManageTeams); !ok {
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
	if _, err := team.Set(brandID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Unable to update team"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Team updated", "team": team})
}

// DeleteTeam removes a non-default team. Requires manage_teams.
// DELETE /api/v2/brands/:brandId/teams/:teamId
func DeleteTeam(c *gin.Context) {
	brandID := c.Param("brandId")
	teamID := c.Param("teamId")
	if _, ok := requireBrandCapability(c, brandID, trendlymodels.CapManageTeams); !ok {
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
	if err := trendlymodels.DeleteTeam(brandID, teamID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Unable to delete team"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Team deleted"})
}

type IAssignSocialTeam struct {
	TeamID string `json:"teamId" binding:"required"`
}

// AssignSocialTeam moves a connected brand social to a team. Requires manage_teams.
// POST /api/v2/brands/:brandId/socials/:id/team
func AssignSocialTeam(c *gin.Context) {
	brandID := c.Param("brandId")
	socialID := c.Param("id")
	if _, ok := requireBrandCapability(c, brandID, trendlymodels.CapManageTeams); !ok {
		return
	}
	var req IAssignSocialTeam
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate the target team exists in this brand.
	team := trendlymodels.Team{}
	if err := team.Get(brandID, req.TeamID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Target team not found"})
		return
	}
	if _, err := trendlymodels.AssignBrandSocialTeam(brandID, socialID, req.TeamID, time.Now().UnixMilli()); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Unable to assign social to team"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Social assigned to team"})
}
