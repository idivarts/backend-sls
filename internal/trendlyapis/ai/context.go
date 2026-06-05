package ai

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
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
	_, err := firestoredb.Client.
		Collection("brands").Doc(brandID).
		Collection("members").Doc(managerID).
		Get(context.Background())
	return err == nil
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
		return sb.String()
	}

	sb.WriteString("\nAnswer concisely. Match the brand voice. If a question is ambiguous, ask one clarifying question.")
	sb.WriteString(controlsInstructions)
	return sb.String()
}

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
		doc, err := firestoredb.Client.
			Collection("brands").Doc(brandID).
			Collection("strategies").Doc(contextID).
			Get(ctx)
		if err != nil {
			return ""
		}
		data := doc.Data()
		var parts []string
		if name, ok := data["name"].(string); ok && name != "" {
			parts = append(parts, "Strategy: "+name)
		}
		if obj, ok := data["objective"].(string); ok && obj != "" {
			parts = append(parts, "Objective: "+obj)
		}
		if md, ok := data["markdownContent"].(string); ok && md != "" {
			parts = append(parts, "Document:\n"+md)
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
	doc, err := firestoredb.Client.Collection("brands").Doc(brandID).Get(context.Background())
	if err != nil {
		return "Nothing saved yet."
	}
	data := doc.Data()
	profile, _ := data["profile"].(map[string]any)
	survey, _ := data["survey"].(map[string]any)

	var saved []string
	add := func(label, val string) {
		if strings.TrimSpace(val) != "" {
			saved = append(saved, label+": "+val)
		}
	}
	add("Brand name", getString(data, "name"))
	if profile != nil {
		add("About", getString(profile, "about"))
		add("Phone", getString(profile, "phone"))
		add("Website", getString(profile, "website"))
		if inds := toSlice(profile["industries"]); len(inds) > 0 {
			parts := make([]string, 0, len(inds))
			for _, v := range inds {
				if s, ok := v.(string); ok {
					parts = append(parts, s)
				}
			}
			add("Industries", strings.Join(parts, ", "))
		}
	}
	add("Brand age", getString(data, "age"))
	if survey != nil {
		add("Survey source", getString(survey, "source"))
		add("Survey purpose", getString(survey, "purpose"))
		add("Survey content volume", getString(survey, "collaborationValue"))
	}

	if len(saved) == 0 {
		return "Nothing saved yet — start by welcoming them and asking the brand name."
	}
	return "Already saved:\n" + strings.Join(saved, "\n")
}

func loadContentBrief(brandID, contentID string) string {
	doc, err := firestoredb.Client.
		Collection("brands").Doc(brandID).
		Collection("contents").Doc(contentID).
		Get(context.Background())
	if err != nil {
		return ""
	}
	data := doc.Data()
	var parts []string
	if title, ok := data["title"].(string); ok && title != "" {
		parts = append(parts, "Title: "+title)
	}
	if platform, ok := data["platform"].(string); ok && platform != "" {
		parts = append(parts, "Platform: "+platform)
	}
	if format, ok := data["contentFormat"].(string); ok && format != "" {
		parts = append(parts, "Format: "+format)
	}
	if desc, ok := data["description"].(string); ok && desc != "" {
		parts = append(parts, "Brief: "+desc)
	}
	if caption, ok := data["caption"].(string); ok && caption != "" {
		parts = append(parts, "Caption: "+caption)
	}
	if hashtags, ok := data["hashtags"].(string); ok && hashtags != "" {
		parts = append(parts, "Hashtags: "+hashtags)
	}
	if script, ok := data["script"].(string); ok && script != "" {
		parts = append(parts, "Script: "+script)
	}
	if summary := summariseAttachments(data["attachments"]); summary != "" {
		parts = append(parts, summary)
	}
	return strings.Join(parts, "\n")
}

// summariseAttachments produces a short, model-friendly description of the media
// currently on a content piece so the AI chat knows what visuals/video exist.
func summariseAttachments(raw any) string {
	list, ok := raw.([]any)
	if !ok || len(list) == 0 {
		return ""
	}
	images, videos := 0, 0
	var urls []string
	for _, item := range list {
		att, ok := item.(map[string]any)
		if !ok {
			continue
		}
		switch t, _ := att["type"].(string); t {
		case "video", "reel":
			videos++
		default:
			images++
			if u, ok := att["imageUrl"].(string); ok && u != "" {
				urls = append(urls, u)
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

	iter := firestoredb.Client.
		Collection("brands").Doc(brandID).
		Collection("contents").
		Where("postingTimeStamp", ">=", start).
		Where("postingTimeStamp", "<", end).
		Documents(context.Background())
	defer iter.Stop()

	var lines []string
	count := 0
	for {
		doc, err := iter.Next()
		if err != nil {
			break
		}
		data := doc.Data()
		if archived, _ := data["isArchived"].(bool); archived {
			continue
		}
		title, _ := data["title"].(string)
		format, _ := data["contentFormat"].(string)
		status, _ := data["status"].(string)
		when, _ := toInt64(data["postingTimeStamp"])
		t := time.UnixMilli(when).Format("2006-01-02")
		// The [id:…] prefix lets the calendar tools target the exact post the
		// user references without re-stating its title.
		lines = append(lines, fmt.Sprintf("- [id:%s] [%s] %s — %s (%s)", doc.Ref.ID, t, title, format, status))
		count++
		if count >= 50 {
			break
		}
	}
	if len(lines) == 0 {
		return "No scheduled posts this month."
	}
	return "Scheduled posts this month:\n" + strings.Join(lines, "\n")
}
