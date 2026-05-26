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

	sb.WriteString("\nAnswer concisely. Match the brand voice. If a question is ambiguous, ask one clarifying question.")
	return sb.String()
}

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
	}
	return ""
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
	if script, ok := data["script"].(string); ok && script != "" {
		parts = append(parts, "Script: "+script)
	}
	return strings.Join(parts, "\n")
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
		title, _ := data["title"].(string)
		platform, _ := data["platform"].(string)
		when, _ := data["postingTimeStamp"].(int64)
		t := time.UnixMilli(when).Format("2006-01-02")
		lines = append(lines, fmt.Sprintf("- [%s] %s (%s)", t, title, platform))
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
