package trendlydiscovery_test

import (
	"log"
	"testing"

	trendlydiscovery "github.com/idivarts/backend-sls/internal/trendly_discovery"
)

func TestDiscovery(t *testing.T) {
	sql := trendlydiscovery.FormSQL(trendlydiscovery.InfluencerFilters{})
	log.Println(sql)
}
