package main

import (
	"log"
	"math"

	"github.com/idivarts/backend-sls/internal/models/trendlyrdb"
	"github.com/idivarts/backend-sls/pkg/rdb"
)

// migrateQualityScores iterates over all socials entries and normalises
// the quality_score column to the new 1-10 range.
//
// Rules:
//  1. Values 1-5  → multiply by 2  (old 5-point scale → new 10-point scale)
//  2. Values 11-100 → divide by 10, round to nearest integer, clamp to [1,10]
//     (old 100-point scale → new 10-point scale)
//  3. Values 6-10  → already in the correct range, skip.
//  4. Value 0      → skip (unset / not scored yet).
func main() {
	offset := 0
	limit := 500
	totalUpdated := 0

	for {
		log.Printf("Fetching batch: offset=%d, limit=%d", offset, limit)

		var socials []trendlyrdb.Socials
		err := rdb.GormDB.
			Model(&trendlyrdb.Socials{}).
			Where("quality_score IS NOT NULL AND quality_score != 0").
			Order("id ASC").
			Limit(limit).
			Offset(offset).
			Find(&socials).Error
		if err != nil {
			log.Fatalf("Failed to fetch socials: %v", err)
		}

		if len(socials) == 0 {
			break
		}

		log.Printf("Fetched %d records", len(socials))

		for _, s := range socials {
			oldScore := s.QualityScore
			newScore := convertScore(oldScore)

			if newScore == oldScore {
				continue
			}

			log.Printf("  [%s] %s: %d → %d", s.ID, s.Username, oldScore, newScore)

			err := rdb.GormDB.
				Model(&trendlyrdb.Socials{}).
				Where("id = ?", s.ID).
				Update("quality_score", newScore).Error
			if err != nil {
				log.Printf("  ERROR updating %s: %v", s.ID, err)
				continue
			}
			totalUpdated++
		}

		if len(socials) < limit {
			break
		}
		offset += limit
	}

	log.Printf("Migration complete. Total records updated: %d", totalUpdated)
}

// convertScore maps an old quality_score to the new 1-10 range.
func convertScore(score int) int {
	switch {
	case score <= 0:
		// Unset — leave as-is
		return 0
	case score >= 1 && score <= 5:
		// Old 1-5 scale → multiply by 2
		return score * 2
	case score >= 6 && score <= 10:
		// Already in 1-10 range — no change
		return score
	case score >= 11 && score <= 100:
		// Old 100-point scale → divide by 10, round, clamp
		converted := int(math.Round(float64(score) / 10.0))
		return clamp(converted, 1, 10)
	default:
		// Unexpected value (>100 or negative) — clamp to 1-10
		converted := int(math.Round(float64(score) / 10.0))
		return clamp(converted, 1, 10)
	}
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
