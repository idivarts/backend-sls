package main

import (
	"fmt"
	"log"

	"github.com/google/uuid"
	"github.com/idivarts/backend-sls/internal/models/trendlybq"
	"github.com/idivarts/backend-sls/internal/models/trendlyrdb"
)

func main() {
	// transferAllData()
	checkAndInsertMissingData()
}

func checkAndInsertMissingData() {
	offset := 0
	limit := 500
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

		log.Printf("Fetched %d records from Socials %s", len(oldSocials), oldSocials[0].Username)

		ids := []string{}
		for _, old := range oldSocials {
			ids = append(ids, old.ID)
		}

		// Push to PostGres
		newSocials, err := trendlyrdb.Socials{}.GetMultiple(ids)
		if err != nil {
			log.Fatalf("Failed to insert to PostGres: %v", err)
		}
		log.Printf("Fetched %d records from PostGres %s", len(newSocials), newSocials[0].Username)

		var missingSocials []trendlyrdb.Socials
		var missingInstaPosts []trendlyrdb.InstagramPost

		for _, oldSocial := range oldSocials {
			found := false
			for _, v := range newSocials {
				if v.Username == oldSocial.Username {
					found = true
					break
				}
			}
			if !found {
				log.Println("User Not Found", oldSocial.Username, oldSocial.ID)
				social, posts := translate(oldSocial)
				missingSocials = append(missingSocials, social)
				missingInstaPosts = append(missingInstaPosts, posts...)
			}
		}
		log.Printf("Missing Socials: %d  | Missing Insta Posts: %d", len(missingSocials), len(missingInstaPosts))

		err = trendlyrdb.Socials{}.InsertMultiple(missingSocials)
		if err != nil {
			log.Fatalf("Failed to insert to PostGres: %v", err)
		}
		log.Printf("Inserted %d socials to PostGres", len(missingSocials))

		err = trendlyrdb.InstagramPost{}.InsertMultiple(missingInstaPosts)
		if err != nil {
			log.Fatalf("Failed to insert to PostGres: %v", err)
		}
		log.Printf("Inserted %d posts to PostGres", len(missingInstaPosts))

		if len(oldSocials) < limit {
			break
		}
		offset += limit
	}

	log.Printf("Migration complete. Total records: %d", totalMigrated)
}

func transferAllData() {
	offset := 0
	limit := 500
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

		var newSocials []trendlyrdb.Socials
		var instaPosts []trendlyrdb.InstagramPost
		for _, old := range oldSocials {
			social, posts := translate(old)
			newSocials = append(newSocials, social)
			instaPosts = append(instaPosts, posts...)
		}

		log.Printf("Translated %d records to SocialsN8N", len(newSocials))

		// Push to PostGres
		err = trendlyrdb.Socials{}.InsertMultiple(newSocials)
		if err != nil {
			log.Fatalf("Failed to insert to PostGres: %v", err)
		}

		log.Printf("Inserted %d records to PostGres", len(newSocials))

		err = trendlyrdb.InstagramPost{}.InsertMultiple(instaPosts)
		if err != nil {
			log.Fatalf("Failed to insert to PostGres: %v", err)
		}

		log.Printf("Inserted %d posts to PostGres", len(instaPosts))

		totalMigrated += len(oldSocials)
		log.Printf("Migrated %d records so far", totalMigrated)

		if len(oldSocials) < limit {
			break
		}
		offset += limit
	}

	log.Printf("Migration complete. Total records: %d", totalMigrated)
}

func translate(old trendlybq.Socials) (trendlyrdb.Socials, []trendlyrdb.InstagramPost) {
	new := trendlyrdb.Socials{
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
		ProfilePicHD:    "",
		Links:           nil,
		ExternalId:      "",
	}

	instaPosts := []trendlyrdb.InstagramPost{}
	for _, r := range old.Reels {
		instaPosts = append(instaPosts, trendlyrdb.InstagramPost{
			ID:                 fmt.Sprintf("%s-%s", r.ID, uuid.New().String()),
			SocialID:           new.ID,
			DisplayURL:         r.ThumbnailURL,
			URL:                r.URL,
			Caption:            r.Caption,
			IsPinned:           r.Pinned,
			Type:               "video",
			PostLocation:       "reels",
			ShortCode:          r.ID,
			VideoURL:           r.URL,
			LikesCount:         r.LikesCount.Int64,
			CommentsCount:      r.CommentsCount.Int64,
			VideoViewCount:     r.ViewsCount.Int64,
			VideoPlayCount:     r.ViewsCount.Int64,
			VideoDuration:      0,
			Timestamp:          "",
			LocationName:       "",
			LocationID:         "",
			Alt:                "",
			Images:             nil,
			IsCommentsDisabled: false,
			AudioURL:           "",
			MusicInfo:          nil,
			Hashtags:           nil,
			Mentions:           nil,
			TaggedUsers:        nil,
			FirstComment:       "",
			LatestComments:     nil,
			ChildPosts:         nil,
		})
	}
	if len(new.Category) > 90 {
		new.Bio = new.Category
		new.Category = ""
	}

	// for _, l := range old.Links {
	// 	new.Links = append(new.Links, trendlybq.SocialLink{
	// 		URL:   l.URL,
	// 		Title: l.Text,
	// 	})
	// }

	return new, instaPosts
}
