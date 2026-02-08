package main

import (
	"log"

	"github.com/idivarts/backend-sls/internal/models/trendlybq"
)

func main() {
	offset := 0
	limit := 50
	totalMigrated := 0

	for {
		log.Printf("Fetching batch: offset=%d, limit=%d", offset, limit)
		oldSocials, err := trendlybq.Socials{}.GetPaginated(offset, limit)
		if err != nil {
			log.Fatalf("Failed to fetch Socials: %v", err)
		}

		if len(oldSocials) == 0 {
			break
		}

		log.Printf("Fetched %d records from Socials", len(oldSocials))

		var newSocials []trendlybq.SocialsN8N
		for _, old := range oldSocials {
			newSocials = append(newSocials, translate(old))
		}

		log.Printf("Translated %d records to SocialsN8N", len(newSocials))

		// Push to BigQuery
		err = trendlybq.SocialsN8N{}.InsertMultiple(newSocials)
		if err != nil {
			log.Fatalf("Failed to insert to BigQuery: %v", err)
		}

		log.Printf("Inserted %d records to BigQuery", len(newSocials))

		// Push to Firestore (Minimized)
		for _, v := range newSocials {
			err = v.UpdateMinified()
			if err != nil {
				log.Printf("Failed to update Firestore for %s: %v", v.ID, err)
			}
			log.Printf("Updated Firestore for %s", v.ID)
		}

		totalMigrated += len(oldSocials)
		log.Printf("Migrated %d records so far", totalMigrated)

		if len(oldSocials) < limit {
			break
		}
		offset += limit
	}

	log.Printf("Migration complete. Total records: %d", totalMigrated)
}

func translate(old trendlybq.Socials) trendlybq.SocialsN8N {
	new := trendlybq.SocialsN8N{
		ID:              old.ID,
		Name:            old.Name,
		Username:        old.Username,
		Bio:             old.Bio,
		ProfilePic:      old.ProfilePic,
		Category:        old.Category,
		SocialType:      old.SocialType,
		ProfileVerified: old.ProfileVerified,
		FollowerCount:   old.FollowerCount,
		FollowingCount:  old.FollowingCount,
		ContentCount:    old.ContentCount,
		ViewsCount:      old.ViewsCount,
		EngagementCount: old.EnagamentsCount,
		EngagementRate:  old.EngagementRate,
		AverageViews:    old.AverageViews,
		AverageLikes:    old.AverageLikes,
		AverageComments: old.AverageComments,
		Gender:          old.Gender,
		Niches:          old.Niches,
		Location:        old.Location,
		QualityScore:    old.QualityScore,
		AddedBy:         old.AddedBy,
		CreationTime:    old.CreationTime,
		LastUpdateTime:  old.LastUpdateTime,
		HasContacts:     old.HasContacts,
	}

	for _, r := range old.Reels {
		new.LatestReels = append(new.LatestReels, trendlybq.SinglePost{
			ID:             r.ID,
			DisplayURL:     r.ThumbnailURL,
			URL:            r.URL,
			Caption:        r.Caption,
			IsPinned:       r.Pinned,
			VideoViewCount: r.ViewsCount,
			LikesCount:     r.LikesCount,
			CommentsCount:  r.CommentsCount,
			Type:           "video",
		})
	}

	for _, l := range old.Links {
		new.Links = append(new.Links, trendlybq.SocialLink{
			URL:   l.URL,
			Title: l.Text,
		})
	}

	return new
}
