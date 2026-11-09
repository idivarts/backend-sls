package director

import (
	"strings"
	"testing"
)

// The guarantee these lock in: a plan is never blocked because nobody knew a
// voice id. That was a real dead end — the ids live in a provider account, so
// the refusal asked the user for something they could not get.

func narrating() *Plan {
	return &Plan{Audio: AudioPlan{VoiceModel: "eleven_multilingual_v2"}}
}

func catalogue() Manifest {
	return Manifest{Voices: []VoiceOption{
		{ID: "v_zara", Name: "Zara", Labels: "british · bright"},
		{ID: "v_adam", Name: "Adam", Labels: "american · calm · narration"},
		{ID: "v_mila", Name: "Mila", Labels: "american · warm"},
	}}
}

// The planner saw the catalogue and the content; it is the better judge of fit
// than any rule here, so its choice is never second-guessed.
func TestAChosenVoiceIsLeftAlone(t *testing.T) {
	p := narrating()
	p.Audio.VoiceID = "v_mila"

	got, supplied := p.EnsureVoice(catalogue())
	if supplied {
		t.Error("the plan's own choice was overwritten")
	}
	if got != "v_mila" {
		t.Errorf("voice = %q, want v_mila", got)
	}
}

// A studio that has been narrated before must go on sounding like itself.
// Changing narrator between runs is worse than any catalogue pick.
func TestTheStudiosOwnVoiceWinsOverTheCatalogue(t *testing.T) {
	m := catalogue()
	m.PreferredVoiceID = "v_existing"

	p := narrating()
	got, supplied := p.EnsureVoice(m)
	if !supplied || got != "v_existing" {
		t.Fatalf("voice = %q (supplied=%v), want the studio's own", got, supplied)
	}
}

// Of several voices, the one the provider labels for narration.
func TestItPrefersAVoiceBuiltForNarration(t *testing.T) {
	p := narrating()
	got, supplied := p.EnsureVoice(catalogue())
	if !supplied || got != "v_adam" {
		t.Fatalf("voice = %q, want the narration-labelled v_adam", got)
	}
}

// No labels to go on: still a voice, and the SAME voice every time. Two plans
// for one brand drifting apart because a provider reordered its listing is a
// difference nobody could explain.
func TestAnUnlabelledCatalogueStillYieldsAStableChoice(t *testing.T) {
	m := Manifest{Voices: []VoiceOption{
		{ID: "v_c", Name: "Carol"}, {ID: "v_a", Name: "Alice"}, {ID: "v_b", Name: "Bob"},
	}}
	first, _ := narrating().EnsureVoice(m)

	// The provider hands them back in a different order next time.
	m.Voices = []VoiceOption{{ID: "v_b", Name: "Bob"}, {ID: "v_c", Name: "Carol"}, {ID: "v_a", Name: "Alice"}}
	second, _ := narrating().EnsureVoice(m)

	if first == "" || first != second {
		t.Fatalf("choice drifted with listing order: %q then %q", first, second)
	}
}

// The provider was unreachable, so there is no catalogue at all. This is the
// case the whole change exists for — it must not end in a refusal.
func TestAnEmptyCatalogueDoesNotBlockThePlan(t *testing.T) {
	p := narrating()
	p.EnsureVoice(Manifest{})

	if Fatal(p.Validate(FixtureManifest("reel", 20, "balanced", 5))) {
		for _, pr := range p.Validate(FixtureManifest("reel", 20, "balanced", 5)) {
			if pr.Fatal && strings.Contains(pr.Message, "voice") {
				t.Fatalf("a missing voice is still fatal: %s", pr.Message)
			}
		}
	}
}

// Nothing is narrated, so there is nothing to keep consistent and no reason to
// name a voice at all.
func TestASilentPlanIsGivenNoVoice(t *testing.T) {
	p := &Plan{}
	if got, supplied := p.EnsureVoice(catalogue()); supplied || got != "" {
		t.Errorf("a plan with no narration was given voice %q", got)
	}
}

// The provider's labels arrive as a map, whose iteration order is random. An
// unsorted flatten would reshuffle the manifest between identical calls.
func TestLabelsFlattenInAStableOrder(t *testing.T) {
	labels := map[string]string{"accent": "american", "age": "young", "use_case": "narration"}
	first := FlattenVoiceLabels(labels)
	for i := 0; i < 20; i++ {
		if FlattenVoiceLabels(labels) != first {
			t.Fatalf("label order is not stable: %q vs %q", first, FlattenVoiceLabels(labels))
		}
	}
	if !strings.Contains(first, "american") || !strings.Contains(first, "narration") {
		t.Errorf("flattened labels lost content: %q", first)
	}
}
