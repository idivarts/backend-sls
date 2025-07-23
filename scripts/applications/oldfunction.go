package main

import (
	"encoding/csv"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
)

func oldMain() {
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
	writer.Write([]string{"User", "Social", "TimeStamp", "Message", "Quotation", "Timeline", "Social Data", "Application Attachment", "Profile Attachment", "Question / Answers"})

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
		socialUrl := ""
		socialData := ""
		applicationAttachments := ""
		profileAttachments := ""
		questionAnswers := ""

		if social.IsInstagram {
			socialUrl = "https://www.instagram.com/" + social.InstaProfile.Username

			if len(social.SocialScreenShots) > 0 {
				socialData += social.SocialScreenShots[0] + "\n" + social.SocialScreenShots[1] + "\n"
			}

			socialData += `Followers : ` + social.InstaProfile.ApproxMetrics.Followers + "\nViews : " + social.InstaProfile.ApproxMetrics.Views + "\nInteractions : " + social.InstaProfile.ApproxMetrics.Interactions
		} else {
			socialUrl = "https://www.facebook.com/" + social.FBProfile.ID
		}

		for _, v := range app.Attachments {
			if v.ImageURL != nil {
				applicationAttachments += (*v.ImageURL) + "\n"
			}
			if v.AppleURL != nil {
				applicationAttachments += (*v.AppleURL) + "\n"
			}
		}
		for _, v := range user.Profile.Attachments {
			if v.ImageURL != nil {
				profileAttachments += (*v.ImageURL) + "\n"
			}
			if v.AppleURL != nil {
				profileAttachments += (*v.AppleURL) + "\n"
			}
		}

		// for _, v := range app.AnswersFromInfluencer {
		// 	questionAnswers += "Question : " + collab.QuestionsToInfluencers[v.Question] + "\n"
		// 	questionAnswers += "Answer : " + v.Answer + "\n\n----\n"
		// }

		writer.Write([]string{
			user.Name,
			socialUrl,
			time.UnixMilli(app.TimeStamp).Format("2006-01-02 15:04:05"),
			app.Message,
			strconv.Itoa(app.Quotation),
			strconv.FormatInt(app.Timeline, 10),
			socialData,
			applicationAttachments,
			profileAttachments,
			questionAnswers,
		})
	}

	log.Println("Applications saved to applications.csv")
}
