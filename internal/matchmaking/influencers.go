package matchmaking

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"cloud.google.com/go/bigquery"
	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
	"github.com/idivarts/backend-sls/pkg/myquery"
	"google.golang.org/api/iterator"
)

type IBrandMember struct {
	BrandID string `form:"brandId" binding:"required"`
}

const (
	sqlFmt = `select id
from (
	SELECT id,
		ARRAY_AGG(distinct location) AS locations,
		ARRAY_AGG(distinct category) AS categories,
		ARRAY_AGG(distinct language) AS languages,
		ANY_VALUE(rRank) AS rRank,
		ANY_VALUE(last_use_time) AS last_use_time
	FROM(
		SELECT *,
		IF(reach_count>20000 AND follower_count>1000, 1, 0) as rRank
		FROM ` + "`trendly-9ab99.matches.influencers`" + ` 
		LEFT JOIN UNNEST(categories) as category
		LEFT JOIN UNNEST(languages) as language
		where completion_percentage>40
		%s
		%s
		%s
	)
	group by id
	order by rRank desc, last_use_time desc
)
LIMIT 100`
)

type ExploreInfluencerCache struct {
	Time int64    `json:"time" firestore:"time"`
	IDs  []string `json:"ids" firestore:"ids"`
}

func GetInfluencers(c *gin.Context) {
	var req IBrandMember
	if err := c.ShouldBindQuery(&req); err != nil {
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

	cacheKey := "explore-influencer-cache"
	ids := []string{}
	cachedData := ExploreInfluencerCache{}

	userSnap, err := firestoredb.Client.Collection("cached").Doc(cacheKey).Get(context.Background())
	if err == nil {
		err = userSnap.DataTo(&cachedData)
		if err == nil {
			if len(cachedData.IDs) > 0 && cachedData.Time > time.Now().Add(-6*time.Hour).UnixMilli() {
				ids = cachedData.IDs
			}
		}
	}

	if len(ids) > 0 {
		c.JSON(http.StatusOK, gin.H{"message": "Succesfully fetched data from cache", "data": ids})
		return
	}

	ids, err = RunBQ(trendlymodels.BrandPreferences{})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error Executing Query"})
		return
	}
	cachedData.IDs = ids
	cachedData.Time = time.Now().UnixMilli()
	_, err = firestoredb.Client.Collection("cached").Doc(cacheKey).Set(context.Background(), cachedData)
	if err != nil {
		log.Println("Error caching data:", err)
	}

	c.JSON(http.StatusOK, gin.H{"message": "Succesfully fetched data", "data": ids})
}
func RunBQ(preference trendlymodels.BrandPreferences) ([]string, error) {
	// AND location in ("Delhi")
	// AND category in ("Fashion / Beauty", "Food")
	// AND language in ("English", "Hindi")

	location := ""
	category := ""
	language := ""

	if preference.Locations != nil && len(preference.Locations) > 0 {
		location = fmt.Sprintf("AND location in (\"%s\")", strings.Join(preference.Locations, `", "`))
	}
	if preference.InfluencerCategories != nil && len(preference.InfluencerCategories) > 0 {
		category = fmt.Sprintf("AND category in (\"%s\")", strings.Join(preference.InfluencerCategories, `", "`))
	}
	if preference.Languages != nil && len(preference.Languages) > 0 {
		language = fmt.Sprintf("AND language in (\"%s\")", strings.Join(preference.Languages, `", "`))
	}

	sql := fmt.Sprintf(sqlFmt, location, category, language)
	// log.Println("Running Query:", sql)

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
