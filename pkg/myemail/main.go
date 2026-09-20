// Package myemail renders the HTML templates in templates/ and hands them to
// whichever provider EMAIL_PROVIDER selects.
//
// It owns the template/content concerns only. Delivery lives in pkg/myses and
// pkg/mysendgrid, behind the mailer.Sender interface.
package myemail

import (
	"bytes"
	"fmt"
	"html/template"
	"log"
	"strings"

	"github.com/idivarts/backend-sls/pkg/mailer"
)

type TemplatePath string

// SendCustomHTMLEmail renders templatePath with data and sends it to a single
// recipient via the configured provider.
func SendCustomHTMLEmail(toEmail string, templatePath TemplatePath, subject string, data map[string]interface{}) error {
	htmlBody, textBody, err := render(templatePath, data)
	if err != nil {
		return err
	}
	return send(toEmail, subject, htmlBody, textBody)
}

// SendCustomHTMLEmailToMultipleRecipients renders templatePath once and sends a
// separate copy to each recipient.
//
// The per-recipient loop is deliberate: recipients must never see each other's
// addresses. SendGrid achieved that with one personalization per address; SES's
// SendEmail would instead put every address into a single visible To: header,
// leaking brand managers' emails to one another.
//
// Partial failures are logged, not returned — SendGrid accepted a batch (202)
// even when individual addresses were bad, and several callers turn a non-nil
// error into an HTTP 400 for the whole request. An error is returned only when
// every recipient failed.
func SendCustomHTMLEmailToMultipleRecipients(toEmails []string, templatePath TemplatePath, subject string, data map[string]interface{}) error {
	htmlBody, textBody, err := render(templatePath, data)
	if err != nil {
		return err
	}

	var (
		attempted int
		failures  []string
	)
	for _, toEmail := range toEmails {
		if strings.TrimSpace(toEmail) == "" {
			continue
		}
		attempted++

		if err := send(toEmail, subject, htmlBody, textBody); err != nil {
			log.Printf("myemail: send to %s failed (subject %q): %v", toEmail, subject, err)
			failures = append(failures, fmt.Sprintf("%s: %v", toEmail, err))
		}
	}

	if attempted == 0 {
		return fmt.Errorf("no valid recipients for subject %q", subject)
	}
	if len(failures) == attempted {
		return fmt.Errorf("failed to send to all %d recipients: %s", attempted, strings.Join(failures, "; "))
	}
	if len(failures) > 0 {
		log.Printf("myemail: %d of %d recipients failed for subject %q", len(failures), attempted, subject)
	}
	return nil
}

func send(toEmail, subject, htmlBody, textBody string) error {
	toEmail = strings.TrimSpace(toEmail)
	if toEmail == "" {
		return fmt.Errorf("recipient email is empty")
	}

	return sender.Send(mailer.Message{
		From:     from,
		ReplyTo:  replyTo,
		To:       toEmail,
		Subject:  subject,
		HTMLBody: htmlBody,
		TextBody: textBody,
	})
}

// render parses and executes a template, returning the HTML body alongside the
// text/plain alternative derived from it.
func render(templatePath TemplatePath, data map[string]interface{}) (htmlBody, textBody string, err error) {
	tmpl, err := template.ParseFiles(string(templatePath))
	if err != nil {
		return "", "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", "", err
	}

	htmlBody = buf.String()
	return htmlBody, htmlToText(htmlBody), nil
}
