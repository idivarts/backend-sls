// Package myses delivers mail through Amazon SES v2.
//
// It implements mailer.Sender and knows nothing about templates or the rest of
// the app — pkg/myemail decides what to send, this decides how.
package myses

import (
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sesv2"
	"github.com/idivarts/backend-sls/pkg/mailer"
)

const (
	// SES enforces a hard per-second send rate, and most callers run inside a
	// request handler or webhook, so a throttled send is retried in-process
	// rather than dropped.
	maxSendAttempts = 4
	retryBaseDelay  = 200 * time.Millisecond
)

// Sender is the SES v2 implementation of mailer.Sender.
type Sender struct {
	region string
	// configurationSet enables delivery/bounce/complaint event publishing. When
	// empty the header is omitted — sends still work, but nothing is tracked.
	configurationSet string

	once   sync.Once
	client *sesv2.SESV2
}

var _ mailer.Sender = (*Sender)(nil)

// New returns a Sender. The AWS client is built lazily on first send so that
// constructing this in a package initializer cannot fail a cold start.
func New(region, configurationSet string) *Sender {
	return &Sender{region: region, configurationSet: configurationSet}
}

func (s *Sender) Name() string { return "ses" }

func (s *Sender) api() *sesv2.SESV2 {
	s.once.Do(func() {
		sess := session.Must(session.NewSessionWithOptions(session.Options{
			SharedConfigState: session.SharedConfigEnable,
			Config:            aws.Config{Region: aws.String(s.region)},
		}))
		s.client = sesv2.New(sess)
	})
	return s.client
}

func (s *Sender) Send(msg mailer.Message) error {
	input := &sesv2.SendEmailInput{
		FromEmailAddress: aws.String(msg.From.String()),
		Destination: &sesv2.Destination{
			ToAddresses: []*string{aws.String(msg.To)},
		},
		Content: &sesv2.EmailContent{
			Simple: &sesv2.Message{
				Subject: content(msg.Subject),
				Body:    &sesv2.Body{Html: content(msg.HTMLBody)},
			},
		},
	}

	if msg.TextBody != "" {
		input.Content.Simple.Body.Text = content(msg.TextBody)
	}
	if s.configurationSet != "" {
		input.ConfigurationSetName = aws.String(s.configurationSet)
	}
	if msg.ReplyTo != "" {
		input.ReplyToAddresses = []*string{aws.String(msg.ReplyTo)}
	}
	for name, value := range msg.Headers {
		input.Content.Simple.Headers = append(input.Content.Simple.Headers, &sesv2.MessageHeader{
			Name:  aws.String(name),
			Value: aws.String(value),
		})
	}

	var lastErr error
	for attempt := 1; attempt <= maxSendAttempts; attempt++ {
		out, err := s.api().SendEmail(input)
		if err == nil {
			log.Printf("myses: accepted message %s for %s", aws.StringValue(out.MessageId), msg.To)
			return nil
		}

		lastErr = err
		if !isRetryable(err) {
			return fmt.Errorf("ses send failed: %w", err)
		}
		if attempt < maxSendAttempts {
			delay := retryBaseDelay * time.Duration(1<<(attempt-1))
			log.Printf("myses: throttled sending to %s (attempt %d/%d), retrying in %s", msg.To, attempt, maxSendAttempts, delay)
			time.Sleep(delay)
		}
	}

	return fmt.Errorf("ses send failed after %d attempts: %w", maxSendAttempts, lastErr)
}

func content(data string) *sesv2.Content {
	return &sesv2.Content{
		Data:    aws.String(data),
		Charset: aws.String("UTF-8"),
	}
}

// isRetryable reports whether err is a transient throttle worth retrying.
// Sending pauses and account suspensions are deliberately excluded — those need
// a human, and retrying only burns the request's remaining time.
func isRetryable(err error) bool {
	var aerr awserr.Error
	if !errors.As(err, &aerr) {
		return false
	}
	switch aerr.Code() {
	case sesv2.ErrCodeTooManyRequestsException, sesv2.ErrCodeLimitExceededException:
		return true
	default:
		return false
	}
}
