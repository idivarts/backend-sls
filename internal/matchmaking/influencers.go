package matchmaking

import (
	"context"
	"net/http"

	"cloud.google.com/go/bigquery"
	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/myquery"
	"google.golang.org/api/iterator"
)

type IBrandMember struct {
	BrandID string `json:"brandId" binding:"required"`
}

const (
	sql = `SELECT 
		id
		FROM ` +
		"`trendly-9ab99.matches.influencers`" +
		` ORDER BY reach_count desc, RAND()
		LIMIT 100`
)

func GetInfluencers(c *gin.Context) {
	var req IBrandMember
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Input is incorrect"})
		return
	}

	managerId, b := middlewares.GetUserId(c)
	if !b {
		c.JSON(http.StatusBadRequest, gin.H{"error": "User not authenticated", "message": "UserId not found"})
		return
	}

	membership := trendlymodels.BrandMember{}
	err := membership.Get(req.BrandID, managerId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Brand Membership not found"})
		return
	}

	brand := trendlymodels.Brand{}
	err = brand.Get(req.BrandID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Brand not found"})
		return
	}

	ids, err := RunBQ()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error Executing Query"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Succesfully fetched data", "data": ids})
}
func RunBQ() ([]string, error) {
	q := myquery.Client.Query(sql)
	data, err := q.Read(context.Background())
	if err != nil {
		return nil, err
	}

	ids := []string{}
	for {
		row := make(map[string]bigquery.Value)
		err := data.Next(&row)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}

		if idVal, ok := row["id"]; ok {
			if idStr, ok := idVal.(string); ok {
				ids = append(ids, idStr)
			}
		}
	}
	return ids, nil
}
