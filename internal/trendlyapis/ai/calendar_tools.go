package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/openrouter"
)

// moduleCalendar is the AI module that powers the content-calendar chat. It runs
// against the brand's content collection (brands/{brandId}/contents) and uses the
// server tools below to juggle, add, remove, and edit content directly — the
// calendar's live Firestore subscription reflects every change with no refresh.
const moduleCalendar = "calendar"

const (
	toolListCalendar  = "list_calendar"
	toolCreateContent = "create_content"
	toolUpdateContent = "update_content"
	toolMoveContent   = "move_content"
	toolRemoveContent = "remove_content"
)

// calendarInstructions is the conversational persona for the calendar module.
// The current month's posts (with their ids) are injected as module context
// above, so the model can map what the user references to a concrete content id.
const calendarInstructions = "\n\nYou are the brand's AI Content Expert for the content calendar. You can operate the " +
	"calendar directly using the tools — never just describe a change you could make; make it, then briefly confirm.\n\n" +
	"What you can do:\n" +
	"- move_content: reschedule a post to a new date (juggle the plan). You CANNOT move a post whose status is " +
	"'scheduled' or 'posted' — tell the user it must be unscheduled first.\n" +
	"- create_content: add a new post (title, idea, date YYYY-MM-DD, and type — one of reel, post, story, carousel, live, text). " +
	"'text' is a plain-text post (no media) for Facebook / LinkedIn / X.\n" +
	"- update_content: change a post's title, idea, date, or type ONLY. These are the only things editable from the " +
	"calendar — if asked to change the caption, script, hashtags, attachments or other inner details, politely say those " +
	"are edited on the content page, not here.\n" +
	"- remove_content: take a post off the calendar (it is archived, so it can be restored).\n" +
	"- list_calendar: look up posts in a given month/year when the user references something outside the month shown above.\n\n" +
	"Rules:\n" +
	"- The 'Module context' above lists this month's posts with their ids in the form [id:XXXX]. When the user has focused " +
	"specific posts (shown as focused text, each carrying its id), act on exactly those ids. Otherwise match by title/date " +
	"from the context list. If a reference is ambiguous between several posts, ask with the ask_options tool before acting.\n" +
	"- Apply changes immediately; the user sees them appear on the calendar live. Keep confirmations short.\n" +
	"- For juggling several posts at once, call the tools one per post in the same turn."

// calendarServerTools are executed on the backend and only attached when the
// conversation's module is calendar.
func calendarServerTools() []openrouter.Tool {
	return []openrouter.Tool{
		openrouter.NewFunctionTool(
			toolListCalendar,
			"List the brand's scheduled (non-archived) content for a month. Use it only when the user references "+
				"posts outside the month already shown in the context above.",
			openrouter.ObjectSchema(map[string]any{
				"month": openrouter.NumberProp("Month number 1-12 (defaults to the current month)."),
				"year":  openrouter.NumberProp("Four-digit year (defaults to the current year)."),
			}, nil),
		),
		openrouter.NewFunctionTool(
			toolCreateContent,
			"Add a new content item to the calendar.",
			openrouter.ObjectSchema(map[string]any{
				"title":    openrouter.StringProp("Short title for the post."),
				"idea":     openrouter.StringProp("A one-line idea / brief for the post."),
				"date":     openrouter.StringProp("The date to place it on, as YYYY-MM-DD."),
				"type":     openrouter.EnumProp("The content format. 'text' is a plain-text post (no media).", []string{"reel", "post", "story", "carousel", "live", "text"}),
				"platform": openrouter.StringProp("Target platform (defaults to Instagram)."),
			}, []string{"title", "date"}),
		),
		openrouter.NewFunctionTool(
			toolUpdateContent,
			"Change a post's title, idea, date, or type. Only these four are editable from the calendar — never caption, "+
				"script, hashtags or other inner details. Pass only the fields you are changing.",
			openrouter.ObjectSchema(map[string]any{
				"contentId": openrouter.StringProp("The id of the post to edit."),
				"title":     openrouter.StringProp("New title."),
				"idea":      openrouter.StringProp("New idea / brief."),
				"date":      openrouter.StringProp("New date as YYYY-MM-DD."),
				"type":      openrouter.EnumProp("New content format. 'text' is a plain-text post (no media).", []string{"reel", "post", "story", "carousel", "live", "text"}),
			}, []string{"contentId"}),
		),
		openrouter.NewFunctionTool(
			toolMoveContent,
			"Reschedule a post to a new date. Fails if the post is already scheduled or posted.",
			openrouter.ObjectSchema(map[string]any{
				"contentId": openrouter.StringProp("The id of the post to move."),
				"date":      openrouter.StringProp("The new date as YYYY-MM-DD."),
			}, []string{"contentId", "date"}),
		),
		openrouter.NewFunctionTool(
			toolRemoveContent,
			"Remove a post from the calendar. It is archived (soft-deleted) so it can be restored later.",
			openrouter.ObjectSchema(map[string]any{
				"contentId": openrouter.StringProp("The id of the post to remove."),
			}, []string{"contentId"}),
		),
	}
}

func isCalendarTool(name string) bool {
	switch name {
	case toolListCalendar, toolCreateContent, toolUpdateContent, toolMoveContent, toolRemoveContent:
		return true
	}
	return false
}

// dispatchCalendarTool runs a calendar server tool. Returns a JSON result string
// (fed back to the model), always false for the completion flag (the calendar
// has no terminal "ready" signal — changes reflect via the live snapshot), and
// any hard error. managerID stamps content created by the chat.
func dispatchCalendarTool(ctx context.Context, brandID, managerID, name, arguments string) (string, bool, error) {
	switch name {
	case toolListCalendar:
		return listCalendarTool(ctx, brandID, arguments)
	case toolCreateContent:
		return createContentTool(ctx, brandID, managerID, arguments)
	case toolUpdateContent:
		return updateContentTool(ctx, brandID, arguments)
	case toolMoveContent:
		return moveContentTool(ctx, brandID, arguments)
	case toolRemoveContent:
		return removeContentTool(ctx, brandID, arguments)
	default:
		return jsonResult(map[string]any{"ok": false, "error": "unknown calendar tool: " + name}), false, nil
	}
}

// contentMovable reports whether a content item can be rescheduled. Scheduled or
// already-posted items are locked (mirrors handleMoveItem on the calendar UI).
func contentLocked(status string) bool {
	return status == "scheduled" || status == "posted"
}

// ── create ───────────────────────────────────────────────────────────────────

type createContentArgs struct {
	Title    string `json:"title"`
	Idea     string `json:"idea"`
	Date     string `json:"date"`
	Type     string `json:"type"`
	Platform string `json:"platform"`
}

func createContentTool(ctx context.Context, brandID, managerID, arguments string) (string, bool, error) {
	var a createContentArgs
	if err := json.Unmarshal([]byte(arguments), &a); err != nil {
		return jsonResult(map[string]any{"ok": false, "error": "could not parse arguments"}), false, nil
	}
	title := strings.TrimSpace(a.Title)
	if title == "" {
		return jsonResult(map[string]any{"ok": false, "error": "title is required"}), false, nil
	}
	postingTs, err := parseStartDateMs(a.Date)
	if err != nil {
		return jsonResult(map[string]any{"ok": false, "error": "date must be YYYY-MM-DD"}), false, nil
	}
	format := strings.ToLower(strings.TrimSpace(a.Type))
	if !validContentFormats[format] {
		format = "post"
	}
	platform := strings.TrimSpace(a.Platform)
	if platform == "" {
		platform = "Instagram"
	}
	now := time.Now().UnixMilli()
	id, err := trendlymodels.CreateContent(ctx, brandID, map[string]any{
		"title":            title,
		"managerId":        managerID,
		"platform":         platform,
		"contentFormat":    format,
		"status":           "draft",
		"description":      strings.TrimSpace(a.Idea),
		"postingTimeStamp": postingTs,
		"isArchived":       false,
		"createdAt":        now,
		"updatedAt":        now,
	})
	if err != nil {
		return jsonResult(map[string]any{"ok": false, "error": "failed to create: " + err.Error()}), false, err
	}
	return jsonResult(map[string]any{"ok": true, "contentId": id, "date": msToDateStr(postingTs)}), false, nil
}

// ── update (title / idea / date / type only) ──────────────────────────────────

type updateContentArgs struct {
	ContentID string  `json:"contentId"`
	Title     *string `json:"title"`
	Idea      *string `json:"idea"`
	Date      *string `json:"date"`
	Type      *string `json:"type"`
}

func updateContentTool(ctx context.Context, brandID, arguments string) (string, bool, error) {
	var a updateContentArgs
	if err := json.Unmarshal([]byte(arguments), &a); err != nil {
		return jsonResult(map[string]any{"ok": false, "error": "could not parse arguments"}), false, nil
	}
	if strings.TrimSpace(a.ContentID) == "" {
		return jsonResult(map[string]any{"ok": false, "error": "contentId is required"}), false, nil
	}
	existing, err := trendlymodels.GetContent(brandID, a.ContentID)
	if err != nil {
		return jsonResult(map[string]any{"ok": false, "error": "content not found"}), false, nil
	}
	status := existing.Status

	var updates []firestore.Update
	var changed []string
	if a.Title != nil {
		if v := strings.TrimSpace(*a.Title); v != "" {
			updates = append(updates, firestore.Update{Path: "title", Value: v})
			changed = append(changed, "title")
		}
	}
	if a.Idea != nil {
		updates = append(updates, firestore.Update{Path: "description", Value: strings.TrimSpace(*a.Idea)})
		changed = append(changed, "idea")
	}
	if a.Type != nil {
		format := strings.ToLower(strings.TrimSpace(*a.Type))
		if !validContentFormats[format] {
			return jsonResult(map[string]any{"ok": false, "error": "type must be one of reel, post, story, carousel, live, text"}), false, nil
		}
		updates = append(updates, firestore.Update{Path: "contentFormat", Value: format})
		changed = append(changed, "type")
	}
	if a.Date != nil {
		if contentLocked(status) {
			return jsonResult(map[string]any{"ok": false, "reason": "this post is " + status + " — its date is locked. Unschedule it first."}), false, nil
		}
		ts, derr := parseStartDateMs(*a.Date)
		if derr != nil {
			return jsonResult(map[string]any{"ok": false, "error": "date must be YYYY-MM-DD"}), false, nil
		}
		updates = append(updates, firestore.Update{Path: "postingTimeStamp", Value: ts})
		changed = append(changed, "date")
	}

	if len(updates) == 0 {
		return jsonResult(map[string]any{"ok": false, "error": "nothing to update — pass at least one of title, idea, date, type"}), false, nil
	}
	if err := trendlymodels.UpdateContent(ctx, brandID, a.ContentID, updates); err != nil {
		return jsonResult(map[string]any{"ok": false, "error": "failed to update: " + err.Error()}), false, err
	}
	return jsonResult(map[string]any{"ok": true, "changed": changed}), false, nil
}

// ── move ──────────────────────────────────────────────────────────────────────

type moveContentArgs struct {
	ContentID string `json:"contentId"`
	Date      string `json:"date"`
}

func moveContentTool(ctx context.Context, brandID, arguments string) (string, bool, error) {
	var a moveContentArgs
	if err := json.Unmarshal([]byte(arguments), &a); err != nil {
		return jsonResult(map[string]any{"ok": false, "error": "could not parse arguments"}), false, nil
	}
	if strings.TrimSpace(a.ContentID) == "" {
		return jsonResult(map[string]any{"ok": false, "error": "contentId is required"}), false, nil
	}
	ts, err := parseStartDateMs(a.Date)
	if err != nil {
		return jsonResult(map[string]any{"ok": false, "error": "date must be YYYY-MM-DD"}), false, nil
	}
	existing, err := trendlymodels.GetContent(brandID, a.ContentID)
	if err != nil {
		return jsonResult(map[string]any{"ok": false, "error": "content not found"}), false, nil
	}
	status := existing.Status
	if contentLocked(status) {
		return jsonResult(map[string]any{"ok": false, "reason": "this post is " + status + " — it can't be moved. Unschedule it first."}), false, nil
	}
	if err := trendlymodels.UpdateContent(ctx, brandID, a.ContentID, []firestore.Update{
		{Path: "postingTimeStamp", Value: ts},
	}); err != nil {
		return jsonResult(map[string]any{"ok": false, "error": "failed to move: " + err.Error()}), false, err
	}
	return jsonResult(map[string]any{"ok": true, "date": msToDateStr(ts)}), false, nil
}

// ── remove (soft-delete) ──────────────────────────────────────────────────────

type removeContentArgs struct {
	ContentID string `json:"contentId"`
}

func removeContentTool(ctx context.Context, brandID, arguments string) (string, bool, error) {
	var a removeContentArgs
	if err := json.Unmarshal([]byte(arguments), &a); err != nil {
		return jsonResult(map[string]any{"ok": false, "error": "could not parse arguments"}), false, nil
	}
	if strings.TrimSpace(a.ContentID) == "" {
		return jsonResult(map[string]any{"ok": false, "error": "contentId is required"}), false, nil
	}
	if err := trendlymodels.UpdateContent(ctx, brandID, a.ContentID, []firestore.Update{
		{Path: "isArchived", Value: true},
	}); err != nil {
		return jsonResult(map[string]any{"ok": false, "error": "failed to remove: " + err.Error()}), false, err
	}
	return jsonResult(map[string]any{"ok": true}), false, nil
}

// ── list ──────────────────────────────────────────────────────────────────────

type listCalendarArgs struct {
	Month *float64 `json:"month"`
	Year  *float64 `json:"year"`
}

func listCalendarTool(ctx context.Context, brandID, arguments string) (string, bool, error) {
	var a listCalendarArgs
	if arguments != "" {
		if err := json.Unmarshal([]byte(arguments), &a); err != nil {
			return jsonResult(map[string]any{"ok": false, "error": "could not parse arguments"}), false, nil
		}
	}
	now := time.Now()
	month := int(now.Month())
	year := now.Year()
	if a.Month != nil && int(*a.Month) >= 1 && int(*a.Month) <= 12 {
		month = int(*a.Month)
	}
	if a.Year != nil && int(*a.Year) > 2000 {
		year = int(*a.Year)
	}
	start := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC).UnixMilli()
	end := time.Date(year, time.Month(month+1), 1, 0, 0, 0, 0, time.UTC).UnixMilli()

	contents, err := trendlymodels.ListContentInRange(ctx, brandID, start, end, false)
	if err != nil {
		return jsonResult(map[string]any{"ok": false, "error": "failed to list: " + err.Error()}), false, err
	}

	out := []map[string]any{}
	for _, ct := range contents {
		out = append(out, map[string]any{
			"id":     ct.ID,
			"title":  ct.Title,
			"date":   msToDateStr(ct.PostingTimeStamp),
			"type":   ct.ContentFormat,
			"status": ct.Status,
		})
	}
	return jsonResult(map[string]any{"ok": true, "month": fmt.Sprintf("%04d-%02d", year, month), "posts": out}), false, nil
}
