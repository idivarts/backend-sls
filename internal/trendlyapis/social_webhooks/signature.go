package socialwebhooks

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"

	"github.com/idivarts/backend-sls/pkg/instagram"
	"github.com/idivarts/backend-sls/pkg/messenger"
)

// metaAppSecrets returns the candidate app secrets used to validate Meta
// signatures. A webhook may originate from either the Facebook app or the
// Instagram-Login app, so we accept a match against either secret. Env vars are
// preferred, falling back to the package constants.
func metaAppSecrets() []string {
	return []string{
		os.Getenv("FB_CLIENT_SECRET"),
		os.Getenv("INSTA_CLIENT_SECRET"),
		messenger.ClientSecret,
		instagram.ClientSecret,
	}
}

// verifyMetaSignature validates Meta's X-Hub-Signature-256 header against the
// raw request body, accepting a match against any configured app secret.
//
// header format: "sha256=<hex digest>"
func verifyMetaSignature(rawBody []byte, header string) bool {
	header = strings.TrimSpace(header)
	if !strings.HasPrefix(header, "sha256=") {
		return false
	}
	want := strings.TrimPrefix(header, "sha256=")

	for _, secret := range metaAppSecrets() {
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

// strictSignature reports whether a failed signature check should hard-reject
// the request. Leave WEBHOOK_STRICT_SIGNATURE unset during initial testing and
// flip it on once app secrets are confirmed in the environment.
func strictSignature() bool {
	return os.Getenv("WEBHOOK_STRICT_SIGNATURE") == "true"
}
