package main

import (
	"log"

	"github.com/idivarts/backend-sls/internal/models/trendlyrdb"
	"github.com/idivarts/backend-sls/pkg/rdb"
)

func main() {
	// Test GORM connection by querying the socials table
	err := rdb.AutoMigrate(&trendlyrdb.InstagramPost{})
	// &trendlyrdb.InstagramPost{},
	if err != nil {
		panic(err)
	}

	log.Printf("Successfully connected to Postgres with GORM")

	// Verify underlying sql.DB is also accessible
	if rdb.DB == nil {
		panic(err)
	}
}
