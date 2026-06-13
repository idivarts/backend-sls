package main

import (
	"log"
	"os"

	"github.com/idivarts/backend-sls/internal/trendlyapis/billing"
)

// One-off migration: backfill the new org token wallet + entitlements onto every
// existing organization (and normalize legacy India plan keys → new USD tiers).
// Dry-run by default; set MIGRATE_APPLY=1 to write.
//
//	go run ./scripts/migrate-credit-wallets            # dry-run
//	MIGRATE_APPLY=1 go run ./scripts/migrate-credit-wallets   # apply
func main() {
	apply := os.Getenv("MIGRATE_APPLY") == "1"
	log.Printf("migrate-credit-wallets: apply=%v", apply)
	if err := billing.MigrateCreditWallets(apply); err != nil {
		log.Fatalf("migration failed: %v", err)
	}
	log.Println("migrate-credit-wallets: complete")
}
