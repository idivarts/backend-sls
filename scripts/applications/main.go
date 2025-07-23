package main

import (
	"encoding/csv"
	"log"
	"os"
	"strconv"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/messenger"
)

const (
	CollaborationID = "cvRCjk3i1J1t7UflMNUz"
	ServiceCharge   = 1000
)

func main() {
	collab := trendlymodels.Collaboration{}
	err := collab.Get(CollaborationID)
	if err != nil {
		panic(err)
	}

	applications, err := trendlymodels.GetAllApplications(CollaborationID)
	if err != nil {
		panic(err)
	}

	file, err := os.Create("applications.csv")
	if err != nil {
		log.Fatalf("Failed to create CSV file: %v", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write CSV header
	writer.Write([]string{"Name", "Followers", "Reach", "Engagement", "Budget", "Profile Link", "Message From Influencer"}) // "Social Data", "Application Attachment", "Profile Attachment", "Question / Answers",

	// Write each application as a CSV row
	for _, app := range applications {
		user := trendlymodels.User{}
		err := user.Get(app.UserID)
		if err != nil {
			log.Println(err)
			continue
		}

		social := trendlymodels.Socials{}
		err = social.Get(app.UserID, *user.PrimarySocial)
		if err != nil {
			log.Println(err)
			continue
		}
		// socialUrl := ""
		applicationUrl := "https://brands.trendly.now/collaboration-application?collaborationId=" + CollaborationID + "&applicationId=" + app.UserID

		// if social.IsInstagram {
		// 	socialUrl = "https://www.instagram.com/" + social.InstaProfile.Username
		// } else {
		// 	socialUrl = "https://www.facebook.com/" + social.FBProfile.ID
		// }
		if social.InstaProfile == nil {
			social.InstaProfile = &messenger.InstagramProfile{}
		}

		writer.Write([]string{
			user.Name,
			social.InstaProfile.ApproxMetrics.Followers,
			social.InstaProfile.ApproxMetrics.Views,
			social.InstaProfile.ApproxMetrics.Interactions,
			strconv.Itoa(app.Quotation + ServiceCharge),
			applicationUrl,
			app.Message,
		})
	}

	log.Println("Applications saved to applications.csv")
}
