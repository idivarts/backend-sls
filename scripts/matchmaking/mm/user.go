package mm

import (
	"context"
	"log"
	"time"

	"github.com/idivarts/backend-sls/internal/models/trendlybq"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
	"github.com/idivarts/backend-sls/pkg/myutil"
	"google.golang.org/api/iterator"
)

// SyncUsers This will be used to sync users
func SyncUsers(iterative bool) error {
	iter := firestoredb.Client.Collection("users").Documents(context.Background())
	defer iter.Stop()

	data := []trendlybq.BQInfluencers{}
	for {
		doc, err := iter.Next()
		if err != nil {
			if err == iterator.Done {
				break
			}
			panic(err.Error())
		}
		if iterative && time.Since(doc.UpdateTime) > 16*time.Hour {
			continue
		}

		log.Println("Creating Doc", doc.Ref.ID)
		user := &trendlymodels.User{}
		err = doc.DataTo(user)
		if err != nil {
			panic(err.Error())
		}

		if user.PrimarySocial != nil {
			socialData, err := firestoredb.Client.Collection("users").Doc(doc.Ref.ID).Collection("socials").Doc(*user.PrimarySocial).Get(context.Background())
			if err != nil {
				log.Println("Error fetching social", doc.Ref.ID)
				continue
			}
			social := &trendlymodels.Socials{}
			err = socialData.DataTo(social)
			if err != nil {
				log.Println("Error In Parsing", doc.Ref.ID)
				continue
			}
			sName := ""
			sType := "facebook"
			followCount := 0
			reach := 0
			interaction := 0
			if social.IsInstagram {
				sType = "instagram"
				sName = social.InstaProfile.Username
				followCount = RangeToMidpoint(social.InstaProfile.ApproxMetrics.Followers)
				reach = RangeToMidpoint(social.InstaProfile.ApproxMetrics.Views)
				interaction = RangeToMidpoint(social.InstaProfile.ApproxMetrics.Interactions)
			} else {
				sName = social.FBProfile.Name
				followCount = social.FBProfile.FollowersCount
			}
			data = append(data, trendlybq.BQInfluencers{
				ID:               doc.Ref.ID,
				Location:         myutil.DerefString(user.Location),
				FollowerCount:    followCount,
				ReachCount:       reach,
				InteractionCount: interaction,
				PrimarySocial:    sName,
				SocialType:       sType,
			})
		} else {
			continue
		}
		if user.Profile != nil {
			categories := user.Profile.Category
			finalCategory := []string{}

			for _, item := range categories {
				if mapped, ok := CONTENT_NICHE_REVERSE_MAP[item]; ok {
					finalCategory = append(finalCategory, item)
					finalCategory = append(finalCategory, mapped...)
				} else {
					finalCategory = append(finalCategory, item)
				}
			}
			uniqueCategories := make(map[string]struct{})
			for _, category := range finalCategory {
				uniqueCategories[category] = struct{}{}
			}
			finalCategory = []string{}
			for category := range uniqueCategories {
				finalCategory = append(finalCategory, category)
			}
			data[len(data)-1].Categories = finalCategory
			data[len(data)-1].CompletionPercentage = *user.Profile.CompletionPercentage
		}
		if user.Preferences != nil {
			collabTypeStr := myutil.DerefString(user.Preferences.PreferredCollaborationType)
			collabType := []string{}

			if myutil.ContainsIgnoreCase(collabTypeStr, "Barter") {
				collabType = append(collabType, "Barter")
			}
			if myutil.ContainsIgnoreCase(collabTypeStr, "Paid") {
				collabType = append(collabType, "PAID")
			}

			data[len(data)-1].Languages = user.Preferences.PreferredLanguages
			data[len(data)-1].PreferredBrandIndustries = user.Preferences.PreferredBrandIndustries
			data[len(data)-1].PostType = user.Preferences.ContentWillingToPost
			data[len(data)-1].CollaborationType = collabType
		}
	}

	log.Println("Deleting", len(data))
	query, err := trendlybq.BQInfluencers{}.DeleteMultipleSQL(INFLUENCER_TABLE, data)
	if err != nil {
		log.Fatalf("Failed to create query: %v", err)
		return err
	}
	deleteJob, err := query.Run(context.Background())
	status, err := deleteJob.Wait(context.Background())
	if err != nil {
		log.Fatalf("Error while waiting for delete job to finish: %v", err)
		return err
	}
	if status.Err() != nil {
		log.Fatalf("Delete job failed: %v", status.Err())
		return status.Err()
	}

	log.Println("Deletion Completed", len(data))

	batchSize := 100
	for i := 0; i < len(data); i += batchSize {
		end := i + batchSize
		if end > len(data) {
			end = len(data)
		}
		batch := data[i:end]
		query, err := trendlybq.BQInfluencers{}.GetInsertMultipleSQL(INFLUENCER_TABLE, batch)
		if err != nil {
			log.Fatalf("Failed to create query: %v", err)
			return err
		}
		j, err := query.Run(context.Background())
		if err != nil {
			log.Fatalf("Failed to execute query: %v", err)
			return err
		}
		log.Println("Job Created, Batch", i, " : ", j.ID())
	}

	return nil
}
