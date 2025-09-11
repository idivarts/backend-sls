package myemail

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"log"
	"os"

	"github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
)

type TemplatePath string

var (
	senderName  = os.Getenv("SENDGRID_NAME")
	senderEmail = os.Getenv("SENDGRID_EMAIL")
	apiKey      = ""
)

func init() {
	if os.Getenv("SENDGRID_API_KEY") == "" {
		senderName = "Trendly Support"
		senderEmail = "no-reply@idiv.in"
		apiKey = os.Getenv("SENDGRID_API_KEY")
	} else {
		apiKey = os.Getenv("SENDGRID_API_KEY")
	}
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

func SendCustomHTMLEmail(toEmail string, templatePath TemplatePath, subject string, data map[string]interface{}) error {
	// Load and parse the HTML template
	tmpl, err := template.ParseFiles(string(templatePath))
	if err != nil {
		return err
	}

	var body bytes.Buffer
	if err := tmpl.Execute(&body, data); err != nil {
		return err
	}

	from := mail.NewEmail(senderName, senderEmail)
	to := mail.NewEmail("", toEmail)
	message := mail.NewSingleEmail(from, subject, to, "", body.String())

	client := sendgrid.NewSendClient(apiKey)
	mLog, err := client.Send(message)
	log.Println("Mail Delivery:", mLog.StatusCode, mLog.Body)
	if mLog.StatusCode >= 300 {
		return errors.New(mLog.Body)
	}
	return err
}

func SendCustomHTMLEmailToMultipleRecipients(toEmails []string, templatePath TemplatePath, subject string, data map[string]interface{}) error {
	// Load and parse the HTML template
	tmpl, err := template.ParseFiles(string(templatePath))
	if err != nil {
		return err
	}

	var body bytes.Buffer
	if err := tmpl.Execute(&body, data); err != nil {
		return err
	}

	from := mail.NewEmail(senderName, senderEmail)
	message := mail.NewV3Mail()
	message.SetFrom(from)
	message.Subject = subject
	message.AddContent(mail.NewContent("text/html", body.String()))

	// Create one personalization object for all recipients
	for _, email := range toEmails {
		to := mail.NewEmail("", email)
		personalization := mail.NewPersonalization()
		personalization.AddTos(to)
		message.AddPersonalizations(personalization)
	}

	client := sendgrid.NewSendClient(apiKey)
	_, err = client.Send(message)
	if err != nil {
		log.Printf("Failed to send bulk email: %v", err)
	}
	// log.Println("Mail Data", x)
	return err
}
