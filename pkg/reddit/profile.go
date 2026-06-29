package reddit

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"strings"
)

// User represents the authenticated Reddit account from /api/v1/me.
type User struct {
	ID           string  `json:"id"`       // base-36 id (without the t2_ prefix)
	Name         string  `json:"name"`     // username (without u/)
	IconImg      string  `json:"icon_img"` // avatar URL (may be HTML-escaped + query params)
	TotalKarma   int64   `json:"total_karma"`
	LinkKarma    int64   `json:"link_karma"`
	CommentKarma int64   `json:"comment_karma"`
	CreatedUTC   float64 `json:"created_utc"`
	Subreddit    struct {
		DisplayNamePrefixed string `json:"display_name_prefixed"` // e.g. "u/spez"
		PublicDescription   string `json:"public_description"`
		Title               string `json:"title"`
	} `json:"subreddit"`
}

// AvatarURL returns a clean avatar URL (HTML-unescaped, query stripped).
func (u *User) AvatarURL() string {
	img := html.UnescapeString(u.IconImg)
	if i := strings.Index(img, "?"); i >= 0 {
		img = img[:i]
	}
	return img
}

// GetMe returns the authenticated user's profile. Requires the identity scope.
func GetMe(accessToken string) (*User, error) {
	req, err := authedRequest(http.MethodGet, "/api/v1/me", accessToken, nil)
	if err != nil {
		return nil, err
	}
	body, _, err := doAuthed(req)
	if err != nil {
		return nil, err
	}
	var u User
	if err := json.Unmarshal(body, &u); err != nil {
		return nil, fmt.Errorf("reddit: parse /me: %w", err)
	}
	return &u, nil
}
