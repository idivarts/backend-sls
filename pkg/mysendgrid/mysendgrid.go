// Package mysendgrid delivers mail through SendGrid.
//
// This is the pre-SES path, kept so a stage can be rolled back with
// EMAIL_PROVIDER=sendgrid. Delete this package (and the sendgrid-go dependency)
// once every stage runs on SES. See docs/ses-setup.md.
package mysendgrid

import (
	"errors"
	"fmt"
	"log"

	"github.com/idivarts/backend-sls/pkg/mailer"
	"github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
)

// Sender is the SendGrid implementation of mailer.Sender.
type Sender struct {
	apiKey string
}

var _ mailer.Sender = (*Sender)(nil)

func New(apiKey string) *Sender {
	return &Sender{apiKey: apiKey}
}

func (s *Sender) Name() string { return "sendgrid" }

func (s *Sender) Send(msg mailer.Message) error {
	if s.apiKey == "" {
		return errors.New("sendgrid send failed: SENDGRID_API_KEY is not set")
	}

	from := mail.NewEmail(msg.From.Name, msg.From.Email)
	to := mail.NewEmail("", msg.To)
	message := mail.NewSingleEmail(from, msg.Subject, to, msg.TextBody, msg.HTMLBody)

	if msg.ReplyTo != "" {
		message.SetReplyTo(mail.NewEmail("", msg.ReplyTo))
	}
	for name, value := range msg.Headers {
		message.SetHeader(name, value)
	}

	resp, err := sendgrid.NewSendClient(s.apiKey).Send(message)
	if err != nil {
		return fmt.Errorf("sendgrid send failed: %w", err)
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("sendgrid send failed (%d): %s", resp.StatusCode, resp.Body)
	}

	log.Printf("mysendgrid: accepted message for %s (%d)", msg.To, resp.StatusCode)
	return nil
}
