package main

import (
	"encoding/json"
	"log"

	"github.com/idivarts/backend-sls/internal/models/trendlyrdb"
	sqshandler "github.com/idivarts/backend-sls/pkg/sqs_handler"
	"github.com/idivarts/backend-sls/scripts/socials-add-entries/sui"
)

func main() {
	const pageSize = 500
	offset := 0

	for {
		socials, err := trendlyrdb.Socials{}.GetPaginated(offset, pageSize)
		if err != nil {
			log.Fatalf("Failed to get socials: %v", err)
		}
		if len(socials) == 0 {
			break
		}

		log.Printf("Processing %d socials (offset %d)\n", len(socials), offset)

		for _, social := range socials {
			log.Println("Social", social.Username)
			highValueInfluencer := false
			useDatabase := true
			if social.QualityScore > 8 {
				highValueInfluencer = true
				useDatabase = false
			}
			scrape := sui.ScrapedSocial{
				Username:            social.Username,
				SocialType:          social.SocialType,
				HighValueInfluencer: highValueInfluencer,
				UseDatabase:         useDatabase,
				Manual: struct {
					Niches       []string `json:"niches"`
					QualityScore int      `json:"qualityScore" binding:"gte=0,lte=10"`
				}{
					Niches:       social.Niches,
					QualityScore: social.QualityScore,
				},
			}
			jsonData, err := json.Marshal(scrape)
			if err != nil {
				log.Fatalf("Failed to marshal scrape: %v", err)
			}
			sqshandler.SendToMessageQueue(string(jsonData), 0)
		}

		offset += len(socials)
	}

	log.Printf("Done. Processed %d total socials.\n", offset)
}
