package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"cloud.google.com/go/firestore"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
	"github.com/idivarts/backend-sls/pkg/openrouter"
)

// moduleOnboarding is the AI module that drives conversational brand setup. It
// runs against a draft brand doc (onboardingComplete=false) and uses the server
// tools below to persist captured fields as the conversation progresses.
const moduleOnboarding = "onboarding"

const (
	toolSetBrandFields    = "set_brand_fields"
	toolCompleteOnboarding = "complete_onboarding"
)

// Validation mirrors the regexes used by the brand form (BrandDetailsForm /
// onboarding-your-brand) so the chat and the fallback form agree on what is
// valid. RE2 has no backreferences but these patterns don't need any.
var (
	phoneRe   = regexp.MustCompile(`^\+?[1-9]\d{0,2}[\s-]?(\(?\d{1,4}\)?[\s-]?)?\d{1,4}([\s-]?\d{1,4}){1,3}$`)
	websiteRe = regexp.MustCompile(`^(https?://)?([a-zA-Z0-9-]+\.)+[a-zA-Z]{2,}(/\S*)?$`)
	validAges = map[string]bool{"JUST_STARTING": true, "LT_1": true, "LT_5": true, "GT_5": true}
)

// onboardingServerTools are executed on the backend (vs. client answer-control
// tools) and only attached when the conversation's module is onboarding.
func onboardingServerTools() []openrouter.Tool {
	return []openrouter.Tool{
		openrouter.NewFunctionTool(
			toolSetBrandFields,
			"Save one or more brand fields collected from the user onto the brand being "+
				"set up. Call this as soon as you learn a value — pass only the fields you "+
				"just learned. Values are validated; if a value is rejected, ask the user again.",
			openrouter.ObjectSchema(map[string]any{
				"name":        openrouter.StringProp("The brand name."),
				"about":       openrouter.StringProp("A short description of the brand."),
				"phone":       openrouter.StringProp("Contact phone number (with country code if given)."),
				"website":     openrouter.StringProp("Brand website URL."),
				"industries": map[string]any{
					"type":        "array",
					"description": "Industries / categories the brand operates in.",
					"items":       map[string]any{"type": "string"},
				},
				"age": openrouter.EnumProp(
					"How established the brand is.",
					[]string{"JUST_STARTING", "LT_1", "LT_5", "GT_5"},
				),
				"surveySource":             openrouter.StringProp("Where the user heard about us (survey)."),
				"surveyPurpose":            openrouter.StringProp("What the user will use the platform for (survey)."),
				"surveyCollaborationValue": openrouter.StringProp("Expected monthly content volume (survey)."),
				"promotionType": map[string]any{
					"type":        "array",
					"description": "Preferred promotion types.",
					"items":       map[string]any{"type": "string"},
				},
				"influencerCategories": map[string]any{
					"type":        "array",
					"description": "Preferred influencer categories.",
					"items":       map[string]any{"type": "string"},
				},
			}, []string{}),
		),
		openrouter.NewFunctionTool(
			toolCompleteOnboarding,
			"Call this only once all required fields are collected (brand name, phone, "+
				"at least one industry, and brand age). It checks the saved brand and, if "+
				"anything required is still missing, tells you what to ask for next.",
			openrouter.ObjectSchema(map[string]any{}, []string{}),
		),
	}
}

// dispatchServerTool runs a server tool. It returns a JSON result string (fed
// back to the model), a completion flag (onboarding complete / strategy doc
// ready — chat.go turns this into the matching WS signal), and any hard error.
// contextID is the conversation's ContextID (e.g. the strategyId) and managerID
// is the conversation's UserID — both passed through for tools that act on a
// specific document or stamp authorship. Validation failures are returned as
// result content (not errors) so the model can recover by re-asking.
func dispatchServerTool(ctx context.Context, brandID, managerID, contextID, name, arguments string) (string, bool, error) {
	switch name {
	case toolSetBrandFields:
		return setBrandFields(ctx, brandID, arguments)
	case toolCompleteOnboarding:
		return completeOnboarding(ctx, brandID)
	case toolSetStrategyBrief, toolGenerateStrategyDoc, toolApplyStrategyEdit:
		return dispatchStrategyTool(ctx, brandID, contextID, name, arguments)
	case toolListCalendar, toolCreateContent, toolUpdateContent, toolMoveContent, toolRemoveContent:
		return dispatchCalendarTool(ctx, brandID, managerID, name, arguments)
	default:
		return jsonResult(map[string]any{"ok": false, "error": "unknown tool: " + name}), false, nil
	}
}

type setBrandFieldsArgs struct {
	Name                 *string  `json:"name"`
	About                *string  `json:"about"`
	Phone                *string  `json:"phone"`
	Website              *string  `json:"website"`
	Industries           []string `json:"industries"`
	Age                  *string  `json:"age"`
	SurveySource         *string  `json:"surveySource"`
	SurveyPurpose        *string  `json:"surveyPurpose"`
	SurveyCollabValue    *string  `json:"surveyCollaborationValue"`
	PromotionType        []string `json:"promotionType"`
	InfluencerCategories []string `json:"influencerCategories"`
}

func setBrandFields(ctx context.Context, brandID, arguments string) (string, bool, error) {
	var a setBrandFieldsArgs
	if arguments != "" {
		if err := json.Unmarshal([]byte(arguments), &a); err != nil {
			return jsonResult(map[string]any{"ok": false, "error": "could not parse arguments"}), false, nil
		}
	}

	var updates []firestore.Update
	var saved []string
	var rejected []string

	if a.Name != nil {
		if v := strings.TrimSpace(*a.Name); v != "" {
			updates = append(updates, firestore.Update{Path: "name", Value: v})
			saved = append(saved, "name")
		}
	}
	if a.About != nil {
		updates = append(updates, firestore.Update{Path: "profile.about", Value: strings.TrimSpace(*a.About)})
		saved = append(saved, "about")
	}
	if a.Phone != nil {
		v := strings.TrimSpace(*a.Phone)
		if phoneRe.MatchString(v) {
			updates = append(updates, firestore.Update{Path: "profile.phone", Value: v})
			saved = append(saved, "phone")
		} else {
			rejected = append(rejected, "phone (invalid format)")
		}
	}
	if a.Website != nil {
		v := strings.TrimSpace(*a.Website)
		if v == "" || websiteRe.MatchString(v) {
			updates = append(updates, firestore.Update{Path: "profile.website", Value: v})
			saved = append(saved, "website")
		} else {
			rejected = append(rejected, "website (invalid format)")
		}
	}
	if a.Industries != nil {
		ind := cleanStrings(a.Industries)
		if len(ind) > 0 {
			updates = append(updates, firestore.Update{Path: "profile.industries", Value: ind})
			saved = append(saved, "industries")
		}
	}
	if a.Age != nil {
		v := strings.TrimSpace(*a.Age)
		if validAges[v] {
			updates = append(updates, firestore.Update{Path: "age", Value: v})
			saved = append(saved, "age")
		} else {
			rejected = append(rejected, "age (must be JUST_STARTING, LT_1, LT_5 or GT_5)")
		}
	}
	if a.SurveySource != nil {
		updates = append(updates, firestore.Update{Path: "survey.source", Value: strings.TrimSpace(*a.SurveySource)})
		saved = append(saved, "surveySource")
	}
	if a.SurveyPurpose != nil {
		updates = append(updates, firestore.Update{Path: "survey.purpose", Value: strings.TrimSpace(*a.SurveyPurpose)})
		saved = append(saved, "surveyPurpose")
	}
	if a.SurveyCollabValue != nil {
		updates = append(updates, firestore.Update{Path: "survey.collaborationValue", Value: strings.TrimSpace(*a.SurveyCollabValue)})
		saved = append(saved, "surveyCollaborationValue")
	}
	if a.PromotionType != nil {
		if v := cleanStrings(a.PromotionType); len(v) > 0 {
			updates = append(updates, firestore.Update{Path: "preferences.promotionType", Value: v})
			saved = append(saved, "promotionType")
		}
	}
	if a.InfluencerCategories != nil {
		if v := cleanStrings(a.InfluencerCategories); len(v) > 0 {
			updates = append(updates, firestore.Update{Path: "preferences.influencerCategories", Value: v})
			saved = append(saved, "influencerCategories")
		}
	}

	if len(updates) > 0 {
		if _, err := firestoredb.Client.Collection("brands").Doc(brandID).Update(ctx, updates); err != nil {
			return jsonResult(map[string]any{"ok": false, "error": "failed to save: " + err.Error()}), false, err
		}
	}

	return jsonResult(map[string]any{
		"ok":       len(rejected) == 0,
		"saved":    saved,
		"rejected": rejected,
	}), false, nil
}

func completeOnboarding(ctx context.Context, brandID string) (string, bool, error) {
	snap, err := firestoredb.Client.Collection("brands").Doc(brandID).Get(ctx)
	if err != nil {
		return jsonResult(map[string]any{"ok": false, "error": "brand not found"}), false, err
	}
	data := snap.Data()

	var missing []string
	if strings.TrimSpace(getString(data, "name")) == "" {
		missing = append(missing, "brand name")
	}
	profile, _ := data["profile"].(map[string]any)
	if profile == nil || strings.TrimSpace(getString(profile, "phone")) == "" {
		missing = append(missing, "phone number")
	}
	if profile == nil || len(toSlice(profile["industries"])) == 0 {
		missing = append(missing, "at least one industry")
	}
	if !validAges[strings.TrimSpace(getString(data, "age"))] {
		missing = append(missing, "brand age")
	}

	if len(missing) > 0 {
		return jsonResult(map[string]any{
			"ok":      false,
			"missing": missing,
			"message": "Still need: " + strings.Join(missing, ", "),
		}), false, nil
	}

	return jsonResult(map[string]any{
		"ok":      true,
		"message": "All required fields collected. Onboarding can be finalized.",
	}), true, nil
}

// ── small helpers ───────────────────────────────────────────────────────────

func jsonResult(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf(`{"ok":false,"error":%q}`, err.Error())
	}
	return string(b)
}

func cleanStrings(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func getString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	s, _ := m[key].(string)
	return s
}

func toSlice(v any) []any {
	s, _ := v.([]any)
	return s
}
