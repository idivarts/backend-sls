package trendlyapis

import (
	"encoding/json"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/myemail"
)

func updateContact(isManager bool, userObject map[string]interface{}) error {
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
			contacts := []myemail.ContactDetails{{
				Email:             *user.Email,
				Name:              user.Name,
				Phone:             phone,
				IsManager:         false,
				ProfileCompletion: pCent,
				LastActivityTime:  aws.Int64(time.Now().UnixMilli()),
				CreationTime:      user.CreationTime,
			}}
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

		contacts := []myemail.ContactDetails{{
			Email:            manager.Email,
			Name:             manager.Name,
			Phone:            manager.PhoneNumber,
			IsManager:        true,
			CompanyName:      "", // Currenly its difficult to fetch the company name
			LastActivityTime: aws.Int64(time.Now().UnixMilli()),
			CreationTime:     aws.Int64(manager.CreationTime),
		}}

		err := myemail.CreateOrUpdateContacts(contacts)
		if err != nil {
			return err
		}
	}
	return nil
}
