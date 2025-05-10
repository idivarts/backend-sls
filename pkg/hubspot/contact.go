package hubspot

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
)

type ContactDetails struct {
	Email             string
	Name              string // Will be split into First and Last name
	Phone             string
	IsManager         bool   // user_type
	CompanyName       string //company
	ProfileCompletion int    // profile_completion
	IsEmailVerified   bool   // hs_content_membership_email_confirmed
}

func CreateOrUpdateContacts(contacts []ContactDetails) error {
	accessToken := apiKey
	inputs := []map[string]interface{}{}
	for _, contact := range contacts {
		// Split full name into first and last name
		var firstName, lastName string
		nameParts := splitName(contact.Name)
		if len(nameParts) > 0 {
			firstName = nameParts[0]
		}
		if len(nameParts) > 1 {
			lastName = nameParts[1]
		}

		// Prepare request body for batch upsert
		payloadProperties := map[string]interface{}{}

		if contact.Email != "" {
			payloadProperties["email"] = contact.Email
		}
		if firstName != "" {
			payloadProperties["firstname"] = firstName
		}
		if lastName != "" {
			payloadProperties["lastname"] = lastName
		}
		if contact.Phone != "" {
			payloadProperties["phone"] = contact.Phone
		}

		if contact.IsManager {
			payloadProperties["user_type"] = "manager"
		} else {
			payloadProperties["user_type"] = "user"
		}

		if contact.ProfileCompletion > 0 {
			var f float32 = float32(contact.ProfileCompletion) / 100
			payloadProperties["profile_completion"] = f
		}

		if contact.IsManager && contact.CompanyName != "" {
			payloadProperties["company"] = contact.CompanyName
		}

		// if contact.IsManager {
		// 	payloadProperties["hs_content_membership_email_confirmed"] = contact.IsEmailVerified
		// }

		inputs = append(inputs, map[string]interface{}{
			"id":         contact.Email,
			"idProperty": "email",
			"properties": payloadProperties,
		})
	}

	payload := map[string]interface{}{
		"inputs": inputs,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Error marshalling contact data: %v\n", err)
		return err
	}

	url := "https://api.hubapi.com/crm/v3/objects/contacts/batch/upsert"
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		log.Printf("Error creating HTTP request: %v\n", err)
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Error making request to HubSpot: %v\n", err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("error: %s", resp.Status)
	}

	log.Printf("HubSpot batch upsert response status: %s\n", resp.Status)
	return nil
}

// Helper to split name into first and last
func splitName(fullName string) []string {
	parts := strings.Split(fullName, " ")
	if len(parts) == 0 {
		return []string{""}
	} else if len(parts) == 1 {
		return []string{parts[0]}
	}
	lName := strings.Join(parts[1:], " ")
	return []string{parts[0], lName}
}
