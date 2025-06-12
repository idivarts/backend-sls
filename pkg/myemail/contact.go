package myemail

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
)

type ContactDetails struct {
	Email             string
	Name              string // Will be split into First and Last name
	Phone             string
	IsManager         bool   // custom: user_type
	CompanyName       string // custom: company
	ProfileCompletion int    // custom: profile_completion
	CreationTime      *int64 // custom: creation_time
	LastActivityTime  *int64 // custom: last_use_time
}

func CreateOrUpdateContacts(contacts []ContactDetails) error {
	sendgridAPIKey := apiKey // Set this to your SendGrid API key

	if len(contacts) == 0 {
		return errors.New("empty-array")
	}

	var contactList []map[string]interface{}
	for _, contact := range contacts {
		contactPayload := map[string]interface{}{
			"email": contact.Email,
		}

		if contact.Name != "" {
			contactPayload["first_name"] = strings.Split(contact.Name, " ")[0]
			if parts := splitName(contact.Name); len(parts) > 1 {
				contactPayload["last_name"] = parts[1]
			}
		}

		if contact.Phone != "" {
			contactPayload["phone_number"] = contact.Phone
		}

		// Optionally add custom fields as per SendGrid schema
		customFields := map[string]interface{}{}
		if contact.IsManager {
			customFields["user_type"] = 1
		} else {
			customFields["user_type"] = 2
		}
		if contact.CompanyName != "" {
			customFields["company"] = contact.CompanyName
		}
		if contact.ProfileCompletion > 0 {
			customFields["profile_completion"] = contact.ProfileCompletion
		}
		if contact.CreationTime != nil {
			customFields["creation_time"] = *contact.CreationTime
		}
		if contact.LastActivityTime != nil {
			customFields["last_use_time"] = *contact.LastActivityTime
		}
		contactPayload["custom_fields"] = customFields

		contactList = append(contactList, contactPayload)
	}

	payload := map[string]interface{}{
		"contacts": contactList,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Error marshalling contact data: %v\n", err)
		return err
	}

	url := "https://api.sendgrid.com/v3/marketing/contacts"
	req, err := http.NewRequest("PUT", url, bytes.NewBuffer(jsonData))
	if err != nil {
		log.Printf("Error creating HTTP request: %v\n", err)
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", sendgridAPIKey))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Error making request to SendGrid: %v\n", err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("error: %s", resp.Status)
	}

	log.Printf("SendGrid contact upload response status: %s\n", resp.Status)
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
