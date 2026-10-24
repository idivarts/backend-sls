package ai

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/idivarts/backend-sls/pkg/openrouter"
)

// Turning a failed model call into something worth showing a brand manager.
//
// The raw error is an OpenRouter status plus its whole JSON body — several
// hundred characters of nested `previous_errors` for a 402. Sending that to the
// chat would be worse than the silence it replaces, so the detail stays in
// CloudWatch and the user gets a sentence that says what happened and whether
// trying again will help.
//
// Phrased for the person reading it: they did not configure the model router
// and cannot act on a status code, so the message says what they can do next.

const genericStreamError = "Something went wrong talking to the AI. Please try again."

// userFacingStreamError maps a model-call failure to one sentence.
func userFacingStreamError(err error) string {
	if err == nil {
		return ""
	}
	// A cancel is the user pressing Stop, or the turn being torn down. It is not
	// a failure and should never surface as one.
	if errors.Is(err, context.Canceled) {
		return ""
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "That took too long and was stopped. Try a shorter request."
	}

	switch openrouter.StatusOf(err) {
	case http.StatusPaymentRequired: // 402
		// Deliberately NOT the token-wallet wording. The org's AI tokens are a
		// separate meter with its own upgrade path; this is Trendly's own
		// upstream account, which the user can do nothing about.
		return "The AI service is temporarily unavailable for this workspace. " +
			"We've been notified — please try again shortly."
	case http.StatusTooManyRequests: // 429
		return "The AI is rate limited right now. Please try again in a moment."
	case http.StatusRequestEntityTooLarge: // 413
		return "This conversation has grown too long for the model. " +
			"Start a new chat to continue."
	case http.StatusUnauthorized, http.StatusForbidden:
		return "The AI service rejected this request. We've been notified."
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return "The AI provider is having trouble. Please try again in a moment."
	}

	// Context length is reported by different providers at different statuses,
	// so it is matched on the text as a fallback rather than a code.
	if s := strings.ToLower(err.Error()); strings.Contains(s, "context length") ||
		strings.Contains(s, "too many tokens") {
		return "This conversation has grown too long for the model. " +
			"Start a new chat to continue."
	}
	return genericStreamError
}
