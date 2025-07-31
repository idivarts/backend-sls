package trendlyapis

import (
	"encoding/json"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/hubspot"
	"github.com/idivarts/backend-sls/pkg/myemail"
)

func updateContact(isManager bool, userId string, userObject map[string]interface{}) error {
	jsonBody, err := json.Marshal(userObject)
	if err != nil {
		return err
	}
	if !isManager {
		user := trendlymodels.User{}
		err = json.Unmarshal(jsonBody, &user)
		if err != nil {
			return err
		}

		if user.Email != nil && *user.Email != "" {
			phone := ""
			pCent := 0
			if user.PhoneNumber != nil {
				phone = *user.PhoneNumber
			}
			if user.Profile != nil {
				pCent = *user.Profile.CompletionPercentage
			}
			socialUrl := ""
			if user.PrimarySocial != nil {
				social := trendlymodels.Socials{}
				err = social.Get(userId, *user.PrimarySocial)
				if err != nil {
					return err
				}
				if social.IsInstagram {
					socialUrl = "https://www.instagram.com/" + social.InstaProfile.Username
				} else {
					socialUrl = "https://www.facebook.com/" + social.FBProfile.ID
				}
			}
			contacts := []myemail.ContactDetails{{
				Email:             *user.Email,
				Name:              user.Name,
				Phone:             phone,
				IsManager:         false,
				ProfileCompletion: pCent,
				LastActivityTime:  aws.Int64(time.Now().UnixMilli()),
				CreationTime:      user.CreationTime,
				SocialLink:        socialUrl,
			}}
			// go hubspot.CreateOrUpdateContacts(contacts)
			err := myemail.CreateOrUpdateContacts(contacts)
			if err != nil {
				return err
			}
		}
	} else {
		manager := trendlymodels.Manager{}
		err = json.Unmarshal(jsonBody, &manager)
		if err != nil {
			return err
		}

		brand, _ := trendlymodels.GetMyFirstBrand(userId)
		brandName := ""
		if brand != nil {
			brandName = brand.Name
		}

		if brand.Profile != nil && brand.Profile.PhoneNumber != nil {
			manager.PhoneNumber = *brand.Profile.PhoneNumber
		}

		contacts := []myemail.ContactDetails{{
			Email:            manager.Email,
			Name:             manager.Name,
			Phone:            manager.PhoneNumber,
			IsManager:        true,
			CompanyName:      brandName, // Currenly its difficult to fetch the company name
			LastActivityTime: aws.Int64(time.Now().UnixMilli()),
			CreationTime:     aws.Int64(manager.CreationTime),
		}}

		go hubspot.CreateOrUpdateContacts(contacts)
		err = myemail.CreateOrUpdateContacts(contacts)
		if err != nil {
			return err
		}
	}
	return nil
}
