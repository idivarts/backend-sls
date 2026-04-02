package trendlydiscovery_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/models/trendlyrdb"
	trendlydiscovery "github.com/idivarts/backend-sls/internal/trendly_discovery"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
	"github.com/idivarts/backend-sls/pkg/rdb"
)

func TestDiscovery(t *testing.T) {
	// sql := trendlydiscovery.FormSQL(trendlydiscovery.InfluencerFilters{
	// 	FollowerMin: aws.Int64(7000),
	// 	Name:        aws.String("Saks"),
	// })
	// log.Println(sql)
}

func TestGetInfluencers_InvalidJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/discovery/influencers", bytes.NewBufferString(`{invalid`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	trendlydiscovery.GetInfluencers(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	var body struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("response JSON: %v", err)
	}
	if body.Message != "Invalid Input" {
		t.Fatalf("message = %q, want Invalid Input", body.Message)
	}
}

func TestGetInfluencers_Success(t *testing.T) {
	if rdb.GormDB == nil {
		t.Skip("postgres not configured (rdb.GormDB is nil)")
	}
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/discovery/influencers", bytes.NewBufferString(`{"name":"thesiickboy"}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	trendlydiscovery.GetInfluencers(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	var resp struct {
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response JSON: %v", err)
	}
	if resp.Message != "Success" {
		t.Fatalf("message = %q, want Success", resp.Message)
	}
	var data []map[string]interface{}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("data JSON: %v", err)
	}
	// isDiscover is only present when true (omitempty on InfluencerItem).
	for i, row := range data {
		if _, ok := row["username"]; !ok {
			t.Errorf("data[%d]: expected embedded socials fields (username)", i)
		}
		t.Log("Influencer Data", i, row["username"], row["isDiscover"])
	}
}

func TestCalcualations(t *testing.T) {
	influencerId := "95d0838c-a5d2-5849-ba4a-b5ea0d7f67a9"

	social := &trendlyrdb.Socials{}
	err := social.Get(influencerId)
	if err != nil {
		t.Error(err)
	}

	calculatedValue := trendlydiscovery.TestCalculations(social)
	calculatedValue.CPM = float32(calculatedValue.EstimatedBudget.Max+calculatedValue.EstimatedBudget.Min) * 1000 / float32(calculatedValue.EstimatedReach.Max+calculatedValue.EstimatedReach.Min)

	pretty, _ := json.MarshalIndent(calculatedValue, "", "  ")
	log.Println(string(pretty))
}

func TestGetAllCount(t *testing.T) {
	allDocs, err := firestoredb.Client.Collection("scrapped-socials").Where("reel_scrapped_count", ">", 0).Where("added_by", "==", "jiko-windows-123").Documents(context.Background()).GetAll()
	if err != nil {
		t.Error(err)
	}
	log.Println("All Documents", len(allDocs))
}
