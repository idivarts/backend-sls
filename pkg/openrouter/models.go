package openrouter

type Tier string

const (
	TierStarter    Tier = "starter"
	TierGrowth     Tier = "growth"
	TierPro        Tier = "pro"
	TierEnterprise Tier = "enterprise"
)

type ModelInfo struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	Provider    string `json:"provider"`
	MinTier     Tier   `json:"minTier"`
	Multimodal  bool   `json:"multimodal,omitempty"`
}

var Models = []ModelInfo{
	{ID: "openai/gpt-4o", DisplayName: "GPT-4o", Provider: "OpenAI", MinTier: TierStarter, Multimodal: true},
	{ID: "anthropic/claude-sonnet-4-5", DisplayName: "Claude Sonnet 4.5", Provider: "Anthropic", MinTier: TierStarter},
	{ID: "anthropic/claude-opus-4", DisplayName: "Claude Opus 4", Provider: "Anthropic", MinTier: TierGrowth},
	{ID: "google/gemini-2.5-pro", DisplayName: "Gemini 2.5 Pro", Provider: "Google", MinTier: TierPro, Multimodal: true},
	{ID: "openai/o3", DisplayName: "o3", Provider: "OpenAI", MinTier: TierPro},
}

var tierRank = map[Tier]int{
	TierStarter:    0,
	TierGrowth:     1,
	TierPro:        2,
	TierEnterprise: 3,
}

func IsUnlockedFor(model ModelInfo, brandTier Tier) bool {
	bt, ok := tierRank[brandTier]
	if !ok {
		bt = 0
	}
	mt, ok := tierRank[model.MinTier]
	if !ok {
		mt = 0
	}
	return bt >= mt
}

func IsKnownModel(id string) bool {
	for _, m := range Models {
		if m.ID == id {
			return true
		}
	}
	return false
}

func ModelByID(id string) (ModelInfo, bool) {
	for _, m := range Models {
		if m.ID == id {
			return m, true
		}
	}
	return ModelInfo{}, false
}

func TierFromPlanKey(planKey string) Tier {
	switch planKey {
	case "growth":
		return TierGrowth
	case "pro":
		return TierPro
	case "enterprise":
		return TierEnterprise
	default:
		return TierStarter
	}
}
