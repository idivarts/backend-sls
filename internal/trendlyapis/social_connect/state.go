package social_connect

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
)

// OAuthState is packed into the `state` query parameter of every OAuth
// authorization request. Using base64 JSON avoids needing a server-side
// session store (since the portal is a static S3 site).
type OAuthState struct {
	UserID         string                    `json:"userId"`
	Platform       trendlymodels.Platform    `json:"platform"`
	App            string                    `json:"app"`            // "users" | "brands"
	CallbackScheme string                    `json:"callbackScheme"` // deep-link scheme or https prefix
	IssuedAt       int64                     `json:"iat"`            // Unix timestamp for expiry check
	// PKCE support: for Twitter, we need to persist the code_verifier through the redirect.
	// For other platforms this field is empty.
	CodeVerifier string `json:"cv,omitempty"`
}

// Encode serialises the state to a URL-safe base64 string.
func (s *OAuthState) Encode() (string, error) {
	s.IssuedAt = time.Now().Unix()
	b, err := json.Marshal(s)
	if err != nil {
		return "", fmt.Errorf("social_connect: failed to encode state: %w", err)
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// DecodeState parses a base64-encoded state string back into an OAuthState.
// It validates that the state was issued within the last 10 minutes.
func DecodeState(encoded string) (*OAuthState, error) {
	b, err := base64.URLEncoding.DecodeString(encoded)
	if err != nil {
		// Try standard base64 as a fallback (some OAuth providers re-encode)
		b, err = base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, fmt.Errorf("social_connect: invalid state encoding: %w", err)
		}
	}

	var s OAuthState
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("social_connect: failed to decode state: %w", err)
	}

	// Guard against replay attacks / very stale states
	age := time.Now().Unix() - s.IssuedAt
	if age > 600 { // 10 minutes
		return nil, fmt.Errorf("social_connect: state expired (age %ds)", age)
	}

	return &s, nil
}

// CallbackSuccessURL returns the connect portal success URL with query params.
func CallbackSuccessURL(connectBase, platform, callbackScheme, app string) string {
	return connectBase + "/connect/success?platform=" + platform +
		"&callbackScheme=" + callbackScheme +
		"&app=" + app
}

// CallbackErrorURL returns the connect portal error URL with a human-readable message.
func CallbackErrorURL(connectBase, platform, callbackScheme, app, message string) string {
	return connectBase + "/connect/error?platform=" + platform +
		"&callbackScheme=" + callbackScheme +
		"&app=" + app +
		"&message=" + urlEncode(message)
}

func urlEncode(s string) string {
	encoded := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if isUnreserved(c) {
			encoded = append(encoded, c)
		} else {
			encoded = append(encoded, '%', hexChar(c>>4), hexChar(c&0xf))
		}
	}
	return string(encoded)
}

func isUnreserved(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') ||
		(c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.' || c == '~'
}

func hexChar(c byte) byte {
	if c < 10 {
		return '0' + c
	}
	return 'A' + c - 10
}
