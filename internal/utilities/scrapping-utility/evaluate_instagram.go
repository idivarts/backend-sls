package sui

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"

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

	// --- Handle database-backed requests in batch ---
	if len(dbReqs) > 0 {
		if err := EvaluateDatabaseInstagrams(dbReqs); err != nil {
			errs = append(errs, err)
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

// EvaluateDatabaseInstagrams processes multiple database-backed Instagram profiles
// using batch fetch and batch insert to minimise round-trips and avoid partial errors.
func EvaluateDatabaseInstagrams(reqs []ScrapedSocial) error {
	if len(reqs) == 0 {
		return nil
	}
	log.Println("Evaluating database instagrams (batch)", len(reqs))

	// Build deterministic IDs for every request and index requests by ID.
	socialIDs := make([]string, len(reqs))
	reqByID := make(map[string]ScrapedSocial, len(reqs))
	for i, r := range reqs {
		s := &trendlyrdb.Socials{Username: r.Username, SocialType: "instagram"}
		id := s.GetID()
		socialIDs[i] = id
		reqByID[id] = r
	}

	// --- Batch-fetch socials ---
	socials, err := (trendlyrdb.Socials{}).GetMultiple(socialIDs)
	if err != nil {
		return fmt.Errorf("batch fetch socials: %w", err)
	}
	socialByID := make(map[string]*trendlyrdb.Socials, len(socials))
	for i := range socials {
		socialByID[socials[i].ID] = &socials[i]
	}

	// --- Batch-fetch video posts for all socials ---
	allPosts, err := (trendlyrdb.InstagramPost{}).GetVideosBySocialIDs(socialIDs)
	if err != nil {
		return fmt.Errorf("batch fetch posts: %w", err)
	}
	// Group posts by social ID (keep at most 30 per social, already ordered by timestamp DESC).
	postsBySocial := make(map[string][]trendlyrdb.InstagramPost, len(socialIDs))
	for _, p := range allPosts {
		if len(postsBySocial[p.SocialID]) < 30 {
			postsBySocial[p.SocialID] = append(postsBySocial[p.SocialID], p)
		}
	}

	// --- Process each social concurrently: analytics + AI enrichment ---
	var errs []error
	var socialsToInsert []trendlyrdb.Socials
	var postsToInsert []trendlyrdb.InstagramPost
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, id := range socialIDs {
		social, ok := socialByID[id]
		if !ok {
			errs = append(errs, fmt.Errorf("social not found for %s", reqByID[id].Username))
			continue
		}

		wg.Add(1)
		go func(social *trendlyrdb.Socials, req ScrapedSocial, posts []trendlyrdb.InstagramPost) {
			defer wg.Done()

			ComputeAnalytics(social, posts)

			instagramRaw := struct {
				*trendlyrdb.Socials
				Posts []trendlyrdb.InstagramPost `json:"reels"`
			}{
				Socials: social,
				Posts:   posts,
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
				mu.Lock()
				errs = append(errs, fmt.Errorf("marshal enrichment for %s: %w", req.Username, err))
				mu.Unlock()
				return
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

			mu.Lock()
			socialsToInsert = append(socialsToInsert, *social)
			postsToInsert = append(postsToInsert, posts...)
			mu.Unlock()
		}(social, reqByID[id], postsBySocial[id])
	}
	wg.Wait()

	// --- Batch-insert socials ---
	if len(socialsToInsert) > 0 {
		if err := (trendlyrdb.Socials{}).InsertMultiple(socialsToInsert); err != nil {
			errs = append(errs, fmt.Errorf("batch insert socials: %w", err))
		}
	}

	// --- Batch-insert posts ---
	if len(postsToInsert) > 0 {
		if err := (trendlyrdb.InstagramPost{}).InsertMultiple(postsToInsert); err != nil {
			errs = append(errs, fmt.Errorf("batch insert posts: %w", err))
		}
	}

	log.Printf("Database instagrams evaluated: %d socials, %d posts", len(socialsToInsert), len(postsToInsert))

	return errors.Join(errs...)
}
