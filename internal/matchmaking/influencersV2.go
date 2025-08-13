package matchmaking

import (
	"context"
	"net/http"

	"cloud.google.com/go/bigquery"
	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/myquery"
	"github.com/idivarts/backend-sls/pkg/myutil"
	"google.golang.org/api/iterator"
)

const (
	sqlI2I = `SELECT id
FROM(
	SELECT *, IF(LOWER(location)=LOWER(@location), 100, IF((RAND()*20)>19, 100, 99)) as lRank,
	IF(reach_count>20000 AND follower_count>5000, 1, 0) as rRank
	FROM ` + "`trendly-9ab99.matches.influencers`" + ` 
	where completion_percentage>40
	AND id NOT IN ("MvLmVKwUcXXZXfBfQHSnq5udnaO2", "mmUwj1YlPUVn0h2hlN4qVw1bEZo1", "jEZf51INayY4ZcJs2ck0XWR8Ptj2")
)
order by lRank desc, rRank desc, last_use_time desc
LIMIT 100`
)

func GetInfluencerForInfluencer(c *gin.Context) {
	userId, b := middlewares.GetUserId(c)
	if !b {
		c.JSON(http.StatusBadRequest, gin.H{"error": "User not authenticated", "message": "UserId not found"})
		return
	}

	user := &trendlymodels.User{}
	err := user.Get(userId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "User not found"})
		return
	}

	influencers := []string{}
	if myutil.DerefString(user.Location) == "" {
		influencers, err = trendlymodels.GetInfluencerIDs(nil, 100)
	} else {
		influencers, err = RunBQ2(myutil.DerefString(user.Location))
	}

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Influencers not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": "Influencers found", "influencers": influencers})
}

func RunBQ2(location string) ([]string, error) {
	q := myquery.Client.Query(sqlI2I)
	q.Parameters = []bigquery.QueryParameter{
		{Name: "location", Value: location},
	}
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
