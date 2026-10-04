package sharing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
)

// Limits for the unfurl card. Titles are clipped by every platform somewhere
// around 60–70 characters and descriptions around 200, so trimming here keeps
// the cut on a word boundary instead of mid-word wherever it renders.
const (
	maxTitleLen       = 70
	maxDescriptionLen = 200
)

// sharePreview is the unfurl card a chat app or social network shows for a link.
type sharePreview struct {
	title       string
	description string
	imageURL    string
}

// fingerprint identifies this exact card, so a cached deep link whose resource has
// since been renamed (or has gained an image) can be detected and re-pointed
// rather than left showing a stale card forever.
func (p sharePreview) fingerprint() string {
	sum := sha256.Sum256([]byte(p.title + "\x00" + p.description + "\x00" + p.imageURL))
	return hex.EncodeToString(sum[:])[:16]
}

// platformLabels renders the stored platform ids the way the product writes them.
var platformLabels = map[string]string{
	trendlymodels.PlatformInstagram:    "Instagram",
	trendlymodels.PlatformFacebook:     "Facebook",
	trendlymodels.PlatformYouTube:      "YouTube",
	trendlymodels.PlatformLinkedIn:     "LinkedIn",
	trendlymodels.PlatformLinkedInPage: "LinkedIn",
	trendlymodels.PlatformTwitter:      "X",
	trendlymodels.PlatformReddit:       "Reddit",
}

func platformLabel(p string) string {
	if p == "" {
		return ""
	}
	if label, ok := platformLabels[strings.ToLower(p)]; ok {
		return label
	}
	// Unknown/new platform: title-case the id rather than dropping it, so a
	// platform added to the app before this map reaches it still reads sensibly.
	r := []rune(p)
	return strings.ToUpper(string(r[0])) + string(r[1:])
}

// joinPlatforms renders a platform list for a one-line summary, deduped because
// linkedin and linkedin_page share a label.
func joinPlatforms(platforms []string) string {
	seen := map[string]bool{}
	labels := []string{}
	for _, p := range platforms {
		if p == "" {
			continue
		}
		label := platformLabel(p)
		if seen[label] {
			continue
		}
		seen[label] = true
		labels = append(labels, label)
	}
	switch len(labels) {
	case 0:
		return ""
	case 1:
		return labels[0]
	case 2:
		return labels[0] + " and " + labels[1]
	default:
		// Past three, naming them all crowds the card out.
		return fmt.Sprintf("%s, %s and %d more", labels[0], labels[1], len(labels)-2)
	}
}

// oneLine collapses the whitespace in prose that may be markdown (a strategy
// objective, a caption) so it cannot smuggle newlines into the card.
func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// clip trims to n characters on a word boundary, adding an ellipsis when it cut.
//
// Counts runes, not bytes: a title in Devanagari or one carrying an emoji is
// several bytes per character, so a byte limit would both clip it far shorter than
// intended and risk slicing a rune in half — which would put invalid UTF-8 into
// the og:title the unfurl renders.
func clip(s string, n int) string {
	s = oneLine(s)
	if n <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	cut := runes[:n]
	// Back up to the last space so a word is never split; if the back half holds
	// no space at all, a hard cut is the only option.
	for i := len(cut) - 1; i > n/2; i-- {
		if cut[i] == ' ' {
			cut = cut[:i]
			break
		}
	}
	return strings.TrimRight(string(cut), " ,.;:–-") + "…"
}

// buildPreview assembles the card for a share link from the shared resource
// itself, so it reads as "Q4 Growth Plan · Acme" rather than "Content strategy".
//
// Every lookup is best-effort: a failed read costs a less specific card, never
// the link. A generic title is the floor, not the default.
func buildPreview(ctx context.Context, link *trendlymodels.ShareLink) sharePreview {
	brandName := ""
	brandImage := ""
	brand := trendlymodels.Brand{}
	if err := brand.Get(link.BrandID); err == nil {
		brandName = brand.Name
		if brand.Image != nil {
			brandImage = *brand.Image
		}
	}

	var p sharePreview
	switch link.Type {
	case trendlymodels.ShareTypeStrategy:
		p = strategyPreview(ctx, link)
	case trendlymodels.ShareTypeContent:
		p = contentPreview(link)
	case trendlymodels.ShareTypeCalendarMonth:
		p = calendarPreview(ctx, link)
	}

	if p.title == "" {
		p.title = "Shared from Trendly"
	}
	// The brand name is what makes the card recognisable in a chat thread — but
	// only when the title doesn't already say it.
	if brandName != "" && !strings.Contains(strings.ToLower(p.title), strings.ToLower(brandName)) {
		p.title = clip(p.title, maxTitleLen-utf8.RuneCountInString(brandName)-3) + " · " + brandName
	} else {
		p.title = clip(p.title, maxTitleLen)
	}
	p.description = clip(p.description, maxDescriptionLen)
	// Brand logo is the fallback image: better a recognisable mark than a blank
	// card, and the only image a strategy has.
	if p.imageURL == "" {
		p.imageURL = brandImage
	}
	return p
}

func strategyPreview(ctx context.Context, link *trendlymodels.ShareLink) sharePreview {
	p := sharePreview{title: "Content strategy", description: "A shared content strategy, read-only."}

	s, err := trendlymodels.GetStrategy(ctx, link.BrandID, link.ResourceID)
	if err != nil || s == nil {
		return p
	}
	if s.Name != "" {
		p.title = s.Name
	}

	// Prefer the author's own words; fall back to a composed summary so the card
	// still says something concrete when no objective was written.
	if s.Objective != "" {
		p.description = s.Objective
		return p
	}
	parts := []string{"Content strategy"}
	if platforms := joinPlatforms(s.Platforms); platforms != "" {
		parts = append(parts, "for "+platforms)
	}
	if s.Timeline != nil && s.Timeline.StartDate > 0 {
		parts = append(parts, "· "+formatRange(s.Timeline.StartDate, s.Timeline.EndDate))
	}
	p.description = strings.Join(parts, " ")
	return p
}

func contentPreview(link *trendlymodels.ShareLink) sharePreview {
	p := sharePreview{title: "Content", description: "A shared post, read-only."}

	ct, err := trendlymodels.GetContent(link.BrandID, link.ResourceID)
	if err != nil || ct == nil {
		return p
	}
	if ct.Title != "" {
		p.title = ct.Title
	}

	switch {
	case ct.Description != "":
		p.description = ct.Description
	case ct.Caption != "":
		p.description = ct.Caption
	default:
		p.description = describeContent(ct)
	}

	p.imageURL = contentImage(ct)
	return p
}

// describeContent composes "Instagram reel · 12 Oct 2026" for content with no
// description or caption of its own.
func describeContent(ct *trendlymodels.Content) string {
	platforms := joinPlatforms(ct.Platforms)
	if platforms == "" && ct.Platform != "" {
		platforms = platformLabel(ct.Platform)
	}

	head := strings.TrimSpace(platforms + " " + string(ct.ContentFormat))
	if head == "" {
		head = "Content"
	}
	if ct.PostingTimeStamp > 0 {
		return head + " · " + time.UnixMilli(ct.PostingTimeStamp).Format("2 Jan 2006")
	}
	return head
}

// contentImage finds the best available image for a content item: the attachment
// that will actually be published first, then the current design's captured
// cover (AI-studio content whose render has not been attached yet), then nothing.
func contentImage(ct *trendlymodels.Content) string {
	for _, a := range ct.Attachments {
		if a.ImageURL != "" {
			return a.ImageURL
		}
	}
	if ct.DesignRef != nil && ct.DesignRef.RenderURL != "" {
		return ct.DesignRef.RenderURL
	}
	return ""
}

func calendarPreview(ctx context.Context, link *trendlymodels.ShareLink) sharePreview {
	p := sharePreview{title: "Content calendar", description: "A shared month of planned posts, read-only."}

	monthStart, err := time.Parse("2006-01", link.Month)
	if err != nil {
		return p
	}
	p.title = monthStart.Format("January 2006") + " content calendar"

	// Reuses the same query the public calendar endpoint serves, so the card
	// counts exactly what a viewer will see — and needs no extra index.
	contents, err := trendlymodels.ListContentInRange(
		ctx, link.BrandID, monthStart.UnixMilli(), monthStart.AddDate(0, 1, 0).UnixMilli(), false,
	)
	if err != nil {
		return p
	}

	platforms := []string{}
	for _, ct := range contents {
		platforms = append(platforms, ct.Platforms...)
		if p.imageURL == "" {
			p.imageURL = contentImage(&ct)
		}
	}

	noun := "posts"
	if len(contents) == 1 {
		noun = "post"
	}
	p.description = fmt.Sprintf("%d %s planned", len(contents), noun)
	if joined := joinPlatforms(platforms); joined != "" {
		p.description += " across " + joined
	}
	p.description += " in " + monthStart.Format("January 2006") + "."
	return p
}

// formatRange renders a timeline as "Oct – Dec 2026", dropping the repeated year.
func formatRange(startMs, endMs int64) string {
	start := time.UnixMilli(startMs)
	if endMs <= 0 {
		return "from " + start.Format("Jan 2006")
	}
	end := time.UnixMilli(endMs)
	if start.Year() == end.Year() {
		if start.Month() == end.Month() {
			return start.Format("Jan 2006")
		}
		return start.Format("Jan") + " – " + end.Format("Jan 2006")
	}
	return start.Format("Jan 2006") + " – " + end.Format("Jan 2006")
}
