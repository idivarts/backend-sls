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
	Email string
	Name  string // Will be split into First and Last name
	Phone string
}

func CreateOrUpdateContact(contact ContactDetails) error {
	accessToken := apiKey

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
	payload := map[string]interface{}{
		"inputs": []map[string]interface{}{
			{
				"id":         contact.Email,
				"idProperty": "email",
				"properties": map[string]string{
					"email":     contact.Email,
					"firstname": firstName,
					"lastname":  lastName,
					"phone":     contact.Phone,
				},
			},
		},
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
