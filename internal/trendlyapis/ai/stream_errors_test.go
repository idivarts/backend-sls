package ai

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/idivarts/backend-sls/pkg/openrouter"
)

// The error that actually reached a user as silence: a 402 whose body is
// several hundred characters of nested provider JSON.
const realCreditBody = `{"error":{"message":"This request requires more credits, or fewer max_tokens. ` +
	`You requested up to 65536 tokens, but can only afford 29411. To increase, visit ` +
	`https://openrouter.ai/settings/credits and add more credits","code":402,"metadata":{` +
	`"limit_source":"openrouter_credits","previous_errors":[{"code":402},{"code":402}]}}}`

func TestUserFacingStreamError_CreditFailureIsReadable(t *testing.T) {
	err := &openrouter.APIError{Status: 402, Body: realCreditBody}
	got := userFacingStreamError(err)

	if got == "" {
		t.Fatal("a 402 must say something — silence is the bug being fixed")
	}
	if got == genericStreamError {
		t.Error("a 402 should get its own message, not the generic one")
	}
	// Nothing from the provider body may reach the user.
	for _, leak := range []string{"402", "65536", "openrouter", "max_tokens", "credits", "{"} {
		if strings.Contains(strings.ToLower(got), strings.ToLower(leak)) {
			t.Errorf("provider detail %q leaked into the user-facing message: %q", leak, got)
		}
	}
	if len(got) > 200 {
		t.Errorf("message is %d chars; it has to fit a toast", len(got))
	}
}

// A 402 upstream is NOT the org running out of its own AI tokens. Those have a
// separate meter and a real upgrade path; conflating them would send the user
// to buy something that cannot help, which is the mistake this session already
// made once.
func TestUserFacingStreamError_CreditFailureIsNotAnUpgradePrompt(t *testing.T) {
	got := strings.ToLower(userFacingStreamError(&openrouter.APIError{Status: 402, Body: realCreditBody}))
	for _, word := range []string{"upgrade", "higher plan", "top-up", "add credits"} {
		if strings.Contains(got, word) {
			t.Errorf("a 402 must not read as an entitlement prompt, got %q", got)
		}
	}
}

func TestUserFacingStreamError_ByStatus(t *testing.T) {
	cases := []struct {
		status int
		expect string
	}{
		{429, "rate limited"},
		{413, "too long"},
		{401, "rejected"},
		{403, "rejected"},
		{502, "trouble"},
		{503, "trouble"},
		{504, "trouble"},
	}
	for _, c := range cases {
		got := userFacingStreamError(&openrouter.APIError{Status: c.status, Body: "{}"})
		if !strings.Contains(strings.ToLower(got), c.expect) {
			t.Errorf("status %d: expected a message mentioning %q, got %q", c.status, c.expect, got)
		}
	}
}

// Stop is not a failure. Reporting a cancel as an error would pop a toast every
// time the user interrupts a turn on purpose.
func TestUserFacingStreamError_CancelIsSilent(t *testing.T) {
	if got := userFacingStreamError(context.Canceled); got != "" {
		t.Errorf("a cancel must stay silent, got %q", got)
	}
	if got := userFacingStreamError(fmt.Errorf("stream: %w", context.Canceled)); got != "" {
		t.Errorf("a wrapped cancel must stay silent, got %q", got)
	}
}

func TestUserFacingStreamError_TimeoutAndUnknown(t *testing.T) {
	if got := userFacingStreamError(context.DeadlineExceeded); !strings.Contains(got, "too long") {
		t.Errorf("a deadline should say so, got %q", got)
	}
	if got := userFacingStreamError(errors.New("connection reset by peer")); got != genericStreamError {
		t.Errorf("an unrecognised error should get the generic message, got %q", got)
	}
	if got := userFacingStreamError(nil); got != "" {
		t.Errorf("nil must produce nothing, got %q", got)
	}
}

// Context-length failures arrive at different statuses per provider, so the
// text fallback has to catch them.
func TestUserFacingStreamError_ContextLengthFallback(t *testing.T) {
	err := &openrouter.APIError{Status: 400, Body: `{"error":{"message":"maximum context length is 200000 tokens"}}`}
	if got := userFacingStreamError(err); !strings.Contains(got, "too long") {
		t.Errorf("a context-length failure should say so, got %q", got)
	}
}

// The budget split is the fix for the 402: an unset max_tokens authorizes
// against the model ceiling, and thinking is counted inside max_tokens, so the
// remainder left for the document has to stay large enough to hold one.
func TestDesignTokenBudget(t *testing.T) {
	if maxTokensFor(openrouter.TaskDesign) == 0 {
		t.Fatal("design must cap max_tokens, or OpenRouter authorizes the model ceiling")
	}
	if maxTokensFor(openrouter.TaskChat) != 0 {
		t.Error("only design is capped; capping chat risks truncating other output")
	}

	r := reasoningFor(openrouter.TaskDesign)
	if r == nil {
		t.Fatal("design should think")
	}
	if r.Effort != "" && r.MaxTokens != 0 {
		t.Error("send effort OR max_tokens, never both — OpenRouter rejects that")
	}
	if !r.Exclude {
		t.Error("thinking must not stream into the chat bubble")
	}
	if left := designMaxTokens - designReasoningTokens; left < 16000 {
		t.Errorf("only %d tokens left for the document after thinking; a multi-slide "+
			"design would be truncated mid-element", left)
	}
	if reasoningFor(openrouter.TaskChat) != nil {
		t.Error("ordinary chat should not pay for thinking")
	}
}
