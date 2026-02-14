package sui

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"

	"github.com/idivarts/backend-sls/internal/models/trendlyrdb"
	"github.com/idivarts/backend-sls/internal/openai/deduce"
	"github.com/idivarts/backend-sls/pkg/apify"
)

// EvaluateInstagrams processes multiple Instagram profiles in one go.
// Profiles that use the database are handled individually, while the rest are
// scraped in a single batched Apify call. Errors for individual profiles are
// collected; processing continues even if some profiles fail.
func EvaluateInstagrams(reqs []ScrapedSocial) error {
	log.Println("Evaluating instagrams (batch)", len(reqs))

	// Separate database-backed vs scrape-backed requests
	var dbReqs []ScrapedSocial
	var scrapeReqs []ScrapedSocial
	for _, r := range reqs {
		if r.UseDatabase {
			dbReqs = append(dbReqs, r)
		} else {
			scrapeReqs = append(scrapeReqs, r)
		}
	}

	var errs []error

	// --- Handle database-backed requests individually ---
	for _, req := range dbReqs {
		if err := EvaluateInstagram(req); err != nil {
			errs = append(errs, fmt.Errorf("db evaluate %s: %w", req.Username, err))
		}
	}

	// --- Batch-scrape the rest via Apify ---
	if len(scrapeReqs) > 0 {
		usernames := make([]string, len(scrapeReqs))
		highValue := make(map[string]bool)

		for i, r := range scrapeReqs {
			usernames[i] = r.Username
			if r.HighValueInfluencer {
				highValue[r.Username] = true
			}
		}

		scraped, err := apify.GetInstagrams(usernames, highValue)
		if err != nil {
			return fmt.Errorf("batch scrape failed: %w", err)
		}

		for _, req := range scrapeReqs {
			instagram, ok := scraped[req.Username]
			if !ok {
				errs = append(errs, fmt.Errorf("no data returned for %s", req.Username))
				continue
			}

			if instagram.Username != req.Username {
				errs = append(errs, fmt.Errorf("instagram username mismatch for %s", req.Username))
				continue
			}

			social, posts := TranslateInstagram(*instagram, req)
			social, posts = MoveImagesToS3(social, posts)

			// Enrich with AI
			enrichPayload := map[string]interface{}{
				"profile": instagram,
			}
			if len(req.Manual.Niches) > 0 || req.Manual.QualityScore > 0 {
				bias := map[string]interface{}{}
				if len(req.Manual.Niches) > 0 {
					bias["suggestedNiches"] = req.Manual.Niches
				}
				if req.Manual.QualityScore > 0 {
					bias["suggestedQualityScore"] = req.Manual.QualityScore
				}
				enrichPayload["bias"] = bias
			}

			enrichJSON, err := json.Marshal(enrichPayload)
			if err != nil {
				errs = append(errs, fmt.Errorf("marshal enrichment for %s: %w", req.Username, err))
				continue
			}

			enriched, err := deduce.EnrichInfluencer(string(enrichJSON))
			if err != nil {
				log.Printf("Enrichment failed for %s, continuing without AI fields: %v", req.Username, err)
			} else {
				social.Gender = enriched.Gender
				social.Location = enriched.Location
				social.Niches = append(enriched.Niches, enriched.SubNiches...)
				if req.Manual.QualityScore > 0 {
					social.QualityScore = req.Manual.QualityScore
				} else {
					social.QualityScore = enriched.Quality
				}
			}

			// Save to DB
			if err := social.Insert(); err != nil {
				errs = append(errs, fmt.Errorf("insert social %s: %w", req.Username, err))
				continue
			}
			if err := (trendlyrdb.InstagramPost{}).InsertMultiple(posts); err != nil {
				errs = append(errs, fmt.Errorf("insert posts %s: %w", req.Username, err))
				continue
			}

			log.Printf("Instagram data saved successfully for %s (ID: %d)", req.Username, social.ID)
		}
	}

	return errors.Join(errs...)
}

func EvaluateInstagram(req ScrapedSocial) error {
	log.Println("Evaluating instagram", req)

	social := &trendlyrdb.Socials{}
	posts := []trendlyrdb.InstagramPost{}
	var instagramRaw interface{}

	if req.UseDatabase {
		err := social.GetInstagram(req.Username)
		if err != nil {
			return err
		}
		posts, err = trendlyrdb.InstagramPost{}.GetVideosBySocialID(social.ID, 30)
		if err != nil {
			return err
		}
		ComputeAnalytics(social, posts)

		instagramRaw = struct {
			*trendlyrdb.Socials
			Posts []trendlyrdb.InstagramPost `json:"reels"`
		}{
			Socials: social,
			Posts:   posts,
		}
	} else {
		// -> Calling api to scrape data
		instagram, err := apify.GetInstagram(req.Username, req.HighValueInfluencer)
		if err != nil {
			return err
		}
		log.Println("Instagram data", instagram)

		if instagram.Username != req.Username {
			return errors.New("instagram username mismatch")
		}

		social, posts = TranslateInstagram(*instagram, req)

		// -> Download all the images
		social, posts = MoveImagesToS3(social, posts)
		instagramRaw = instagram
	}
	// -> Send Raw for estimations (with Bias input which were sent manually)
	enrichPayload := map[string]interface{}{
		"profile": instagramRaw,
	}
	if len(req.Manual.Niches) > 0 || req.Manual.QualityScore > 0 {
		bias := map[string]interface{}{}
		if len(req.Manual.Niches) > 0 {
			bias["suggestedNiches"] = req.Manual.Niches
		}
		if req.Manual.QualityScore > 0 {
			bias["suggestedQualityScore"] = req.Manual.QualityScore
		}
		enrichPayload["bias"] = bias
	}

	enrichJSON, err := json.Marshal(enrichPayload)
	if err != nil {
		return fmt.Errorf("failed to marshal enrichment payload: %w", err)
	}

	enriched, err := deduce.EnrichInfluencer(string(enrichJSON))
	if err != nil {
		log.Println("Enrichment failed, continuing without AI fields:", err)
	} else {
		social.Gender = enriched.Gender
		social.Location = enriched.Location
		social.Niches = append(enriched.Niches, enriched.SubNiches...)
		if req.Manual.QualityScore > 0 {
			social.QualityScore = req.Manual.QualityScore
		} else {
			social.QualityScore = enriched.Quality
		}
	}

	// -> Save updated data in mysql
	err = social.Insert()
	if err != nil {
		return err
	}
	err = trendlyrdb.InstagramPost{}.InsertMultiple(posts)
	if err != nil {
		return err
	}

	log.Println("Instagram data saved successfully", social.ID)

	return nil
}
