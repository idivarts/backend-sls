package myemail

import (
	"log"
	"os"
	"strings"

	"github.com/idivarts/backend-sls/pkg/mailer"
	"github.com/idivarts/backend-sls/pkg/myses"
)

const (
	defaultSenderName  = "Trendly Support"
	defaultSenderEmail = "no-reply@idiv.in"
	defaultSESRegion   = "us-east-1"
)

var (
	// sender is resolved once at startup. Swapping or adding a provider means
	// changing this one assignment — nothing else in the package, and no
	// handler, knows which service delivers the mail.
	sender mailer.Sender

	from    mailer.Address
	replyTo string
)

func init() {
	// EMAIL_SENDER_* are the current names. The legacy SENDGRID_NAME /
	// SENDGRID_EMAIL are still honoured as a fallback so a stale deploy cannot
	// produce an empty From, which SES rejects outright — From must match a
	// verified identity.
	from = mailer.Address{
		Name:  firstNonEmpty(os.Getenv("EMAIL_SENDER_NAME"), os.Getenv("SENDGRID_NAME"), defaultSenderName),
		Email: firstNonEmpty(os.Getenv("EMAIL_SENDER_ADDRESS"), os.Getenv("SENDGRID_EMAIL"), defaultSenderEmail),
	}
	replyTo = strings.TrimSpace(os.Getenv("EMAIL_REPLY_TO"))

	sender = myses.New(
		firstNonEmpty(os.Getenv("SES_REGION"), os.Getenv("AWS_REGION"), defaultSESRegion),
		strings.TrimSpace(os.Getenv("SES_CONFIGURATION_SET")),
	)

	log.Printf("myemail: sending as %s via %s", from, sender.Name())
}

// ActiveProvider reports which service outbound mail is routed to.
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
