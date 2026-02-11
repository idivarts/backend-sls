package trendlydiscovery

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/models/trendlyrdb"
)

// GetNiches returns paginated niches with optional search filtering.
// Query params:
//   - offset (int, default 0)
//   - limit  (int, default 20, max 100)
//   - search (string, optional) — filters niches matching the search key
func GetNiches(c *gin.Context) {
	offset := 0
	if v := c.Query("offset"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed < 0 {
			c.JSON(400, gin.H{"message": "Invalid offset"})
			return
		}
		offset = parsed
	}

	limit := 20
	if v := c.Query("limit"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed <= 0 {
			c.JSON(400, gin.H{"message": "Invalid limit"})
			return
		}
		if parsed > 100 {
			parsed = 100
		}
		limit = parsed
	}

	searchKey := c.Query("search")

	niches, err := trendlyrdb.NicheCount{}.GetPaginated(offset, limit, searchKey)
	if err != nil {
		c.JSON(500, gin.H{"message": "Failed to fetch niches", "error": err.Error()})
		return
	}

	c.JSON(200, gin.H{"message": "Success", "data": niches})
}
