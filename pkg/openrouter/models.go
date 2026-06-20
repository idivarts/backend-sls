package openrouter

import (
	"context"
	"log"
	"sort"
	"sync"
	"time"

	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
)

// ---------------------------------------------------------------------------
// Plans
// ---------------------------------------------------------------------------

// Plan is a subscription tier. Ordering: free < pro < team < agency.
type Plan string

const (
	PlanFree   Plan = "free"
	PlanPro    Plan = "pro"
	PlanTeam   Plan = "team"
	PlanAgency Plan = "agency"
)

var planRank = map[Plan]int{
	PlanFree:   0,
	PlanPro:    1,
	PlanTeam:   2,
	PlanAgency: 3,
}

// PlanFromKey maps an org Billing.PlanKey string to a Plan. It understands the
// current USD plan keys (free/pro/team/agency) and the legacy India keys
// (starter/growth/enterprise). Unknown/empty keys fall back to free.
func PlanFromKey(planKey string) Plan {
	switch planKey {
	case "free":
		return PlanFree
	case "pro":
		return PlanPro
	case "team":
		return PlanTeam
	case "agency":
		return PlanAgency
	// legacy India tiers
	case "starter":
		return PlanFree
	case "growth":
		return PlanTeam
	case "enterprise":
		return PlanAgency
	default:
		return PlanFree
	}
}

// ---------------------------------------------------------------------------
// Registry types
// ---------------------------------------------------------------------------

// ModelInfo describes a single selectable/usable model. JSON tags mirror the
// camelCase field names stored in Firestore so the frontend reads them directly.
type ModelInfo struct {
	ID          string `json:"id" firestore:"id"`
	DisplayName string `json:"displayName" firestore:"displayName"`
	Provider    string `json:"provider" firestore:"provider"`
	MinPlan     Plan   `json:"minPlan" firestore:"minPlan"`
	Vision      bool   `json:"vision,omitempty" firestore:"vision"`
	ImageGen    bool   `json:"imageGen,omitempty" firestore:"imageGen"`
}

// TaskConfig is the ordered list of allowed models for a task (best -> fallbacks).
type TaskConfig struct {
	Allowed []string `json:"allowed" firestore:"allowed"`
}

// Registry is the full AI config: model catalog + per-task allowed lists.
type Registry struct {
	Models []ModelInfo
	Tasks  map[TaskType]TaskConfig
}

func (r Registry) modelByID(id string) (ModelInfo, bool) {
	for _, m := range r.Models {
		if m.ID == id {
			return m, true
		}
	}
	return ModelInfo{}, false
}

// Unlocked reports whether a model is available to a plan.
func Unlocked(m ModelInfo, plan Plan) bool {
	return planRank[plan] >= planRank[m.MinPlan]
}

// ---------------------------------------------------------------------------
// Default (fallback) registry
// ---------------------------------------------------------------------------

// defaultRegistry is the built-in fallback used when Firestore is unavailable or
// not yet seeded. Keep it in sync with scripts/seed_ai_config.
func defaultRegistry() Registry {
	return Registry{
		Models: []ModelInfo{
			{ID: "google/gemini-3.5-flash", DisplayName: "Gemini 3.5 Flash", Provider: "Google", MinPlan: PlanFree, Vision: true},
			{ID: "anthropic/claude-sonnet-4.6", DisplayName: "Claude Sonnet 4.6", Provider: "Anthropic", MinPlan: PlanFree},
			{ID: "openai/gpt-5.4", DisplayName: "GPT-5.4", Provider: "OpenAI", MinPlan: PlanPro, Vision: true},
			{ID: "anthropic/claude-opus-4.8", DisplayName: "Claude Opus 4.8", Provider: "Anthropic", MinPlan: PlanPro, Vision: true},
			{ID: "google/gemini-3.1-pro-preview", DisplayName: "Gemini 3 Pro", Provider: "Google", MinPlan: PlanTeam, Vision: true},
			{ID: "openai/gpt-5.5", DisplayName: "GPT-5.5", Provider: "OpenAI", MinPlan: PlanTeam, Vision: true},
			{ID: "google/gemini-3.1-flash-image", DisplayName: "Gemini 3.1 Flash Image", Provider: "Google", MinPlan: PlanPro, Vision: true, ImageGen: true},
			{ID: "google/gemini-3-pro-image", DisplayName: "Gemini 3 Pro Image", Provider: "Google", MinPlan: PlanTeam, Vision: true, ImageGen: true},
		},
		Tasks: map[TaskType]TaskConfig{
			TaskChat:       {Allowed: []string{"google/gemini-3.5-flash", "anthropic/claude-sonnet-4.6", "openai/gpt-5.4", "anthropic/claude-opus-4.8", "openai/gpt-5.5", "google/gemini-3.1-pro-preview"}},
			TaskQuickEdit:  {Allowed: []string{"google/gemini-3.5-flash", "anthropic/claude-sonnet-4.6"}},
			TaskCaption:    {Allowed: []string{"google/gemini-3.5-flash", "anthropic/claude-sonnet-4.6", "openai/gpt-5.4"}},
			TaskHashtag:    {Allowed: []string{"google/gemini-3.5-flash", "anthropic/claude-sonnet-4.6"}},
			TaskStrategy:   {Allowed: []string{"anthropic/claude-opus-4.8", "openai/gpt-5.5", "anthropic/claude-sonnet-4.6", "google/gemini-3.5-flash"}},
			TaskScript:     {Allowed: []string{"anthropic/claude-opus-4.8", "openai/gpt-5.5", "openai/gpt-5.4"}},
			TaskMultimodal: {Allowed: []string{"google/gemini-3.1-pro-preview", "openai/gpt-5.4", "google/gemini-3.5-flash"}},
			TaskReasoning:  {Allowed: []string{"openai/gpt-5.5", "anthropic/claude-opus-4.8", "openai/gpt-5.4"}},
			TaskImage:      {Allowed: []string{"google/gemini-3-pro-image", "google/gemini-3.1-flash-image"}},
		},
	}
}

// ---------------------------------------------------------------------------
// Cached Firestore-backed registry
// ---------------------------------------------------------------------------

const (
	configCollection = "ai_config"
	modelsDocID      = "models"
	tasksDocID       = "tasks"
	registryTTL      = 5 * time.Minute
)

var (
	regMu     sync.RWMutex
	cachedReg *Registry
	cachedAt  time.Time
)

// modelsDoc / tasksDoc mirror the Firestore document shapes.
type modelsDoc struct {
	Models []modelDocItem `firestore:"models"`
}

type modelDocItem struct {
	ID          string `firestore:"id"`
	DisplayName string `firestore:"displayName"`
	Provider    string `firestore:"provider"`
	MinPlan     string `firestore:"minPlan"`
	Vision      bool   `firestore:"vision"`
	ImageGen    bool   `firestore:"imageGen"`
	Enabled     *bool  `firestore:"enabled"`
	Order       int    `firestore:"order"`
}

type tasksDoc struct {
	Tasks map[string]struct {
		Allowed []string `firestore:"allowed"`
	} `firestore:"tasks"`
}

// EnsureRegistry refreshes the in-memory registry from Firestore if the cache is
// stale. Safe to call on every request; it is a no-op while the cache is fresh.
// On Firestore failure it keeps the last-known registry (or the built-in default).
func EnsureRegistry(ctx context.Context) {
	regMu.RLock()
	fresh := cachedReg != nil && time.Since(cachedAt) < registryTTL
	regMu.RUnlock()
	if fresh {
		return
	}

	reg, err := loadRegistryFromFirestore(ctx)
	if err != nil || reg == nil || len(reg.Models) == 0 {
		if err != nil {
			log.Printf("[openrouter] ai_config load failed, using cached/default: %v", err)
		}
		regMu.Lock()
		if cachedReg == nil {
			d := defaultRegistry()
			cachedReg = &d
			cachedAt = time.Now()
		}
		regMu.Unlock()
		return
	}

	regMu.Lock()
	cachedReg = reg
	cachedAt = time.Now()
	regMu.Unlock()
}

// currentRegistry returns the cached registry, or the built-in default if the
// cache has never been populated.
func currentRegistry() Registry {
	regMu.RLock()
	defer regMu.RUnlock()
	if cachedReg != nil {
		return *cachedReg
	}
	return defaultRegistry()
}

func loadRegistryFromFirestore(ctx context.Context) (*Registry, error) {
	if firestoredb.Client == nil {
		return nil, nil
	}

	mSnap, err := firestoredb.Client.Collection(configCollection).Doc(modelsDocID).Get(ctx)
	if err != nil {
		return nil, err
	}
	var md modelsDoc
	if err := mSnap.DataTo(&md); err != nil {
		return nil, err
	}

	tSnap, err := firestoredb.Client.Collection(configCollection).Doc(tasksDocID).Get(ctx)
	if err != nil {
		return nil, err
	}
	var td tasksDoc
	if err := tSnap.DataTo(&td); err != nil {
		return nil, err
	}

	reg := &Registry{
		Models: make([]ModelInfo, 0, len(md.Models)),
		Tasks:  make(map[TaskType]TaskConfig, len(td.Tasks)),
	}

	items := make([]modelDocItem, 0, len(md.Models))
	for _, it := range md.Models {
		if it.Enabled != nil && !*it.Enabled {
			continue
		}
		items = append(items, it)
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].Order < items[j].Order })
	for _, it := range items {
		reg.Models = append(reg.Models, ModelInfo{
			ID:          it.ID,
			DisplayName: it.DisplayName,
			Provider:    it.Provider,
			MinPlan:     Plan(it.MinPlan),
			Vision:      it.Vision,
			ImageGen:    it.ImageGen,
		})
	}

	for k, v := range td.Tasks {
		reg.Tasks[TaskType(k)] = TaskConfig{Allowed: v.Allowed}
	}

	return reg, nil
}

// ---------------------------------------------------------------------------
// Public accessors / resolution
// ---------------------------------------------------------------------------

// ListModels returns the current model catalog (cached).
func ListModels() []ModelInfo {
	return currentRegistry().Models
}

// AllowedModels returns the ordered allowed model ids for a task.
func AllowedModels(task TaskType) []string {
	if tc, ok := currentRegistry().Tasks[task]; ok {
		return tc.Allowed
	}
	return nil
}

// Resolve picks the model to use for a task given the caller's plan and an
// optional explicitly-requested model. It returns the resolved model id, or
// locked=true when the plan unlocks no model allowed for this task (the caller
// must then surface an upgrade prompt rather than running anything).
//
//   - A requested model is honoured only if it is allowed for the task AND
//     unlocked for the plan.
//   - Otherwise the first allowed model the plan unlocks (best -> fallback) wins.
func Resolve(task TaskType, plan Plan, requested string) (model string, locked bool) {
	reg := currentRegistry()
	tc, ok := reg.Tasks[task]
	if !ok || len(tc.Allowed) == 0 {
		return "", true
	}

	if requested != "" {
		for _, id := range tc.Allowed {
			if id == requested {
				if m, ok := reg.modelByID(id); ok && Unlocked(m, plan) {
					return id, false
				}
			}
		}
	}

	for _, id := range tc.Allowed {
		if m, ok := reg.modelByID(id); ok && Unlocked(m, plan) {
			return id, false
		}
	}
	return "", true
}
