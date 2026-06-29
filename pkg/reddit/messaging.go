package reddit

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// Message is a Reddit private message (a t4 thing).
type Message struct {
	Fullname   string `json:"fullname"` // t4_… fullname (from "name")
	ID         string `json:"id"`
	Subject    string `json:"subject"`
	Body       string `json:"body"`
	Author     string `json:"author"` // sender username
	Dest       string `json:"dest"`   // recipient username
	CreatedUTC int64  `json:"created_utc"`
	New        bool   `json:"new"` // true when unread
}

// GetInbox returns the authenticated user's inbox private messages. Comment
// replies and other non-PM kinds are ignored. Requires the privatemessages
// scope.
func GetInbox(accessToken string, limit int) ([]Message, error) {
	if limit <= 0 {
		limit = 25
	}
	p := fmt.Sprintf("/message/inbox?limit=%s&raw_json=1", itoa(limit))
	req, err := authedRequest(http.MethodGet, p, accessToken, nil)
	if err != nil {
		return nil, err
	}
	body, _, err := doAuthed(req)
	if err != nil {
		return nil, err
	}

	var l listing
	if err := json.Unmarshal(body, &l); err != nil {
		return nil, fmt.Errorf("reddit: parse inbox listing: %w", err)
	}

	out := make([]Message, 0, len(l.Data.Children))
	for _, child := range l.Data.Children {
		if child.Kind != "t4" {
			// Ignore comment-reply (t1) and other non-PM kinds.
			continue
		}
		var raw struct {
			Name       string  `json:"name"`
			ID         string  `json:"id"`
			Subject    string  `json:"subject"`
			Body       string  `json:"body"`
			Author     string  `json:"author"`
			Dest       string  `json:"dest"`
			CreatedUTC float64 `json:"created_utc"`
			New        bool    `json:"new"`
		}
		if err := json.Unmarshal(child.Data, &raw); err != nil {
			return nil, fmt.Errorf("reddit: parse message: %w", err)
		}
		out = append(out, Message{
			Fullname:   raw.Name,
			ID:         raw.ID,
			Subject:    raw.Subject,
			Body:       raw.Body,
			Author:     raw.Author,
			Dest:       raw.Dest,
			CreatedUTC: int64(raw.CreatedUTC),
			New:        raw.New,
		})
	}
	return out, nil
}

// ComposeMessage sends a private message to a user. Requires the privatemessages
// scope.
//
// NOTE: Reddit froze legacy private messages as READ-ONLY in Aug 2025 (replaced
// by Chat). /api/compose may therefore fail at the API for many accounts; we
// return whatever error the API surfaces rather than pre-emptively blocking.
func ComposeMessage(accessToken, to, subject, text string) error {
	form := url.Values{}
	form.Set("to", to)
	form.Set("subject", subject)
	form.Set("text", text)
	_, err := formPost(accessToken, "/api/compose", form)
	return err
}
