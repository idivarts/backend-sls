// Package crm holds the contact record that the CRM sync backends share.
//
// It exists so that pkg/hubspot and pkg/mysendgrid can speak the same
// vocabulary without depending on each other — each one maps ContactDetails
// onto its own provider's payload.
package crm

import (
	"regexp"
	"strings"
)

// ContactDetails is one synced person. The comments note the custom-field name
// each optional attribute maps to on the provider side.
type ContactDetails struct {
	Email             string
	Name              string // Split into first/last by the provider mappers
	Phone             string
	IsManager         bool   // custom: user_type
	CompanyName       string // custom: company
	SocialLink        string
	ProfileCompletion int    // custom: profile_completion
	CreationTime      *int64 // custom: creation_time
	LastActivityTime  *int64 // custom: last_use_time
}

var (
	nonAlphaRe = regexp.MustCompile(`[^a-zA-Z ]+`)
	spacesRe   = regexp.MustCompile(`\s+`)
)

// CleanName strips everything that is not a letter or a space, collapses
// repeated whitespace and trims the result.
func CleanName(name string) string {
	cleaned := nonAlphaRe.ReplaceAllString(name, " ")
	return strings.TrimSpace(spacesRe.ReplaceAllString(cleaned, " "))
}

// SplitName splits a full name into [first] or [first, last].
func SplitName(fullName string) []string {
	parts := strings.Split(fullName, " ")
	if len(parts) == 0 {
		return []string{""}
	} else if len(parts) == 1 {
		return []string{parts[0]}
	}
	lName := strings.Join(parts[1:], " ")
	return []string{parts[0], lName}
}
