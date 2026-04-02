package main

import (
	"log"

	"github.com/idivarts/backend-sls/internal/models/trendlyrdb"
	"github.com/idivarts/backend-sls/pkg/rdb"
)

// migrateQualityScores normalises the quality_score column to the new 1-10
// range using batch UPDATE statements executed directly in the database.
//
// Rules:
//  1. Values 1-5   -> multiply by 2  (old 5-point scale -> new 10-point scale)
//  2. Values 11-100 -> divide by 10, round to nearest integer, clamp to [1,10]
//     (old 100-point scale -> new 10-point scale)
//  3. Values 6-10  -> already in the correct range, skip.
//  4. Value 0/NULL -> skip (unset / not scored yet).
func main() {
	// Step 1: Old 1-5 scale -> multiply by 2
	res := rdb.GormDB.
		Model(&trendlyrdb.Socials{}).
		Where("quality_score >= 1 AND quality_score <= 5").
		Update("quality_score", rdb.GormDB.Raw("quality_score * 2"))
	if res.Error != nil {
		log.Fatalf("Failed to migrate 1-5 range: %v", res.Error)
	}
	log.Printf("Migrated %d rows (1-5 scale x2)", res.RowsAffected)

	// Step 2: Old 100-point scale -> ROUND(quality_score / 10), clamped to [1, 10]
	res = rdb.GormDB.
		Model(&trendlyrdb.Socials{}).
		Where("quality_score >= 11 AND quality_score <= 100").
		Update("quality_score", rdb.GormDB.Raw("LEAST(GREATEST(ROUND(quality_score / 10.0), 1), 10)"))
	if res.Error != nil {
		log.Fatalf("Failed to migrate 11-100 range: %v", res.Error)
	}
	log.Printf("Migrated %d rows (100-point scale /10)", res.RowsAffected)

	// Step 3: Anything above 100 (unexpected) -> clamp to 10
	res = rdb.GormDB.
		Model(&trendlyrdb.Socials{}).
		Where("quality_score > 100").
		Update("quality_score", 10)
	if res.Error != nil {
		log.Fatalf("Failed to clamp >100 values: %v", res.Error)
	}
	if res.RowsAffected > 0 {
		log.Printf("Clamped %d rows with quality_score > 100 to 10", res.RowsAffected)
	}

	log.Println("Migration complete.")
}
