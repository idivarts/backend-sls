package tools

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// TestAllToolsRegistered verifies every registered fetch tool exposes a
// non-empty, unique name and a description, and that AllTools() mirrors the
// registry length.
func TestAllToolsRegistered(t *testing.T) {
	all := AllTools()
	if len(all) != len(registry) {
		t.Fatalf("AllTools()=%d, registry=%d", len(all), len(registry))
	}
	seen := map[string]bool{}
	for _, tool := range all {
		name := tool.Function.Name
		if strings.TrimSpace(name) == "" {
			t.Errorf("tool with empty name")
		}
		if strings.TrimSpace(tool.Function.Description) == "" {
			t.Errorf("tool %q has empty description", name)
		}
		if seen[name] {
			t.Errorf("duplicate tool name %q", name)
		}
		seen[name] = true
		if !Has(name) {
			t.Errorf("Has(%q)=false for a registered tool", name)
		}
	}
	// A representative sample of the expected suite must be present.
	for _, want := range []string{
		"get_content_in_timeframe", "get_post_analytics", "get_account_overview",
		"get_inbox_conversations", "get_brand_profile", "get_entitlements_and_usage",
	} {
		if !Has(want) {
			t.Errorf("expected tool %q not registered", want)
		}
	}
}

// TestDispatchUnknown ensures an unrecognized tool name yields ErrToolNotFound
// (so the chat loop can fall through to its unknown-tool handling).
func TestDispatchUnknown(t *testing.T) {
	_, err := Dispatch(context.Background(), "brand123", "does_not_exist", "")
	if !errors.Is(err, ErrToolNotFound) {
		t.Fatalf("Dispatch unknown tool: got err=%v, want ErrToolNotFound", err)
	}
}
