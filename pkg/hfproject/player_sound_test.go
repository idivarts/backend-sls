package hfproject

import (
	"strings"
	"testing"
)

// A film with a voiceover that plays in silence is a film nobody can
// check. The page does not autoplay, so there is no policy reason to
// start muted — the user presses play, and a press is the gesture an
// autoplay policy asks for.
func TestThePlayerStartsUnmuted(t *testing.T) {
	page := playerPage("https://cdn.example.com/player.js")
	if strings.Contains(page, "muted") {
		t.Fatalf("the player starts muted:\n%s", page)
	}
	if strings.Contains(page, "autoplay") {
		t.Fatal("the page autoplays, which would need muted back")
	}
	// The two attributes that ARE load-bearing.
	if !strings.Contains(page, "controls") || !strings.Contains(page, "interactive") {
		t.Fatal("controls or interactive went missing")
	}
}
