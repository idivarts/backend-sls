package mediastore

// Where a stored object can be read from.
//
// The one place outside this package that needs it is the design studio's
// media panel: it lists a film's own files and lets someone open one, and
// a path alone is not something a browser can show. Everything else here
// hands back a URL with the object it just wrote; this answers for an
// object that was written by something else — the render worker, which
// drops resolved media straight into the project.

import (
	"fmt"
	"strings"
)

// PublicURL is the CDN address of an object key.
func PublicURL(t Target, key string) (string, error) {
	_, cdn, _, err := resolveTarget(t)
	if err != nil {
		return "", err
	}
	if cdn == "" {
		return "", fmt.Errorf("mediastore.PublicURL: the %s distribution is not configured", t)
	}
	return cdn + "/" + strings.TrimPrefix(key, "/"), nil
}
