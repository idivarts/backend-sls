// Package mailer holds the provider-neutral vocabulary for outbound email:
// the message shape and the Sender interface that each provider implements.
//
// It deliberately depends on nothing else in the tree. pkg/myemail composes a
// concrete Sender (pkg/myses or pkg/mysendgrid) and renders templates into a
// Message; the provider packages only know how to put a Message on the wire.
package mailer

import "fmt"

// Address is an RFC 5322 mailbox. Name is optional.
type Address struct {
	Name  string
	Email string
}

// String renders the address for a From/To header, quoting the display name so
// a comma or period in it cannot break the header.
func (a Address) String() string {
	if a.Name == "" {
		return a.Email
	}
	return fmt.Sprintf("%q <%s>", a.Name, a.Email)
}

// Message is a single rendered email to a single recipient.
//
// One recipient per Message is intentional: Trendly frequently mails a set of
// brand managers, and they must never see each other's addresses.
type Message struct {
	From     Address
	ReplyTo  string
	To       string
	Subject  string
	HTMLBody string
	// TextBody is the text/plain alternative. Always set it — HTML-only mail
	// scores badly with spam filters.
	TextBody string
	// Headers carries extra RFC 5322 headers, e.g. List-Unsubscribe on bulk
	// mail. Usually empty for transactional sends.
	Headers map[string]string
}

// Sender delivers a Message through one provider.
type Sender interface {
	Send(msg Message) error
	// Name identifies the provider for logs, e.g. "ses".
	Name() string
}
