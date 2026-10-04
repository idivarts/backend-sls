package branch

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// Link type values accepted by the API's `type` field.
const (
	// TypeDefault is an evergreen link: unlimited clicks, deduped by Branch so
	// the same link data returns the same URL. This is what a share link wants.
	TypeDefault = 0
	// TypeOneTime stops resolving after its first click.
	TypeOneTime = 1
	// TypeMarketing shows up in Branch's Marketing dashboard (Quick Links).
	TypeMarketing = 2
)

// LinkData is the `data` dictionary of a Branch link. Branch's own control
// parameters are the `$`-prefixed keys; anything else is free-form app data,
// handed verbatim to the SDK's subscribe callback on open.
//
// Only the keys this codebase actually sets are modelled. Extra is for the rest
// (including the custom app payload) and is flattened into the same JSON object,
// because Branch has no nested home for custom keys.
type LinkData struct {
	// OGTitle/OGDescription/OGImageURL drive the social/chat unfurl preview.
	OGTitle       string `json:"$og_title,omitempty"`
	OGDescription string `json:"$og_description,omitempty"`
	OGImageURL    string `json:"$og_image_url,omitempty"`

	// DeeplinkPath is where the app should land, e.g. "/share/<token>". The RN
	// side reads it in branch.subscribe and parks it (utils/deep-link-intent.ts),
	// which rejects anything that is not an in-app path.
	DeeplinkPath string `json:"$deeplink_path,omitempty"`

	// DesktopURL is where a desktop browser goes — the plain web share URL.
	// FallbackURL covers any platform with no more specific URL set, and is what
	// a mobile browser gets when the app is not installed.
	DesktopURL  string `json:"$desktop_url,omitempty"`
	FallbackURL string `json:"$fallback_url,omitempty"`
	IOSURL      string `json:"$ios_url,omitempty"`
	AndroidURL  string `json:"$android_url,omitempty"`

	// CanonicalURL dedupes the link across channels in Branch's analytics.
	CanonicalURL string `json:"$canonical_url,omitempty"`

	// Extra carries custom (non-`$`) app data, merged into this same object.
	Extra map[string]string `json:"-"`
}

// MarshalJSON flattens Extra alongside the modelled `$` keys. Branch expects one
// flat `data` object, so custom keys cannot live under a nested field.
func (d LinkData) MarshalJSON() ([]byte, error) {
	// alias drops the custom MarshalJSON so the struct half encodes normally
	// (marshalling d directly here would recurse forever).
	type alias LinkData
	buf, err := json.Marshal(alias(d))
	if err != nil {
		return nil, err
	}
	flat := map[string]interface{}{}
	if err := json.Unmarshal(buf, &flat); err != nil {
		return nil, err
	}
	for k, v := range d.Extra {
		// A `$`-prefixed Extra key would silently shadow a modelled control
		// param; the modelled field is the one that should win.
		if v == "" {
			continue
		}
		if _, taken := flat[k]; taken {
			continue
		}
		flat[k] = v
	}
	return json.Marshal(flat)
}

// LinkRequest is the POST /v1/url body. BranchKey is filled in by CreateLink
// from the Credentials, so callers never handle the key.
type LinkRequest struct {
	BranchKey string   `json:"branch_key"`
	Channel   string   `json:"channel,omitempty"`
	Feature   string   `json:"feature,omitempty"`
	Campaign  string   `json:"campaign,omitempty"`
	Stage     string   `json:"stage,omitempty"`
	Tags      []string `json:"tags,omitempty"`
	// Alias is a custom vanity slug. Branch rejects a POST whose alias is already
	// taken, so leave it empty to get a generated short ID.
	Alias string   `json:"alias,omitempty"`
	Type  int      `json:"type,omitempty"`
	Data  LinkData `json:"data"`
}

// CreateLink mints a Branch deep link and returns its short URL.
//
//	POST /v1/url
func (c Credentials) CreateLink(ctx context.Context, req LinkRequest) (string, error) {
	if !c.Configured() {
		return "", ErrNotConfigured
	}
	req.BranchKey = c.Key

	var out struct {
		URL string `json:"url"`
	}
	if err := doJSON(ctx, "POST", "/v1/url", req, &out); err != nil {
		return "", err
	}
	if out.URL == "" {
		return "", fmt.Errorf("branch: create returned no url")
	}
	return out.URL, nil
}

// ReadLink returns the stored configuration of an existing link. Note that for a
// short link this also resets Branch's expiration window on it.
//
//	GET /v1/url?url=…&branch_key=…
func (c Credentials) ReadLink(ctx context.Context, link string) (map[string]interface{}, error) {
	if !c.Configured() {
		return nil, ErrNotConfigured
	}
	q := url.Values{}
	q.Set("url", link)
	q.Set("branch_key", c.Key)

	var out map[string]interface{}
	if err := doJSON(ctx, "GET", "/v1/url?"+q.Encode(), nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// UpdateLink rewrites an existing link's configuration in place, so a shared URL
// that is already circulating can be re-pointed (e.g. refreshing a stale OG
// image) without minting a new one.
//
// Requires the branch_secret. `alias`, `identity`, `type`, `app_id`, `domain` and
// `creation_source` cannot be changed by this call.
//
//	PUT /v1/url?url=…
func (c Credentials) UpdateLink(ctx context.Context, link string, req LinkRequest) error {
	if !c.Configured() {
		return ErrNotConfigured
	}
	if c.Secret == "" {
		return fmt.Errorf("branch: BRANCH_*_SECRET is required to update a link")
	}
	req.BranchKey = c.Key

	body := map[string]interface{}{
		"branch_key":    c.Key,
		"branch_secret": c.Secret,
		"channel":       req.Channel,
		"feature":       req.Feature,
		"campaign":      req.Campaign,
		"stage":         req.Stage,
		"tags":          req.Tags,
		"data":          req.Data,
	}
	q := url.Values{}
	q.Set("url", link)
	return doJSON(ctx, "PUT", "/v1/url?"+q.Encode(), body, nil)
}
