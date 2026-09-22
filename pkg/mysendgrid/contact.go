// Package mysendgrid syncs marketing contacts to SendGrid.
//
// Email delivery moved to Amazon SES (pkg/myses) — this package no longer sends
// mail. Marketing contacts stayed behind because SES has no equivalent: its
// "contact lists" only do unsubscribe management. See docs/ses-setup.md §8 —
// the intended resolution is to fold this into HubSpot and delete it.
package mysendgrid

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/idivarts/backend-sls/pkg/crm"
)

var marketingAPIKey = os.Getenv("SENDGRID_API_KEY")

func CreateOrUpdateContacts(contacts []crm.ContactDetails) error {
	sendgridAPIKey := marketingAPIKey

	if len(contacts) == 0 {
		return errors.New("empty-array")
	}

	var contactList []map[string]interface{}
	for _, contact := range contacts {
		contactPayload := map[string]interface{}{
			"email": contact.Email,
		}

		if contact.Name != "" {
			contact.Name = crm.CleanName(contact.Name)
			contactPayload["first_name"] = strings.Split(contact.Name, " ")[0]
			if parts := crm.SplitName(contact.Name); len(parts) > 1 {
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
		if contact.SocialLink != "" {
			customFields["social_link"] = contact.SocialLink
		}
		if loc, err := time.LoadLocation("Asia/Kolkata"); err == nil {
			if contact.CreationTime != nil {
				t := time.UnixMilli(*contact.CreationTime).In(loc)
				customFields["creation_time"] = t.Format(time.RFC3339)
			}
			if contact.LastActivityTime != nil {
				t := time.UnixMilli(*contact.LastActivityTime).In(loc)
				customFields["last_use_time"] = t.Format(time.RFC3339)
			}
		} else {
			log.Printf("Error loading IST timezone: %v", err)
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
		bodyBytes, _ := io.ReadAll(resp.Body)
		log.Printf("Error response body: %s\n", string(bodyBytes))
		return fmt.Errorf("error: %s", resp.Status)
	}
	var response struct {
		JobID string `json:"job_id"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		log.Printf("Error decoding response: %v\n", err)
		return err
	}

	log.Printf("SendGrid contact upload job ID and status: %s | %s\n", response.JobID, resp.Status)
	return nil
}

func FetchContacts() ([]crm.ContactDetails, error) {
	sendgridAPIKey := marketingAPIKey

	url := "https://api.sendgrid.com/v3/marketing/contacts"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Printf("Error creating HTTP request: %v\n", err)
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", sendgridAPIKey))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Error making request to SendGrid: %v\n", err)
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("error: %s", resp.Status)
	}

	var response struct {
		Result []struct {
			Email        string                 `json:"email"`
			FirstName    string                 `json:"first_name"`
			LastName     string                 `json:"last_name"`
			PhoneNumber  string                 `json:"phone_number"`
			CustomFields map[string]interface{} `json:"custom_fields"`
		} `json:"result"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		log.Printf("Error decoding response: %v\n", err)
		return nil, err
	}

	var contacts []crm.ContactDetails
	for _, res := range response.Result {
		contact := crm.ContactDetails{
			Email: res.Email,
			Name:  fmt.Sprintf("%s %s", res.FirstName, res.LastName),
			Phone: res.PhoneNumber,
		}

		if userType, ok := res.CustomFields["user_type"].(float64); ok {
			contact.IsManager = userType == 1
		}
		if company, ok := res.CustomFields["company"].(string); ok {
			contact.CompanyName = company
		}
		if profileCompletion, ok := res.CustomFields["profile_completion"].(float64); ok {
			contact.ProfileCompletion = int(profileCompletion)
		}
		if creationTime, ok := res.CustomFields["creation_time"].(float64); ok {
			creationTimeInt := int64(creationTime)
			contact.CreationTime = &creationTimeInt
		}
		if lastUseTime, ok := res.CustomFields["last_use_time"].(float64); ok {
			lastUseTimeInt := int64(lastUseTime)
			contact.LastActivityTime = &lastUseTimeInt
		}

		contacts = append(contacts, contact)
	}
	log.Printf("Fetched %d contacts from SendGrid\n", len(contacts))

	return contacts, nil
}

func GetJobStatus(jobID string) (string, error) {
	sendgridAPIKey := marketingAPIKey

	if jobID == "" {
		return "", errors.New("jobID cannot be empty")
	}

	url := fmt.Sprintf("https://api.sendgrid.com/v3/marketing/contacts/imports/%s", jobID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Printf("Error creating HTTP request: %v\n", err)
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", sendgridAPIKey))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Error making request to SendGrid: %v\n", err)
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("error: %s", resp.Status)
	}

	var response map[string]interface{}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		log.Printf("Error decoding response: %v\n", err)
		return "", err
	}

	formattedResponse, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		log.Printf("Error formatting response: %v\n", err)
		return "", err
	}
	log.Printf("Formatted response: %s\n", string(formattedResponse))

	log.Printf("Job ID %s status: %s\n", jobID, response["status"])
	return response["status"].(string), nil
}
