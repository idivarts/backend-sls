package canva

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// PKCE holds a generated verifier/challenge pair for one auth attempt.
type PKCE struct {
	Verifier  string
	Challenge string
}

// NewPKCE generates a high-entropy code verifier and its S256 challenge.
func NewPKCE() (PKCE, error) {
	b := make([]byte, 64)
	if _, err := rand.Read(b); err != nil {
		return PKCE{}, err
	}
	verifier := base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	return PKCE{Verifier: verifier, Challenge: challenge}, nil
}

// RandomState returns a high-entropy state/nonce string.
func RandomState() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// AuthorizeURL builds the Canva authorize URL for the given PKCE challenge and
// state.
func AuthorizeURL(challenge, state string) string {
	q := url.Values{}
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "s256")
	q.Set("scope", DefaultScopes)
	q.Set("response_type", "code")
	q.Set("client_id", clientID)
	q.Set("state", state)
	if redirectURI != "" {
		q.Set("redirect_uri", redirectURI)
	}
	return authorizeURL + "?" + q.Encode()
}

// Token is the OAuth token response.
type Token struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"` // seconds
	Scope        string `json:"scope"`
	TokenType    string `json:"token_type"`
	// ExpiresAtMs is derived (not from Canva) for convenience.
	ExpiresAtMs int64 `json:"-"`
}

// ExchangeCode swaps an authorization code for tokens (backend-only, Basic
// auth). code_verifier is the PKCE verifier from the auth attempt.
func ExchangeCode(code, verifier string) (*Token, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("code_verifier", verifier)
	if redirectURI != "" {
		form.Set("redirect_uri", redirectURI)
	}
	return tokenRequest(form)
}

// RefreshToken exchanges a (single-use) refresh token for a new token pair.
func RefreshToken(refreshToken string) (*Token, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	return tokenRequest(form)
}

func tokenRequest(form url.Values) (*Token, error) {
	if !Configured() {
		return nil, fmt.Errorf("canva: CANVA_CLIENT_ID/SECRET not set")
	}
	req, err := http.NewRequest(http.MethodPost, apiBase+"/oauth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(clientID, clientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("canva token: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("canva token returned %s: %s", resp.Status, string(raw))
	}
	var t Token
	if err := jsonUnmarshal(raw, &t); err != nil {
		return nil, err
	}
	t.ExpiresAtMs = time.Now().Add(time.Duration(t.ExpiresIn) * time.Second).UnixMilli()
	return &t, nil
}
