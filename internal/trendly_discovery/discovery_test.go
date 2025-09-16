package trendlydiscovery_test

import (
	"encoding/json"
	"log"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/idivarts/backend-sls/internal/models/trendlybq"
	trendlydiscovery "github.com/idivarts/backend-sls/internal/trendly_discovery"
)

func TestDiscovery(t *testing.T) {
	sql := trendlydiscovery.FormSQL(trendlydiscovery.InfluencerFilters{
		FollowerMin: aws.Int64(7000),
		Name:        aws.String("Saks"),
	})
	log.Println(sql)
}

func TestCalcualations(t *testing.T) {
	influencerId := "1556f797-e72b-591a-beb0-4c25c7480f16"

	social := &trendlybq.Socials{}
	err := social.Get(influencerId)
	if err != nil {
		t.Error(err)
	}

	calculatedValue := trendlydiscovery.TestCalculations(social)

	pretty, _ := json.MarshalIndent(calculatedValue, "", "  ")
	log.Println(string(pretty))
}
