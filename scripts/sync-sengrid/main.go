package main

import (
	"context"
	"log"
	"regexp"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
	"github.com/idivarts/backend-sls/pkg/myemail"
	"google.golang.org/api/iterator"
)

func main() {
	// Run as an AWS Lambda handler
	lambda.Start(handler)
}

func handler(ctx context.Context) (string, error) {
	start := time.Now().UnixMicro()
	log.Println("Lambda invocation start", start)

	log.Println("Syncing Users")
	syncUsers()
	log.Println("Syncing Managers")
	syncManagers()
	log.Println("Sync Completed")

	log.Println("Lambda invocation end", time.Now().UnixMicro())
	return "ok", nil
}

func syncManagers() {
	iter := firestoredb.Client.Collection("managers").Documents(context.Background())
	defer iter.Stop()

	contacts := []myemail.ContactDetails{}
	for {
		doc, err := iter.Next()
		if err != nil {
			if err == iterator.Done {
				break
			}
			panic(err.Error())
		}
		if time.Since(doc.UpdateTime) > 48*time.Hour {
			continue
		}

		log.Println("Creating Doc")
		manager := &trendlymodels.Manager{}
		err = doc.DataTo(manager)
		if err != nil {
			panic(err.Error())
		}
		brand, _ := trendlymodels.GetMyFirstBrand(doc.Ref.ID)
		brandName := ""
		if brand != nil {
			brandName = brand.Name
		}

		if manager.Email != "" {
			if manager.CreationTime == 0 {
				manager.CreationTime = time.Now().UnixMilli()
			}
			mContact := myemail.ContactDetails{
				Email:        manager.Email,
				Name:         manager.Name,
				IsManager:    true,
				CreationTime: &manager.CreationTime,
				CompanyName:  brandName,
			}
			if brand != nil {
				if brand.Profile != nil && brand.Profile.PhoneNumber != nil {
					mContact.Phone = *brand.Profile.PhoneNumber
				}
				if brand.Profile != nil && brand.Profile.Website != nil {
					mContact.SocialLink = *brand.Profile.Website
				}
			}
			contacts = append(contacts, mContact)
		}
	}
	log.Println("Got all docs", len(contacts))
	for i := 0; i < len(contacts); i += 100 {
		err := myemail.CreateOrUpdateContacts(contacts[i:min(i+100, len(contacts))])
		if err != nil {
			panic(err.Error())
		}
		log.Println("Upsert Batch Complete")
	}
}

func syncUsers() {
	iter := firestoredb.Client.Collection("users").Documents(context.Background())
	defer iter.Stop()

	incompleteProfiles := 0
	contacts := []myemail.ContactDetails{}
	for {
		doc, err := iter.Next()
		if err != nil {
			if err == iterator.Done {
				break
			}
			panic(err.Error())
		}
		if time.Since(doc.UpdateTime) > 48*time.Hour {
			continue
		}

		user := &trendlymodels.User{}
		err = doc.DataTo(user)
		if err != nil {
			panic(err.Error())
		}

		if user.Email != nil && *user.Email != "" {
			validEmail := regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
			if !validEmail.MatchString(*user.Email) {
				log.Println("Invalid Doc", len(contacts), *user.Email)
				continue
			}
			log.Println("Creating Doc", len(contacts), *user.Email)

			phone := ""
			pCent := 0
			if user.PhoneNumber != nil {
				phone = *user.PhoneNumber
			}
			socialUrl := ""
			if user.PrimarySocial != nil {
				social := trendlymodels.Socials{}
				err = social.Get(doc.Ref.ID, *user.PrimarySocial)
				if err != nil {
					log.Println(err)
					continue
				}
				if social.IsInstagram {
					socialUrl = "https://www.instagram.com/" + social.InstaProfile.Username
				} else {
					socialUrl = "https://www.facebook.com/" + social.FBProfile.ID
				}
			}

			if user.Profile != nil {
				pCent = *user.Profile.CompletionPercentage
			}
			contacts = append(contacts, myemail.ContactDetails{
				Email:             *user.Email,
				Name:              user.Name,
				Phone:             phone,
				IsManager:         false,
				ProfileCompletion: pCent,
				CreationTime:      user.CreationTime,
				LastActivityTime:  user.LastUseTime,
				SocialLink:        socialUrl,
			})
			if pCent < 60 {
				incompleteProfiles++
			}
		}
	}
	log.Println("Got all docs", len(contacts), incompleteProfiles, ":", len(contacts)-incompleteProfiles)
	for i := 0; i < len(contacts); i += 100 {
		err := myemail.CreateOrUpdateContacts(contacts[i:min(i+100, len(contacts))])
		if err != nil {
			log.Println("Error", err.Error())
			panic(err.Error())
		}
		log.Println("Upsert Batch Complete")
	}
}
