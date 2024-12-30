package myemail

import (
	"encoding/base64"
	"fmt"
	"log"
	"os"

	"github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
)

var (
	senderName  = os.Getenv("SENDGRID_NAME")
	senderEmail = os.Getenv("SENDGRID_EMAIL")
	apiKey      = ""
)

func init() {
	base64key := os.Getenv("SENDGRID_API_KEY")
	// Decode the string
	decodedBytes, err := base64.StdEncoding.DecodeString(base64key)
	if err != nil {
		log.Fatalf("Failed to decode base64 string: %v", err)
	}

	// Convert the bytes to string
	apiKey = string(decodedBytes)
	// fmt.Println("Decoded string:", decodedStr)

}

// SendEmailUsingTemplate This will be used to send email using template
func SendEmailUsingTemplate(toEmail, templateID string, dynamicData map[string]interface{}) error {

	// Sender details
	from := mail.NewEmail(senderName, senderEmail)

	// Recipient details
	to := mail.NewEmail("", toEmail)

	// Create the mail object
	message := mail.NewV3Mail()
	message.SetFrom(from)

	// Add recipient
	personalization := mail.NewPersonalization()
	personalization.AddTos(to)

	// Add dynamic template data
	for key, value := range dynamicData {
		personalization.SetDynamicTemplateData(key, value)
	}
	message.AddPersonalizations(personalization)

	// Set the template ID
	message.SetTemplateID(templateID)

	// Send the email
	client := sendgrid.NewSendClient(apiKey)
	response, err := client.Send(message)
	if err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	// Log response for debugging
	log.Printf("Response status: %d\nResponse body: %s\nResponse headers: %v\n",
		response.StatusCode, response.Body, response.Headers)

	return nil
}
