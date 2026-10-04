package sharing

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
)

func TestJoinPlatforms(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want string
	}{
		{"none", nil, ""},
		{"one", []string{"instagram"}, "Instagram"},
		{"two", []string{"instagram", "linkedin"}, "Instagram and LinkedIn"},
		{"three", []string{"instagram", "linkedin", "youtube"}, "Instagram, LinkedIn and 1 more"},
		{"four", []string{"instagram", "linkedin", "youtube", "reddit"}, "Instagram, LinkedIn and 2 more"},
		// linkedin and linkedin_page share a label, so the card must not read
		// "LinkedIn and LinkedIn".
		{"deduped label", []string{"linkedin", "linkedin_page"}, "LinkedIn"},
		{"blanks skipped", []string{"", "instagram", ""}, "Instagram"},
		{"unknown platform still named", []string{"threads"}, "Threads"},
		{"empty id alone", []string{""}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := joinPlatforms(c.in); got != c.want {
				t.Errorf("joinPlatforms(%v) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestClip(t *testing.T) {
	t.Run("leaves a short string alone", func(t *testing.T) {
		if got := clip("Q4 Growth Plan", 70); got != "Q4 Growth Plan" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("cuts on a word boundary", func(t *testing.T) {
		got := clip("the quick brown fox jumps over the lazy dog", 20)
		if !strings.HasSuffix(got, "…") {
			t.Errorf("got %q, want an ellipsis", got)
		}
		// Must not split "brown" mid-word.
		if strings.Contains(got, "bro…") {
			t.Errorf("got %q, want a whole-word cut", got)
		}
		if n := utf8.RuneCountInString(got); n > 21 {
			t.Errorf("got %q (%d runes), want <= limit + ellipsis", got, n)
		}
	})

	t.Run("hard-cuts a single long word", func(t *testing.T) {
		got := clip(strings.Repeat("a", 50), 10)
		if got != strings.Repeat("a", 10)+"…" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("collapses newlines from markdown prose", func(t *testing.T) {
		got := clip("Grow\n\nreach   on\tInstagram", 70)
		if got != "Grow reach on Instagram" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("drops dangling punctuation before the ellipsis", func(t *testing.T) {
		if got := clip("one two three, four five", 15); strings.Contains(got, ",…") {
			t.Errorf("got %q", got)
		}
	})
}

func TestDescribeContent(t *testing.T) {
	cases := []struct {
		name string
		in   trendlymodels.Content
		want string
	}{
		{
			"platform, format and date",
			trendlymodels.Content{
				Platforms:        []string{"instagram"},
				ContentFormat:    trendlymodels.ContentFormatReel,
				PostingTimeStamp: time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC).UnixMilli(),
			},
			"Instagram reel · 12 Oct 2026",
		},
		{
			"no date",
			trendlymodels.Content{Platforms: []string{"linkedin"}, ContentFormat: trendlymodels.ContentFormatPost},
			"LinkedIn post",
		},
		{
			// Old docs carry the capitalised single-platform field instead.
			"falls back to the legacy Platform field",
			trendlymodels.Content{Platform: "instagram", ContentFormat: trendlymodels.ContentFormatStory},
			"Instagram story",
		},
		{
			"nothing known at all",
			trendlymodels.Content{},
			"Content",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ct := c.in
			if got := describeContent(&ct); got != c.want {
				t.Errorf("describeContent() = %q, want %q", got, c.want)
			}
		})
	}
}

// The attachment that will actually be published outranks the design's captured
// cover, which outranks nothing — an AI-studio item whose render is not attached
// yet still gets an image.
func TestContentImagePrefersAttachment(t *testing.T) {
	withBoth := trendlymodels.Content{
		Attachments: []trendlymodels.ContentAttachment{{ImageURL: "https://cdn/attach.png"}},
		DesignRef:   &trendlymodels.ContentDesignRef{RenderURL: "https://cdn/render.png"},
	}
	if got := contentImage(&withBoth); got != "https://cdn/attach.png" {
		t.Errorf("got %q, want the attachment", got)
	}

	renderOnly := trendlymodels.Content{
		DesignRef: &trendlymodels.ContentDesignRef{RenderURL: "https://cdn/render.png"},
	}
	if got := contentImage(&renderOnly); got != "https://cdn/render.png" {
		t.Errorf("got %q, want the design render", got)
	}

	// A video attachment with no still must not win over the design cover.
	videoOnly := trendlymodels.Content{
		Attachments: []trendlymodels.ContentAttachment{{PlayURL: "https://cdn/clip.mp4"}},
		DesignRef:   &trendlymodels.ContentDesignRef{RenderURL: "https://cdn/render.png"},
	}
	if got := contentImage(&videoOnly); got != "https://cdn/render.png" {
		t.Errorf("got %q, want the design render", got)
	}

	if got := contentImage(&trendlymodels.Content{}); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestFormatRange(t *testing.T) {
	ms := func(y int, m time.Month) int64 {
		return time.Date(y, m, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
	}
	cases := []struct {
		name       string
		start, end int64
		want       string
	}{
		{"same month", ms(2026, time.October), ms(2026, time.October), "Oct 2026"},
		{"same year drops the repeat", ms(2026, time.October), ms(2026, time.December), "Oct – Dec 2026"},
		{"across years keeps both", ms(2026, time.November), ms(2027, time.February), "Nov 2026 – Feb 2027"},
		{"open ended", ms(2026, time.October), 0, "from Oct 2026"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := formatRange(c.start, c.end); got != c.want {
				t.Errorf("formatRange() = %q, want %q", got, c.want)
			}
		})
	}
}

// The fingerprint is what decides whether an already-minted link gets re-pointed,
// so it has to change on any visible part of the card and only on those.
func TestPreviewFingerprint(t *testing.T) {
	base := sharePreview{title: "Q4 Growth Plan", description: "Instagram and LinkedIn", imageURL: "https://cdn/a.png"}

	if base.fingerprint() != base.fingerprint() {
		t.Error("fingerprint is not stable")
	}
	for _, changed := range []sharePreview{
		{title: "Q1 Growth Plan", description: base.description, imageURL: base.imageURL},
		{title: base.title, description: "Instagram only", imageURL: base.imageURL},
		{title: base.title, description: base.description, imageURL: "https://cdn/b.png"},
		// An image appearing where there was none must also count as a change.
		{title: base.title, description: base.description, imageURL: ""},
	} {
		if changed.fingerprint() == base.fingerprint() {
			t.Errorf("fingerprint unchanged for %+v", changed)
		}
	}

	// Field boundaries are delimited, so moving text between them is still a change.
	a := sharePreview{title: "ab", description: "c"}
	b := sharePreview{title: "a", description: "bc"}
	if a.fingerprint() == b.fingerprint() {
		t.Error("fingerprint collides across field boundaries")
	}
}

// A byte-based limit would clip a multi-byte title far shorter than intended and
// could slice a rune in half, putting invalid UTF-8 into og:title.
func TestClipIsRuneSafe(t *testing.T) {
	t.Run("counts characters, not bytes", func(t *testing.T) {
		// 12 Devanagari characters, ~36 bytes — must survive a 20-char limit whole.
		title := "दीपावली अभियान"
		if got := clip(title, 20); got != title {
			t.Errorf("got %q, want the title untouched", got)
		}
	})

	t.Run("never emits a broken rune", func(t *testing.T) {
		for _, in := range []string{
			strings.Repeat("🚀", 40),
			strings.Repeat("दी", 40),
			"Diwali 🪔 campaign plan for the whole quarter and then some more text",
		} {
			got := clip(in, 10)
			if !utf8.ValidString(got) {
				t.Errorf("clip(%q) produced invalid UTF-8: %q", in, got)
			}
			if n := utf8.RuneCountInString(got); n > 11 {
				t.Errorf("clip(%q) = %d runes, want <= 11", in, n)
			}
		}
	})
}
