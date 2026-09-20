package myemail

import (
	"log"
	"os"
	"strings"

	"github.com/idivarts/backend-sls/pkg/mailer"
	"github.com/idivarts/backend-sls/pkg/mysendgrid"
	"github.com/idivarts/backend-sls/pkg/myses"
)

const (
	providerSES      = "ses"
	providerSendGrid = "sendgrid"

	defaultSenderName  = "Trendly Support"
	defaultSenderEmail = "no-reply@idiv.in"
	defaultSESRegion   = "us-east-1"
)

var (
	// sender is the provider chosen once at startup from EMAIL_PROVIDER.
	sender mailer.Sender

	from    mailer.Address
	replyTo string
)

func init() {
	// EMAIL_SENDER_* are the provider-neutral names. The legacy SENDGRID_NAME /
	// SENDGRID_EMAIL are still honoured as a fallback so a partially rolled-out
	// deploy always resolves to a usable sender instead of an empty From (which
	// SES rejects outright, since From must match a verified identity).
	from = mailer.Address{
		Name:  firstNonEmpty(os.Getenv("EMAIL_SENDER_NAME"), os.Getenv("SENDGRID_NAME"), defaultSenderName),
		Email: firstNonEmpty(os.Getenv("EMAIL_SENDER_ADDRESS"), os.Getenv("SENDGRID_EMAIL"), defaultSenderEmail),
	}
	replyTo = strings.TrimSpace(os.Getenv("EMAIL_REPLY_TO"))

	// Unset (or empty, which is what an undefined GitHub Environment variable
	// resolves to) means SES. Anything unrecognised is loud rather than silent:
	// a typo must not quietly route mail somewhere unintended.
	switch chosen := strings.ToLower(strings.TrimSpace(os.Getenv("EMAIL_PROVIDER"))); chosen {
	case providerSendGrid:
		sender = mysendgrid.New(os.Getenv("SENDGRID_API_KEY"))
	case providerSES, "":
		sender = newSESSender()
	default:
		log.Printf("myemail: unrecognised EMAIL_PROVIDER %q, falling back to %s", chosen, providerSES)
		sender = newSESSender()
	}

	log.Printf("myemail: sending as %s via %s", from, sender.Name())
}

func newSESSender() mailer.Sender {
	return myses.New(
		firstNonEmpty(os.Getenv("SES_REGION"), os.Getenv("AWS_REGION"), defaultSESRegion),
		strings.TrimSpace(os.Getenv("SES_CONFIGURATION_SET")),
	)
}

// ActiveProvider reports which service outbound mail is currently routed to.
func ActiveProvider() string {
	return sender.Name()
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}
