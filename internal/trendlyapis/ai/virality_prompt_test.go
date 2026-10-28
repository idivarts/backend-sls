package ai

import (
	"strings"
	"testing"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
)

func TestModuleHasVirality(t *testing.T) {
	cases := map[string]bool{
		moduleContent:    true,
		moduleCalendar:   true,
		moduleStrategy:   true,
		moduleOnboarding: false,
		moduleMedia:      false,
		"":               false,
	}
	for module, want := range cases {
		if got := moduleHasVirality(module); got != want {
			t.Errorf("moduleHasVirality(%q) = %v, want %v", module, got, want)
		}
	}
}

func TestPlatformPlaybookBlock(t *testing.T) {
	t.Run("empty input yields no block", func(t *testing.T) {
		if got := platformPlaybookBlock(nil); got != "" {
			t.Errorf("want empty, got %q", got)
		}
	})

	t.Run("unknown platforms are skipped, not guessed at", func(t *testing.T) {
		got := platformPlaybookBlock([]trendlymodels.Platform{"myspace", "orkut"})
		if got != "" {
			t.Errorf("want empty for unknown platforms, got %q", got)
		}
	})

	t.Run("only the targeted platform is included", func(t *testing.T) {
		got := platformPlaybookBlock([]trendlymodels.Platform{trendlymodels.PlatformInstagram})
		if !strings.Contains(got, "INSTAGRAM") {
			t.Error("want the Instagram playbook")
		}
		// The whole point of targeting: a Reel author must not pay for Reddit.
		for _, other := range []string{"REDDIT", "YOUTUBE", "LINKEDIN", "TWITTER"} {
			if strings.Contains(got, other) {
				t.Errorf("unexpected %s playbook in an Instagram-only block", other)
			}
		}
	})

	t.Run("linkedin_page folds into linkedin without duplicating", func(t *testing.T) {
		got := platformPlaybookBlock([]trendlymodels.Platform{
			trendlymodels.PlatformLinkedIn,
			trendlymodels.PlatformLinkedInPage,
		})
		if n := strings.Count(got, "LINKEDIN\n"); n != 1 {
			t.Errorf("want exactly 1 LinkedIn playbook, got %d", n)
		}
	})

	t.Run("page alone still resolves to the linkedin playbook", func(t *testing.T) {
		got := platformPlaybookBlock([]trendlymodels.Platform{trendlymodels.PlatformLinkedInPage})
		if !strings.Contains(got, "LINKEDIN") {
			t.Error("linkedin_page alone should still load LinkedIn's rules")
		}
	})

	t.Run("duplicates collapse", func(t *testing.T) {
		got := platformPlaybookBlock([]trendlymodels.Platform{
			trendlymodels.PlatformInstagram,
			trendlymodels.PlatformInstagram,
		})
		if n := strings.Count(got, "INSTAGRAM\n"); n != 1 {
			t.Errorf("want 1 Instagram playbook, got %d", n)
		}
	})

	t.Run("case and whitespace are tolerated", func(t *testing.T) {
		got := platformPlaybookBlock([]trendlymodels.Platform{" Instagram "})
		if !strings.Contains(got, "INSTAGRAM") {
			t.Errorf("want the Instagram playbook for a padded, mixed-case value, got %q", got)
		}
	})

	t.Run("the cap keeps the platforms whose rules differ most", func(t *testing.T) {
		// Alphabetical truncation would keep facebook/instagram/linkedin/reddit
		// and drop youtube and twitter, handing a YouTube video Reddit's rules.
		all := []trendlymodels.Platform{
			trendlymodels.PlatformReddit,
			trendlymodels.PlatformFacebook,
			trendlymodels.PlatformInstagram,
			trendlymodels.PlatformYouTube,
			trendlymodels.PlatformLinkedIn,
			trendlymodels.PlatformTwitter,
		}
		got := platformPlaybookBlock(all)
		for _, want := range []string{"INSTAGRAM", "YOUTUBE", "LINKEDIN", "TWITTER"} {
			if !strings.Contains(got, want) {
				t.Errorf("the cap dropped %s, which it should have kept", want)
			}
		}
		if strings.Contains(got, "REDDIT\n") || strings.Contains(got, "FACEBOOK\n") {
			t.Error("the cap kept a lower-priority playbook over a higher-priority one")
		}
	})

	t.Run("caps the number of playbooks", func(t *testing.T) {
		all := []trendlymodels.Platform{
			trendlymodels.PlatformInstagram,
			trendlymodels.PlatformFacebook,
			trendlymodels.PlatformYouTube,
			trendlymodels.PlatformLinkedIn,
			trendlymodels.PlatformTwitter,
			trendlymodels.PlatformReddit,
		}
		got := platformPlaybookBlock(all)
		// Every playbook opens with its name on its own line.
		if n := strings.Count(got, "\n- "); n == 0 {
			t.Fatal("expected playbook bullets")
		}
		var included int
		for _, name := range []string{"INSTAGRAM\n", "FACEBOOK\n", "YOUTUBE", "LINKEDIN\n", "TWITTER", "REDDIT\n"} {
			if strings.Contains(got, name) {
				included++
			}
		}
		if included > maxPlatformPlaybooks {
			t.Errorf("want at most %d playbooks, got %d", maxPlatformPlaybooks, included)
		}
	})

	t.Run("order is stable across calls", func(t *testing.T) {
		in := []trendlymodels.Platform{
			trendlymodels.PlatformTwitter,
			trendlymodels.PlatformInstagram,
			trendlymodels.PlatformYouTube,
		}
		first := platformPlaybookBlock(in)
		for i := 0; i < 5; i++ {
			if got := platformPlaybookBlock(in); got != first {
				t.Fatal("playbook block is not deterministic across calls")
			}
		}
		// Reordering the input must not reorder the output either — an unstable
		// system prompt would make the Phase 2 scorer's variance unattributable.
		shuffled := []trendlymodels.Platform{
			trendlymodels.PlatformYouTube,
			trendlymodels.PlatformTwitter,
			trendlymodels.PlatformInstagram,
		}
		if got := platformPlaybookBlock(shuffled); got != first {
			t.Error("playbook block depends on input order")
		}
	})
}

func TestPlatformList(t *testing.T) {
	cases := []struct {
		name   string
		single string
		many   []string
		want   []trendlymodels.Platform
	}{
		{"neither", "", nil, []trendlymodels.Platform{}},
		{"single only", "instagram", nil, []trendlymodels.Platform{"instagram"}},
		{"list only", "", []string{"instagram", "youtube"}, []trendlymodels.Platform{"instagram", "youtube"}},
		{"list wins over single", "reddit", []string{"instagram"}, []trendlymodels.Platform{"instagram"}},
		{"blanks dropped", "", []string{"instagram", "", "  "}, []trendlymodels.Platform{"instagram"}},
		{"whitespace trimmed", " instagram ", nil, []trendlymodels.Platform{"instagram"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := platformList(c.single, c.many)
			if len(got) != len(c.want) {
				t.Fatalf("got %v, want %v", got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("got %v, want %v", got, c.want)
				}
			}
		})
	}
}

// A variation request names two platforms and only one of them is the one
// being written for. Getting this backwards is silent: the prompt still reads
// fluently, it is just telling the model to optimise for LinkedIn using
// Instagram's ranking rules.
func TestTargetPlatformListPrefersTheVariationBeingWritten(t *testing.T) {
	cases := []struct {
		name     string
		target   string
		generic  string
		wantHead trendlymodels.Platform
		wantLen  int
	}{
		{"variation request uses the target", "linkedin", "instagram", "linkedin", 1},
		{"generic request falls back", "", "instagram", "instagram", 1},
		{"blank target is not a target", "   ", "instagram", "instagram", 1},
		{"neither set yields nothing", "", "", "", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := targetPlatformList(c.target, c.generic)
			if len(got) != c.wantLen {
				t.Fatalf("got %v, want %d entries", got, c.wantLen)
			}
			if c.wantLen > 0 && got[0] != c.wantHead {
				t.Errorf("got %v, want %v", got[0], c.wantHead)
			}
		})
	}
}

// The craft contract is the one block whose absence is silent: nothing fails,
// the model just writes worse content. These lock in the rules that came from
// a platform's own documentation, so a future edit cannot quietly drop them.
func TestViralityCraftCoversTheSourcedRules(t *testing.T) {
	musts := []string{
		"reshare",          // Instagram's top Reels prediction
		"follower count",   // TikTok: not a direct ranking factor
		"Skip",             // a predicted negative
		"watermark",        // one of Instagram's six deprioritizers
		"engagement bait",  // demoted by Meta, suppressed by LinkedIn
		"Berger & Milkman", // the arousal finding is attributed, not asserted
		"ONE ask",          // the single-CTA rule
		"MUTED",            // sound-off readability
	}
	for _, m := range musts {
		if !strings.Contains(viralityCraftInstructions, m) {
			t.Errorf("craft contract no longer mentions %q", m)
		}
	}
}

func TestViralityPatternsCarryAntiUses(t *testing.T) {
	// Every archetype needs its "not when" half — a curiosity gap with no
	// anti-use is a clickbait generator.
	archetypes := strings.Count(viralityPatternsBlock, "\n- ")
	nots := strings.Count(viralityPatternsBlock, "Not ") + strings.Count(viralityPatternsBlock, "Never ")
	if archetypes == 0 {
		t.Fatal("no archetypes found")
	}
	if nots < 5 {
		t.Errorf("only %d anti-uses across %d bullets; archetypes need their 'not when' half", nots, archetypes)
	}
}
