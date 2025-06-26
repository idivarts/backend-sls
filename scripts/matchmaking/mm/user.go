package mm

import (
	"context"
	"log"
	"time"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
	"github.com/idivarts/backend-sls/pkg/myutil"
	"google.golang.org/api/iterator"
)

// SyncUsers This will be used to sync users
func SyncUsers(iterative bool) error {
	iter := firestoredb.Client.Collection("users").Documents(context.Background())
	defer iter.Stop()

	data := []trendlymodels.BQInfluencers{}
	for {
		doc, err := iter.Next()
		if err != nil {
			if err == iterator.Done {
				break
			}
			panic(err.Error())
		}
		if iterative && time.Since(doc.UpdateTime) > 28*time.Hour {
			continue
		}

		log.Println("Creating Doc")
		user := &trendlymodels.User{}
		err = doc.DataTo(user)
		if err != nil {
			panic(err.Error())
		}

		if user.Profile != nil {
			data = append(data, trendlymodels.BQInfluencers{
				ID:                   doc.Ref.ID,
				Location:             myutil.DerefString(user.Location),
				Categories:           user.Profile.Category,
				FollowerCount:        0,
				ReachCount:           0,
				InteractionCount:     0,
				CompletionPercentage: 0,
				PrimarySocial:        "",
				SocialType:           "",
			})
		}
		if user.Preferences != nil {
			data[len(data)-1].Languages = user.Preferences.PreferredLanguages
			data[len(data)-1].PreferredBrandIndustries = user.Preferences.PreferredBrandIndustries
			data[len(data)-1].PostType = user.Preferences.ContentWillingToPost
			data[len(data)-1].CollaborationType = []string{myutil.DerefString(user.Preferences.PreferredCollaborationType)}
		}
	}
	query, err := trendlymodels.BQInfluencers{}.GetInsertMultipleSQL(INFLUENCER_TABLE, data)

	j, err := query.Run(context.Background())
	if err != nil {
		log.Fatalf("Failed to execute query: %v", err)
		return err
	}

	log.Println("Job Created", j.ID())

	return nil
}
