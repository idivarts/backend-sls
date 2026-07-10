package scenegraph

// BrandKit is the subset of a brand's identity used to seed and resolve scenes.
// It is assembled by the handler from the brand doc + branding tokens.
type BrandKit struct {
	HeadingFont string            // resolves brand.heading
	BodyFont    string            // resolves brand.body
	Colors      map[string]string // token -> hex, e.g. {"brand.navy":"#054463"}
	LogoURL     string            // resolves brand.logo
	Primary     string            // primary hex for backgrounds
}

// Resolver builds a Resolver from the brand kit, falling back to Trendly
// defaults for any unset token.
func (bk BrandKit) Resolver() Resolver {
	def := DefaultResolver()
	return Resolver{
		Color: func(t string) string {
			if bk.Colors != nil {
				if v, ok := bk.Colors[t]; ok {
					return v
				}
			}
			return def.Color(t)
		},
		Font: func(t string) string {
			switch t {
			case "brand.heading":
				if bk.HeadingFont != "" {
					return bk.HeadingFont
				}
			case "brand.body":
				if bk.BodyFont != "" {
					return bk.BodyFont
				}
			}
			return def.Font(t)
		},
		Media: func(src *Src, slot string) string {
			if slot == "brand.logo" && bk.LogoURL != "" {
				return bk.LogoURL
			}
			if src != nil && src.Kind == "brand" && src.Ref == "brand.logo" && bk.LogoURL != "" {
				return bk.LogoURL
			}
			return def.Media(src, slot)
		},
	}
}

// Preset sizes for the common formats.
var (
	SizePost  = Size{W: 1080, H: 1350} // 4:5 portrait post
	SizeReel  = Size{W: 1080, H: 1920} // 9:16 reel/story
	SizeVideo = Size{W: 1920, H: 1080} // 16:9 landscape
	SizeSquare = Size{W: 1080, H: 1080}
)

// SeedImageScene produces a valid starter image document seeded from the brand
// kit: a branded background, a logo top-left, a hero image slot in the centre
// and a headline + subhead at the bottom. This is the deterministic scaffold
// the AI fills/tweaks — it never designs pixels from scratch.
func SeedImageScene(size Size, headline, subhead string, bk BrandKit) *Document {
	primary := bk.Primary
	if primary == "" {
		primary = "brand.navy"
	}
	els := []Element{
		{ID: "el_logo", Type: ElLogo, Zone: ZoneTop, HAlign: "left", Slot: "brand.logo"},
		{ID: "el_hero", Type: ElImage, Zone: ZoneCenter, Src: &Src{Kind: "generated", Ref: ""},
			Transforms: &Transforms{Fit: "cover"}},
		{ID: "el_head", Type: ElText, Zone: ZoneBottom, HAlign: "center", Text: headline,
			Style: &Style{Font: "brand.heading", Color: "brand.white", Weight: "700", Align: "center", Scrim: true}},
	}
	if subhead != "" {
		els = append(els, Element{ID: "el_sub", Type: ElText, Zone: ZoneBottom, HAlign: "center", Text: subhead,
			Style: &Style{Font: "brand.body", Color: "brand.white", Weight: "500", Align: "center", Scale: 0.6}})
	}
	return &Document{
		Version:    SchemaVersion,
		Type:       DocImage,
		Size:       size,
		Background: Background{Kind: "gradient", Value: primary, Value2: "brand.accent", Angle: 135},
		Zones:      []Zone{ZoneTop, ZoneCenter, ZoneBottom, ZoneFull},
		Elements:   els,
	}
}

// MotionTemplates is the v1 library of video scene templates the AI selects,
// sequences, fills and tweaks (it never animates from scratch). Each maps to a
// known render recipe.
var MotionTemplates = []string{
	"hook_card",           // bold headline, fast pop-in
	"product_showcase",    // hero image, Ken Burns
	"testimonial_lower3",  // quote + lower-third
	"stat_reveal",         // number reveal
	"cta_outro",           // logo + CTA
}

// SeedVideoScene produces a valid starter video document: a hook card + a CTA
// outro reusing image scenes, a crossfade, and a music bed brief. Durations are
// preset; the AI tweaks via temporal ops.
func SeedVideoScene(size Size, headline, cta string, bk BrandKit) *Document {
	hook := SeedImageScene(size, headline, "", bk)
	hook.Type = DocImage
	outro := SeedImageScene(size, cta, "", bk)

	return &Document{
		Version:    SchemaVersion,
		Type:       DocVideo,
		Size:       size,
		Background: Background{Kind: "color", Value: "brand.navy"},
		Zones:      []Zone{ZoneTop, ZoneCenter, ZoneBottom, ZoneFull},
		Timeline: &Timeline{
			FPS: 30,
			Scenes: []Scene{
				{ID: "sc1", Duration: 3.0, Template: "hook_card", Scene: hook, Enter: "fadeIn", Exit: "slideUp"},
				{ID: "sc2", Duration: 2.5, Template: "cta_outro", Scene: outro, Enter: "fadeIn"},
			},
			Transitions: []Transition{{A: "sc1", B: "sc2", Type: "crossfade"}},
			Audio:       []AudioTrack{{Type: "music", Volume: 0.6, Duck: true}},
			Captions:    &Captions{Enabled: false, Source: "voiceover"},
		},
	}
}
