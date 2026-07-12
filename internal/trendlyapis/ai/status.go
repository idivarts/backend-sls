package ai

import (
	"strings"
	"time"
)

// streamHeartbeatInterval is how often a contentless "ping" frame is pushed to
// the client for the whole duration of a chat turn. It must be comfortably
// below the client's streaming-silence watchdog window so a slow-but-alive turn
// (a long server tool, a slow model round-trip between tool steps) never trips
// the "response was interrupted" watchdog while the lambda is still working.
const streamHeartbeatInterval = 15 * time.Second

// wsStatus pushes a human-readable progress label to the client mid-turn. It
// both tells the user what the AI is doing during a silent gap (a server tool
// running, before the first token) and — like any WS frame — re-arms the
// client's streaming watchdog. Empty labels are dropped.
func wsStatus(conn, convID, label string) {
	if label == "" {
		return
	}
	wsSend(conn, map[string]any{
		"type":           "status",
		"conversationId": convID,
		"label":          label,
	})
}

// wsPing is a contentless heartbeat: it keeps the client's streaming watchdog
// alive during a long silent operation WITHOUT changing the visible status
// label (the client re-arms its watchdog on any frame but has no handler for
// "ping"). Emitted on a timer for the whole turn — see startHeartbeat.
func wsPing(conn, convID string) {
	wsSend(conn, map[string]any{
		"type":           "ping",
		"conversationId": convID,
	})
}

// startHeartbeat begins pushing a ping frame to the connection every
// streamHeartbeatInterval until the returned stop function is called (defer it
// at the top of a turn). The whole AI turn runs synchronously in the WS lambda
// and can be silent for a while (no token deltas flow while a server tool runs
// or the next model step is being requested); the heartbeat guarantees the
// client sees regular activity so its watchdog only fires when the turn is
// genuinely dead (lambda timeout / dropped socket), never merely slow.
func startHeartbeat(conn, convID string) (stop func()) {
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(streamHeartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				wsPing(conn, convID)
			}
		}
	}()
	var once bool
	return func() {
		if once {
			return
		}
		once = true
		close(done)
	}
}

// toolStatusLabel maps a tool name to a friendly present-progressive label
// shown to the user while that tool runs. Curated labels win; anything else
// falls back to a readable phrase derived from the tool name so a newly-added
// tool still gets a sensible label for free.
func toolStatusLabel(name string) string {
	if label, ok := curatedToolLabels[name]; ok {
		return label
	}
	return humanizeToolName(name)
}

var curatedToolLabels = map[string]string{
	// Brand / onboarding
	"update_brand_memory": "Updating brand memory…",
	"set_brand_fields":    "Saving your brand details…",
	"complete_onboarding": "Finalizing your brand…",
	// Strategy
	"set_strategy_brief":    "Noting your strategy brief…",
	"generate_strategy_doc": "Writing your strategy…",
	"apply_strategy_edit":   "Updating your strategy…",
	// Calendar / content
	"list_calendar":  "Checking your content calendar…",
	"create_content": "Adding content to your calendar…",
	"update_content": "Updating your content…",
	"move_content":   "Rescheduling your content…",
	"remove_content": "Removing content…",
	// Studio (design / media)
	"generate_design":    "Designing your visual…",
	"apply_design_edits": "Editing your design…",
	"generate_image":     "Generating an image…",
	"generate_music":     "Composing music…",
	"generate_voiceover": "Recording the voiceover…",
}

// humanizeToolName turns a snake_case tool name into a friendly progress phrase
// based on its verb prefix, e.g. get_top_performing_posts → "Reading your top
// performing posts…".
func humanizeToolName(name string) string {
	parts := strings.Split(name, "_")
	if len(parts) == 0 || parts[0] == "" {
		return "Working…"
	}
	verb, rest := parts[0], strings.Join(parts[1:], " ")
	switch verb {
	case "get", "list", "fetch", "read", "search":
		if rest == "" {
			return "Reading your data…"
		}
		return "Reading your " + rest + "…"
	case "generate", "create", "make", "compose", "draft":
		if rest == "" {
			return "Creating…"
		}
		return "Creating " + rest + "…"
	case "set", "update", "apply", "save", "move", "remove", "delete":
		if rest == "" {
			return "Updating…"
		}
		return "Updating your " + rest + "…"
	default:
		return "Working…"
	}
}
