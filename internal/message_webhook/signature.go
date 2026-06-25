package messagewebhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"

	"github.com/idivarts/backend-sls/pkg/instagram"
	"github.com/idivarts/backend-sls/pkg/facebook"
)

// verifyWebhookSignature validates Meta's X-Hub-Signature-256 header against the
// raw request body. A webhook may originate from either the Facebook app or the
// Instagram-Login app, so we accept a match against either app secret.
//
// header format: "sha256=<hex digest>"
func verifyWebhookSignature(rawBody []byte, header string) bool {
	header = strings.TrimSpace(header)
	if !strings.HasPrefix(header, "sha256=") {
		return false
	}
	want := strings.TrimPrefix(header, "sha256=")

	for _, secret := range candidateAppSecrets() {
		if secret == "" {
			continue
		}
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(rawBody)
		got := hex.EncodeToString(mac.Sum(nil))
		if hmac.Equal([]byte(got), []byte(want)) {
			return true
		}
	}
	return false
}

// candidateAppSecrets returns the app secrets to test a signature against,
// preferring env vars and falling back to the package constants.
func candidateAppSecrets() []string {
	return []string{
		os.Getenv("FB_CLIENT_SECRET"),
		os.Getenv("INSTA_CLIENT_SECRET"),
		facebook.ClientSecret,
		instagram.ClientSecret,
	}
}
