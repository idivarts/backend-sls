package trendlymodels

import (
	"context"
	"fmt"

	"cloud.google.com/go/firestore"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// DesignSystemDocID is the fixed document id for a brand's single Design System.
// There is exactly ONE Design System per brand, so the document always lives at
// brands/{brandId}/designSystem/main — the fixed id is what enforces
// one-per-brand (no way to create a second).
const DesignSystemDocID = "main"

// BrandDesignSystem is a brand's single, declared design/brand standard. It is
// read into the AI conversation (as a compact system-prompt block + brand-kit
// CSS tokens for the design tools + imagery guidance for image generation) so
// everything the AI produces for the brand — captions, scripts, images, reels,
// designs — follows one consistent standard.
//
// Stored at brands/{brandId}/designSystem/main (see DesignSystemDocID). The
// document is client-readable/writable by brand members (the editor UI writes it
// directly, like content/variations); backend reads it here through this model.
//
// Every section is optional — the user fills the Design System progressively,
// and only the populated parts reach the AI.
type BrandDesignSystem struct {
	ID string `json:"id,omitempty" firestore:"-"`

	Identity          *DSIdentity                   `json:"identity,omitempty" firestore:"identity,omitempty"`
	Palette           []DSColor                     `json:"palette,omitempty" firestore:"palette,omitempty"`
	Fonts             []DSFont                      `json:"fonts,omitempty" firestore:"fonts,omitempty"`
	Logos             []DSLogo                      `json:"logos,omitempty" firestore:"logos,omitempty"`
	Assets            []DSAsset                     `json:"assets,omitempty" firestore:"assets,omitempty"`
	LogoGuidelines    string                        `json:"logoGuidelines,omitempty" firestore:"logoGuidelines,omitempty"`
	Imagery           *DSImagery                    `json:"imagery,omitempty" firestore:"imagery,omitempty"`
	Voice             *DSVoice                      `json:"voice,omitempty" firestore:"voice,omitempty"`
	Rules             *DSContentRules               `json:"rules,omitempty" firestore:"rules,omitempty"`
	PlatformOverrides map[string]DSPlatformOverride `json:"platformOverrides,omitempty" firestore:"platformOverrides,omitempty"`

	UpdatedAt int64  `json:"updatedAt,omitempty" firestore:"updatedAt,omitempty"`
	UpdatedBy string `json:"updatedBy,omitempty" firestore:"updatedBy,omitempty"`
}

// DSIdentity — brand positioning. Delivered to the AI as system-prompt prose.
type DSIdentity struct {
	Tagline     string   `json:"tagline,omitempty" firestore:"tagline,omitempty"`
	Mission     string   `json:"mission,omitempty" firestore:"mission,omitempty"`
	Audience    string   `json:"audience,omitempty" firestore:"audience,omitempty"`
	Personality []string `json:"personality,omitempty" firestore:"personality,omitempty"`
	ValueProps  []string `json:"valueProps,omitempty" firestore:"valueProps,omitempty"`
	Competitors []string `json:"competitors,omitempty" firestore:"competitors,omitempty"`
}

// DSColor — one brand color as an entity (role + representation + guidance),
// never a bare hex. Delivered to the design/image tools as CSS tokens.
type DSColor struct {
	Name      string `json:"name,omitempty" firestore:"name,omitempty"`
	Role      string `json:"role,omitempty" firestore:"role,omitempty"` // primary|secondary|accent|neutral|background|text
	Hex       string `json:"hex,omitempty" firestore:"hex,omitempty"`
	UsageNote string `json:"usageNote,omitempty" firestore:"usageNote,omitempty"`
}

// DSFont — a typeface with its role + provenance. Delivered as design tokens.
type DSFont struct {
	Role     string `json:"role,omitempty" firestore:"role,omitempty"` // heading|subheading|body|caption|quote
	Family   string `json:"family,omitempty" firestore:"family,omitempty"`
	Weight   string `json:"weight,omitempty" firestore:"weight,omitempty"`
	Source   string `json:"source,omitempty" firestore:"source,omitempty"` // google|adobe|custom
	URL      string `json:"url,omitempty" firestore:"url,omitempty"`
	Fallback string `json:"fallback,omitempty" firestore:"fallback,omitempty"`
}

// DSLogo — a logo variant + its usage note. Uploaded via the app's S3 flow.
type DSLogo struct {
	Variant        string `json:"variant,omitempty" firestore:"variant,omitempty"` // primary|stacked|icon|white|mono
	URL            string `json:"url,omitempty" firestore:"url,omitempty"`
	ClearSpaceNote string `json:"clearSpaceNote,omitempty" firestore:"clearSpaceNote,omitempty"`
}

// DSAsset — a supporting brand asset (pattern, icon, sticker, watermark).
type DSAsset struct {
	Type string `json:"type,omitempty" firestore:"type,omitempty"`
	URL  string `json:"url,omitempty" firestore:"url,omitempty"`
	Name string `json:"name,omitempty" firestore:"name,omitempty"`
}

// DSImagery — the brand's photography/illustration style. The mood keywords +
// avoid list are injected into the generate_image prompt.
type DSImagery struct {
	MoodKeywords    []string `json:"moodKeywords,omitempty" firestore:"moodKeywords,omitempty"`
	ColorTreatment  string   `json:"colorTreatment,omitempty" firestore:"colorTreatment,omitempty"`
	Composition     string   `json:"composition,omitempty" firestore:"composition,omitempty"`
	SubjectMatter   string   `json:"subjectMatter,omitempty" firestore:"subjectMatter,omitempty"`
	StyleType       string   `json:"styleType,omitempty" firestore:"styleType,omitempty"` // photo|illustration|3d|mixed
	Avoid           []string `json:"avoid,omitempty" firestore:"avoid,omitempty"`
	ReferenceImages []string `json:"referenceImages,omitempty" firestore:"referenceImages,omitempty"`
}

// DSVoice — brand voice & tone. Supersedes the legacy Brand.AIVoice string; this
// section is now the single source of truth for how the brand sounds.
type DSVoice struct {
	Adjectives    []string       `json:"adjectives,omitempty" firestore:"adjectives,omitempty"`
	Tone          *DSToneSliders `json:"tone,omitempty" firestore:"tone,omitempty"`
	POV           string         `json:"pov,omitempty" firestore:"pov,omitempty"`                 // we|you|brand
	EmojiPolicy   string         `json:"emojiPolicy,omitempty" firestore:"emojiPolicy,omitempty"` // none|minimal|liberal
	ReadingLevel  string         `json:"readingLevel,omitempty" firestore:"readingLevel,omitempty"`
	SamplePhrases []string       `json:"samplePhrases,omitempty" firestore:"samplePhrases,omitempty"`
	Guidelines    string         `json:"guidelines,omitempty" firestore:"guidelines,omitempty"`
}

// DSToneSliders — 0..100 tone dials (50 = neutral). Rendered into words for the
// prompt (e.g. "quite casual", "very playful") only when meaningfully off-center.
type DSToneSliders struct {
	Formality   *int `json:"formality,omitempty" firestore:"formality,omitempty"`     // 0 casual … 100 formal
	Playfulness *int `json:"playfulness,omitempty" firestore:"playfulness,omitempty"` // 0 serious … 100 playful
	Warmth      *int `json:"warmth,omitempty" firestore:"warmth,omitempty"`           // 0 neutral … 100 warm
	Boldness    *int `json:"boldness,omitempty" firestore:"boldness,omitempty"`       // 0 understated … 100 bold
	Luxury      *int `json:"luxury,omitempty" firestore:"luxury,omitempty"`           // 0 accessible … 100 premium
}

// DSContentRules — the guardrails the AI must respect on every caption/script.
type DSContentRules struct {
	PreferredTerms   []string        `json:"preferredTerms,omitempty" firestore:"preferredTerms,omitempty"`
	BannedWords      []string        `json:"bannedWords,omitempty" firestore:"bannedWords,omitempty"`
	ApprovedClaims   []string        `json:"approvedClaims,omitempty" firestore:"approvedClaims,omitempty"`
	Disclaimers      []string        `json:"disclaimers,omitempty" firestore:"disclaimers,omitempty"`
	RequiredMentions []string        `json:"requiredMentions,omitempty" firestore:"requiredMentions,omitempty"`
	Hashtag          *DSHashtagRules `json:"hashtag,omitempty" firestore:"hashtag,omitempty"`
	CTAStyle         string          `json:"ctaStyle,omitempty" firestore:"ctaStyle,omitempty"`
	Grammar          string          `json:"grammar,omitempty" firestore:"grammar,omitempty"`
	Dos              []string        `json:"dos,omitempty" firestore:"dos,omitempty"`
	Donts            []string        `json:"donts,omitempty" firestore:"donts,omitempty"`
}

// DSHashtagRules — branded/banned hashtags and a soft cap.
type DSHashtagRules struct {
	Branded  []string `json:"branded,omitempty" firestore:"branded,omitempty"`
	Banned   []string `json:"banned,omitempty" firestore:"banned,omitempty"`
	MaxCount *int     `json:"maxCount,omitempty" firestore:"maxCount,omitempty"`
}

// DSPlatformOverride — per-network diffs on top of the shared design system
// (mirrors the ContentVariation "override, not snapshot" idea). Keyed by platform
// on BrandDesignSystem.PlatformOverrides.
type DSPlatformOverride struct {
	ToneOverride    string   `json:"toneOverride,omitempty" firestore:"toneOverride,omitempty"`
	CaptionLength   string   `json:"captionLength,omitempty" firestore:"captionLength,omitempty"` // short|medium|long
	HashtagStrategy string   `json:"hashtagStrategy,omitempty" firestore:"hashtagStrategy,omitempty"`
	ContentPillars  []string `json:"contentPillars,omitempty" firestore:"contentPillars,omitempty"`
	Notes           string   `json:"notes,omitempty" firestore:"notes,omitempty"`
}

func designSystemDoc(brandID string) *firestore.DocumentRef {
	return firestoredb.Client.
		Collection(fmt.Sprintf("brands/%s/designSystem", brandID)).
		Doc(DesignSystemDocID)
}

// GetDesignSystem reads a brand's Design System, or (nil, nil) when the brand has
// not created one yet (a missing Design System is not an error — the AI simply
// runs without design-system context, exactly as it does today).
func GetDesignSystem(ctx context.Context, brandID string) (*BrandDesignSystem, error) {
	if brandID == "" {
		return nil, nil
	}
	doc, err := designSystemDoc(brandID).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, err
	}
	if !doc.Exists() {
		return nil, nil
	}
	var ds BrandDesignSystem
	if err := doc.DataTo(&ds); err != nil {
		return nil, err
	}
	ds.ID = doc.Ref.ID
	return &ds, nil
}

// UpsertDesignSystem writes the whole Design System document (create-or-replace).
// Used by backend callers (e.g. the AI autofill tool) that produce a full draft;
// the editor UI writes individual fields directly through the client Firestore
// path, like content/variations.
func UpsertDesignSystem(ctx context.Context, brandID string, ds *BrandDesignSystem) error {
	if brandID == "" || ds == nil {
		return fmt.Errorf("brandID and design system are required")
	}
	_, err := designSystemDoc(brandID).Set(ctx, ds)
	return err
}

// UpdateDesignSystemFields applies a partial update to a brand's Design System
// document. Callers build the []firestore.Update (supports nested FieldPath
// updates like "voice.pov"); the Firestore call lives here in the model.
func UpdateDesignSystemFields(ctx context.Context, brandID string, updates []firestore.Update) error {
	_, err := designSystemDoc(brandID).Update(ctx, updates)
	return err
}
