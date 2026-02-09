package rdb_test

import (
	"log"
	"testing"

	"github.com/idivarts/backend-sls/pkg/rdb"
)

func TestInitPostgres(t *testing.T) {
	// Test GORM connection by querying the socials table
	var count int64
	err := rdb.GormDB.Table("socials").Count(&count).Error
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
