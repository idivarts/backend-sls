package openrouter

import "testing"

// setRegistry installs a fake live registry for the duration of a test, the way
// EnsureRegistry would after a Firestore load.
func setRegistry(t *testing.T, r Registry) {
	t.Helper()
	regMu.Lock()
	prev := cachedReg
	cachedReg = &r
	regMu.Unlock()
	t.Cleanup(func() {
		regMu.Lock()
		cachedReg = prev
		regMu.Unlock()
	})
}

// A live config as it actually stood in Firestore when the design lane shipped:
// the models are there, but ai_config/tasks predates the "design" task.
func staleLiveRegistry() Registry {
	return Registry{
		Models: []ModelInfo{
			{ID: "google/gemini-3.5-flash", MinPlan: PlanFree, Vision: true},
			{ID: "anthropic/claude-sonnet-4.6", MinPlan: PlanFree},
			{ID: "openai/gpt-5.4", MinPlan: PlanPro, Vision: true},
			{ID: "anthropic/claude-opus-4.8", MinPlan: PlanPro, Vision: true},
			{ID: "google/gemini-3-pro-preview", MinPlan: PlanTeam, Vision: true},
			{ID: "openai/gpt-5.5", MinPlan: PlanTeam, Vision: true},
		},
		Tasks: map[TaskType]TaskConfig{
			TaskChat: {Allowed: []string{"google/gemini-3.5-flash", "anthropic/claude-opus-4.8"}},
		},
	}
}

// The regression this file exists for: a task the live ai_config has never heard
// of used to resolve LOCKED, which killed the feature on every plan — including
// the highest — and told the user to upgrade to fix it.
func TestResolve_MissingTaskFallsBackInsteadOfLocking(t *testing.T) {
	setRegistry(t, staleLiveRegistry())

	for _, plan := range []Plan{PlanFree, PlanPro, PlanTeam, PlanAgency} {
		model, locked := Resolve(TaskDesign, plan, "")
		if locked {
			t.Errorf("plan %q: design locked with no design task configured; "+
				"a deploy-order gap must not read as an entitlement failure", plan)
		}
		if model == "" {
			t.Errorf("plan %q: resolved to an empty model", plan)
		}
	}
}

// The fallback must resolve against the LIVE catalog, so it cannot hand back a
// model id that is not deployed — that would just move the failure to OpenRouter.
func TestResolve_FallbackPrefersLiveCatalog(t *testing.T) {
	setRegistry(t, staleLiveRegistry())

	model, locked := Resolve(TaskDesign, PlanTeam, "")
	if locked {
		t.Fatal("unexpected lock")
	}
	live := staleLiveRegistry()
	if _, ok := live.modelByID(model); !ok {
		t.Fatalf("resolved %q, which the live catalog does not describe", model)
	}
	// Team unlocks the top of the design list.
	if model != "anthropic/claude-opus-4.8" {
		t.Errorf("team should get the best allowed design model, got %q", model)
	}
}

// A plan genuinely unlocking nothing is still a lock — the fallback must not
// paper over a real entitlement answer.
func TestResolve_ConfiguredTaskStillLocksWhenPlanUnlocksNothing(t *testing.T) {
	setRegistry(t, Registry{
		Models: []ModelInfo{{ID: "openai/gpt-5.5", MinPlan: PlanTeam}},
		Tasks:  map[TaskType]TaskConfig{TaskScript: {Allowed: []string{"openai/gpt-5.5"}}},
	})
	if _, locked := Resolve(TaskScript, PlanFree, ""); !locked {
		t.Error("free should be locked out of a team-only task")
	}
	if _, locked := Resolve(TaskScript, PlanTeam, ""); locked {
		t.Error("team should not be locked out of a team-only task")
	}
}

// A task nothing knows about anywhere has nothing to run on, and locking is the
// honest answer.
func TestResolve_UnknownTaskEverywhereLocks(t *testing.T) {
	setRegistry(t, staleLiveRegistry())
	if _, locked := Resolve(TaskType("no_such_task"), PlanAgency, ""); !locked {
		t.Error("a task with no config anywhere should lock")
	}
}

// Every model the design lane may pick has to be vision-capable: the design loop
// renders its own output and looks at it.
func TestDefaultRegistry_DesignModelsAreAllVision(t *testing.T) {
	d := defaultRegistry()
	tc, ok := d.Tasks[TaskDesign]
	if !ok || len(tc.Allowed) == 0 {
		t.Fatal("the built-in registry has no design task")
	}
	for _, id := range tc.Allowed {
		m, ok := d.modelByID(id)
		if !ok {
			t.Errorf("design allows %q, which the built-in catalog does not describe", id)
			continue
		}
		if !m.Vision {
			t.Errorf("design allows %q, which cannot see its own render", id)
		}
	}
}

// Every id named in a task list must exist in the catalog it ships with —
// the drift that put google/gemini-3.1-pro-preview in the lists while the
// deployed catalog carried google/gemini-3-pro-preview.
func TestDefaultRegistry_EveryAllowedModelExists(t *testing.T) {
	d := defaultRegistry()
	for task, tc := range d.Tasks {
		for _, id := range tc.Allowed {
			if _, ok := d.modelByID(id); !ok {
				t.Errorf("task %q allows %q, which is not in the catalog", task, id)
			}
		}
	}
}
