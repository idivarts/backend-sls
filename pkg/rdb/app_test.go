package rdb_test

import (
	"log"
	"testing"

	"github.com/idivarts/backend-sls/internal/models/trendlyrdb"
	"github.com/idivarts/backend-sls/pkg/rdb"
	"github.com/lib/pq"
)

func TestInitPostgres(t *testing.T) {
	// Test GORM connection by querying the socials table
	count, err := trendlyrdb.Socials{}.Count()
	if err != nil {
		t.Errorf("GORM query failed: %v", err)
		return
	}

	log.Printf("Successfully connected to Postgres with GORM. Socials count: %d", count)

	// Verify underlying sql.DB is also accessible
	if rdb.DB == nil {
		t.Error("sql.DB is nil")
	}
}

func TestInsert(t *testing.T) {
	// Test GORM connection by querying the socials table
	tSocial := trendlyrdb.Socials{
		Username:        "test-username-2",
		Name:            "test-name",
		Bio:             "test-bio",
		ProfilePic:      "test-profile-pic",
		ProfilePicHD:    "test-profile-pic-hd",
		Category:        "test-category",
		SocialType:      "instagram",
		ProfileVerified: false,
		FollowerCount:   100,
		FollowingCount:  200,
		ContentCount:    300,
		ViewsCount:      400,
		EngagementCount: 500,
		EngagementRate:  600,
		AverageViews:    700,
		AverageLikes:    800,
		AverageComments: 900,
		Links: []trendlyrdb.Links{
			{
				Title: "website",
				URL:   "Somethign great website",
			},
			{
				Title: "website2",
				URL:   "Somethign great website2",
			},
		},
		Gender:         "male",
		Location:       "test-location",
		Niches:         pq.StringArray{},
		QualityScore:   0,
		AddedBy:        "rahul-test",
		CreationTime:   0,
		LastUpdateTime: 0,
		ExternalId:     "",
	}
	err := tSocial.Insert()
	if err != nil {
		t.Errorf("GORM query failed: %v", err)
		return
	}

	log.Printf("Successfully inserted to Postgres with GORM")

	// Verify underlying sql.DB is also accessible
	if rdb.DB == nil {
		t.Error("sql.DB is nil")
	}
}
