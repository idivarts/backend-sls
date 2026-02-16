package main

import (
	"log"
	"time"

	"github.com/idivarts/backend-sls/internal/models/trendlyrdb"
	sui "github.com/idivarts/backend-sls/internal/utilities/scrapping-utility"
)

func main() {
	const pageSize = 100
	offset := 8720

	for {
		socials, err := trendlyrdb.Socials{}.GetPaginated(offset, pageSize)
		scrapeList := []sui.ScrapedSocial{}
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
			// if social.QualityScore > 9 {
			// 	highValueInfluencer = true
			// 	useDatabase = false
			// }
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
			scrapeList = append(scrapeList, scrape)
		}
		log.Println("Sending to evaluate Socials: %d, (%d, %d)", len(scrapeList), offset, (offset + pageSize))
		err = sui.EvaluateInstagrams(scrapeList)
		if err != nil {
			log.Println("Failed to evaluate socials: %v", err)
		} else {
			log.Println("Evaluated Socials: %d, (%d, %d)", len(scrapeList), offset, (offset + pageSize))
		}
		offset += len(socials)

		log.Println("Sleeping for 30 seconds. Currently at", offset, (offset + pageSize))
		time.Sleep(50 * time.Second)
		log.Println("Done Sleeping")
	}

	log.Printf("Done. Processed %d total socials.\n", offset)
}
