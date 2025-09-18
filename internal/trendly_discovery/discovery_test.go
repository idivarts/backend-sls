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
	influencerId := "95d0838c-a5d2-5849-ba4a-b5ea0d7f67a9"

	social := &trendlybq.Socials{}
	err := social.Get(influencerId)
	if err != nil {
		t.Error(err)
	}

	calculatedValue := trendlydiscovery.TestCalculations(social)
	calculatedValue.CPM = float32(calculatedValue.EstimatedBudget.Max+calculatedValue.EstimatedBudget.Min) * 1000 / float32(calculatedValue.EstimatedReach.Max+calculatedValue.EstimatedReach.Min)

	pretty, _ := json.MarshalIndent(calculatedValue, "", "  ")
	log.Println(string(pretty))
}
