package ai

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
)

const maxContextChars = 8000

func loadBrand(brandID string) (*trendlymodels.Brand, error) {
	if brandID == "" {
		return nil, errors.New("brandId is required")
	}
	var b trendlymodels.Brand
	if err := b.Get(brandID); err != nil {
		return nil, err
	}
	return &b, nil
}

func verifyBrandAccess(brandID, managerID string) bool {
	if brandID == "" || managerID == "" {
		return false
	}
	var member trendlymodels.BrandMember
	return member.Get(brandID, managerID) == nil
}

func buildSystemPrompt(brand *trendlymodels.Brand, module, brandID, contextID, focusedText string) string {
	var sb strings.Builder
	sb.WriteString("You are an AI assistant for ")
	if brand != nil {
		sb.WriteString(brand.Name)
	}
	if brand != nil && brand.Profile != nil && len(brand.Profile.Industries) > 0 {
		sb.WriteString(", a ")
		sb.WriteString(strings.Join(brand.Profile.Industries, "/"))
		sb.WriteString(" brand")
	}
	sb.WriteString(".\n")
	if brand != nil && brand.AIVoice != nil && *brand.AIVoice != "" {
		sb.WriteString("Brand voice: ")
		sb.WriteString(*brand.AIVoice)
		sb.WriteString("\n")
	}
	// Pre-feed the brand's long-term memory. Because buildSystemPrompt runs fresh
	// on every message against a freshly loaded brand, every conversation — new or
	// old — always sees the latest memory the moment the user sends a message.
	if brand != nil && brand.AIMemory != nil && strings.TrimSpace(*brand.AIMemory) != "" {
		mem := strings.TrimSpace(*brand.AIMemory)
		if len(mem) > maxContextChars {
			mem = mem[:maxContextChars]
		}
		sb.WriteString("Brand memory (durable facts the user has shared before — always honor these and do not re-ask for anything already stated here):\n")
		sb.WriteString(mem)
		sb.WriteString("\n")
	}
	if module != "" {
		sb.WriteString("Module: ")
		sb.WriteString(module)
		sb.WriteString("\n")
	}

	if ctxStr := loadModuleContext(module, brandID, contextID); ctxStr != "" {
		sb.WriteString("Module context:\n")
		if len(ctxStr) > maxContextChars {
			ctxStr = ctxStr[:maxContextChars] + "\n…(truncated)"
		}
		sb.WriteString(ctxStr)
		sb.WriteString("\n")
	}

	if focusedText != "" {
		sb.WriteString("The user is focused on this text: \"")
		sb.WriteString(focusedText)
		sb.WriteString("\"\n")
	}

	// Memory-writing capability is available in every module — appended here,
	// before the per-module instruction blocks that return early below.
	sb.WriteString(memoryInstructions)

	if module == moduleOnboarding {
		sb.WriteString(onboardingInstructions)
		return sb.String()
	}

	if module == moduleStrategy {
		sb.WriteString(strategyInstructions)
		sb.WriteString(controlsInstructions)
		return sb.String()
	}

	if module == moduleCalendar {
		sb.WriteString(calendarInstructions)
		sb.WriteString(controlsInstructions)
		sb.WriteString(imageGenInstructions)
		return sb.String()
	}

	sb.WriteString("\nAnswer concisely. Match the brand voice. If a question is ambiguous, ask one clarifying question.")
	sb.WriteString(controlsInstructions)
	if moduleHasImageGen(module) {
		sb.WriteString(imageGenInstructions)
	}
	return sb.String()
}

// imageGenInstructions tells the model it can both SEE images the user attaches
// and CREATE images with the generate_image tool (content/calendar modules).
const imageGenInstructions = "\n\nYou can see images the user attaches and you can create images with the " +
	"generate_image tool. When the user asks you to create, generate, draw, design, or edit an " +
	"image/visual/graphic, call generate_image with a detailed visual prompt. To edit or transform " +
	"an existing image (one the user attached this turn, or an image already on the content), pass " +
	"its URL(s) as inputImages for image-to-image. After it returns, refer to the new image " +
	"naturally — it is shown to the user automatically. Never call it for non-visual requests."

// controlsInstructions tells the model how to use the answer-control tools that
// are available in every module.
const controlsInstructions = "\n\nWhen a question has a small, known set of valid answers, ask it with the " +
	"ask_options tool instead of plain text. When you need a value with a specific " +
	"format (phone, website/URL, email), ask with the ask_input tool. Ask one question " +
	"at a time and never call these tools for open-ended questions."

// onboardingInstructions defines the conversational brand-setup persona. The
// current draft-brand state is injected as module context above, so the model
// knows which fields are still missing.
const onboardingInstructions = "\n\nYou are guiding a new user through setting up their brand on Trendly, one " +
	"friendly question at a time. Collect, in a natural order: brand name, a short " +
	"'about', phone number, website (optional), the industries/categories they operate " +
	"in, and how established the brand is. Optionally ask the short survey questions " +
	"(where they heard about us, what they'll use Trendly for, expected monthly content " +
	"volume).\n\n" +
	"Rules:\n" +
	"- Ask ONE thing at a time. Keep messages short and warm.\n" +
	"- For constrained answers (industries, brand age, survey questions) use the ask_options tool. " +
	"For brand age, the option labels must map to: 'Just starting' → JUST_STARTING, 'Less than a year' → LT_1, " +
	"'1 to 5 years' → LT_5, '5+ years' → GT_5.\n" +
	"- For phone use ask_input with inputType 'phone'; for website use ask_input with inputType 'url' and mark it optional.\n" +
	"- As soon as you learn any value, call set_brand_fields with just that field. If it is rejected as invalid, ask again.\n" +
	"- The 'Module context' above shows what is already saved — do not re-ask for fields that are filled.\n" +
	"- Once brand name, phone, at least one industry, and brand age are all saved, call complete_onboarding. " +
	"If it reports missing fields, ask for those next. When it succeeds, send a short, warm closing message."

func loadModuleContext(module, brandID, contextID string) string {
	if brandID == "" {
		return ""
	}
	ctx := context.Background()
	switch module {
	case "strategy":
		if contextID == "" {
			return ""
		}
		strat, err := trendlymodels.GetStrategy(ctx, brandID, contextID)
		if err != nil {
			return ""
		}
		var parts []string
		if strat.Name != "" {
			parts = append(parts, "Strategy: "+strat.Name)
		}
		if strat.Objective != "" {
			parts = append(parts, "Objective: "+strat.Objective)
		}
		if strat.MarkdownContent != "" {
			parts = append(parts, "Document:\n"+strat.MarkdownContent)
		}
		return strings.Join(parts, "\n\n")

	case "calendar":
		if contextID != "" {
			return loadContentBrief(brandID, contextID)
		}
		return loadCalendarMonth(brandID)

	case "content":
		if contextID == "" {
			return ""
		}
		return loadContentBrief(brandID, contextID)

	case moduleOnboarding:
		return loadOnboardingState(brandID)
	}
	return ""
}

// loadOnboardingState summarises which brand fields are already saved on the
// draft brand, so the onboarding model knows what is left to ask.
func loadOnboardingState(brandID string) string {
	var brand trendlymodels.Brand
	if err := brand.Get(brandID); err != nil {
		return "Nothing saved yet."
	}

	var saved []string
	add := func(label, val string) {
		if strings.TrimSpace(val) != "" {
			saved = append(saved, label+": "+val)
		}
	}
	deref := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	add("Brand name", brand.Name)
	if brand.Profile != nil {
		add("About", deref(brand.Profile.About))
		add("Phone", deref(brand.Profile.PhoneNumber))
		add("Website", deref(brand.Profile.Website))
		if len(brand.Profile.Industries) > 0 {
			add("Industries", strings.Join(brand.Profile.Industries, ", "))
		}
	}
	add("Brand age", deref(brand.Age))
	if brand.Survey != nil {
		add("Survey source", deref(brand.Survey.Source))
		add("Survey purpose", deref(brand.Survey.Purpose))
		add("Survey content volume", deref(brand.Survey.CollaborationValue))
	}

	if len(saved) == 0 {
		return "Nothing saved yet — start by welcoming them and asking the brand name."
	}
	return "Already saved:\n" + strings.Join(saved, "\n")
}

func loadContentBrief(brandID, contentID string) string {
	ct, err := trendlymodels.GetContent(brandID, contentID)
	if err != nil {
		return ""
	}
	return contentBriefText(ct)
}

// contentBriefText renders a content doc into the compact brief the AI prompts
// use. Shared by the persisted-doc path (loadContentBrief) and the live-edits
// path (briefFromFields) so both produce an identical shape.
func contentBriefText(ct *trendlymodels.Content) string {
	if ct == nil {
		return ""
	}
	var parts []string
	if ct.Title != "" {
		parts = append(parts, "Title: "+ct.Title)
	}
	if len(ct.Platforms) > 0 {
		parts = append(parts, "Platforms: "+strings.Join(ct.Platforms, ", "))
	} else if ct.Platform != "" {
		parts = append(parts, "Platform: "+ct.Platform)
	}
	if ct.ContentFormat != "" {
		parts = append(parts, "Format: "+ct.ContentFormat)
	}
	if ct.Description != "" {
		parts = append(parts, "Brief: "+ct.Description)
	}
	if ct.Caption != "" {
		parts = append(parts, "Caption: "+ct.Caption)
	}
	if ct.Hashtags != "" {
		parts = append(parts, "Hashtags: "+ct.Hashtags)
	}
	if ct.Script != "" {
		parts = append(parts, "Script: "+ct.Script)
	}
	if summary := summariseAttachments(ct.Attachments); summary != "" {
		parts = append(parts, summary)
	}
	return strings.Join(parts, "\n")
}

// briefFromFields builds the content brief from the live editor fields the app
// sends with a generation request. These reflect the user's current, possibly
// unsaved, edits — so the AI works against what's on screen now rather than the
// last-saved Firestore doc. Returns "" when nothing usable is supplied.
func briefFromFields(title, platform, format, description, caption, hashtags, script string) string {
	ct := &trendlymodels.Content{
		Title:         strings.TrimSpace(title),
		Platform:      strings.TrimSpace(platform),
		ContentFormat: strings.TrimSpace(format),
		Description:   strings.TrimSpace(description),
		Caption:       strings.TrimSpace(caption),
		Hashtags:      strings.TrimSpace(hashtags),
		Script:        strings.TrimSpace(script),
	}
	return contentBriefText(ct)
}

// summariseAttachments produces a short, model-friendly description of the media
// currently on a content piece so the AI chat knows what visuals/video exist.
func summariseAttachments(list []trendlymodels.ContentAttachment) string {
	if len(list) == 0 {
		return ""
	}
	images, videos := 0, 0
	var urls []string
	for _, att := range list {
		switch att.Type {
		case "video", "reel":
			videos++
		default:
			images++
			if att.ImageURL != "" {
				urls = append(urls, att.ImageURL)
			}
		}
	}
	if images == 0 && videos == 0 {
		return ""
	}
	var segs []string
	if images > 0 {
		segs = append(segs, fmt.Sprintf("%d image(s)", images))
	}
	if videos > 0 {
		segs = append(segs, fmt.Sprintf("%d video(s)", videos))
	}
	line := "Media attached: " + strings.Join(segs, ", ")
	if len(urls) > 0 {
		line += " — " + strings.Join(urls, ", ")
	}
	return line
}

func loadCalendarMonth(brandID string) string {
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).UnixMilli()
	end := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, time.UTC).UnixMilli()

	contents, err := trendlymodels.ListContentInRange(context.Background(), brandID, start, end, false)
	if err != nil {
		return "No scheduled posts this month."
	}

	var lines []string
	for _, ct := range contents {
		t := time.UnixMilli(ct.PostingTimeStamp).Format("2006-01-02")
		// The [id:…] prefix lets the calendar tools target the exact post the
		// user references without re-stating its title.
		lines = append(lines, fmt.Sprintf("- [id:%s] [%s] %s — %s (%s)", ct.ID, t, ct.Title, ct.ContentFormat, ct.Status))
		if len(lines) >= 50 {
			break
		}
	}
	if len(lines) == 0 {
		return "No scheduled posts this month."
	}
	return "Scheduled posts this month:\n" + strings.Join(lines, "\n")
}
