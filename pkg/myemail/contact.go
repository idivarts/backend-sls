package myemail

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

type ContactDetails struct {
	Email             string
	Name              string // Will be split into First and Last name
	Phone             string
	IsManager         bool   // custom: user_type
	CompanyName       string // custom: company
	SocialLink        string
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

func FetchContacts() ([]ContactDetails, error) {
	sendgridAPIKey := apiKey // Set this to your SendGrid API key

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

	var contacts []ContactDetails
	for _, res := range response.Result {
		contact := ContactDetails{
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
	sendgridAPIKey := apiKey // Set this to your SendGrid API key

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
