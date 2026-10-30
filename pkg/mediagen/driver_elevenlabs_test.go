package mediagen

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/idivarts/backend-sls/pkg/elevenlabs"
)

func TestElevenlabsServesVideoOnly(t *testing.T) {
	d := elevenlabsDriver{}
	if !d.Supports(KindVideo) {
		t.Error("should serve video")
	}
	for _, k := range []Kind{KindImage, KindStock, KindMusic} {
		if d.Supports(k) {
			t.Errorf("should not serve %q", k)
		}
	}
}

func TestElevenlabsRejectsWhatTheAPIForbids(t *testing.T) {
	d := elevenlabsDriver{}
	cases := map[string]Spec{
		"no model": {Kind: KindVideo, Prompt: "a cat"},
		"wrong kind": {Kind: KindImage, ModelID: elevenlabs.VideoVeo31},
		// The API forbids this outright. Rejecting beats dropping one silently:
		// a caller who pinned a last frame and got a clip ignoring it has paid
		// for a misunderstanding.
		"frame plus reference images": {
			Kind: KindVideo, ModelID: elevenlabs.VideoVeo31, Prompt: "a cat",
			StartFrame: &AssetRef{URL: "https://cdn/x.png"},
			ImageRefs:  []AssetRef{{URL: "https://cdn/y.png"}},
		},
		"source video": {
			Kind: KindVideo, ModelID: elevenlabs.VideoVeo31, Prompt: "a cat",
			SourceVideo: &AssetRef{URL: "https://cdn/v.mp4"},
		},
	}
	for name, s := range cases {
		if _, err := d.Submit(context.Background(), s); !errors.Is(err, ErrUnsupported) {
			t.Errorf("%s: want ErrUnsupported, got %v", name, err)
		}
	}
}

func TestResultFromGenerationMapsEveryStatus(t *testing.T) {
	// pending and generating BOTH mean "still running". Treating only one as
	// running is how a poll loop decides a live job has vanished.
	for _, s := range []string{elevenlabs.VideoPending, elevenlabs.VideoGenerating} {
		got := resultFromGeneration(&elevenlabs.VideoGeneration{ID: "g1", Status: s})
		if got.Status != StatusPending {
			t.Errorf("%s should be pending, got %s", s, got.Status)
		}
	}

	done := resultFromGeneration(&elevenlabs.VideoGeneration{
		ID: "g1", Status: elevenlabs.VideoCompleted,
		ContentURL: "https://signed/clip.mp4", ContentMimeType: "video/mp4",
	})
	if done.Status != StatusSucceeded || len(done.Assets) != 1 {
		t.Fatalf("got %+v", done)
	}
	// Must NOT be hotlinkable: the url is signed and expires within the hour,
	// and a composition is re-rendered weeks later.
	if done.Assets[0].Hotlink {
		t.Error("a signed, expiring url must never be marked hotlinkable")
	}

	failed := resultFromGeneration(&elevenlabs.VideoGeneration{
		ID: "g1", Status: elevenlabs.VideoFailed, FailureReason: "moderated",
	})
	if failed.Status != StatusFailed {
		t.Fatalf("got %+v", failed)
	}
	// A moderation refusal is something the user can act on, so it says so
	// rather than apologising generically.
	if !strings.Contains(failed.Error, "declined") {
		t.Errorf("moderated should explain itself, got %q", failed.Error)
	}
}

func TestFailureMessagesOfferANextStep(t *testing.T) {
	for reason, want := range map[string]string{
		"moderated":          "describing the shot differently",
		"invalid_parameters": "different length or size",
		"timeout":            "shorter",
	} {
		got := userFacingFailure(&elevenlabs.VideoGeneration{FailureReason: reason})
		if !strings.Contains(got, want) {
			t.Errorf("%s: %q does not mention %q", reason, got, want)
		}
	}
	// An unmapped reason still has to say something true.
	generic := userFacingFailure(&elevenlabs.VideoGeneration{FailureReason: "internal_error"})
	if generic == "" {
		t.Error("an unmapped failure still needs a message")
	}
}

func TestWebhookAndPollAgreeOnWhatDoneLooksLike(t *testing.T) {
	// The payload is documented as matching the terminal GET response, so both
	// decode through one path — which is what stops them disagreeing.
	body := []byte(`{"id":"gen_1","status":"completed","content_url":"https://signed/c.mp4","content_mime_type":"video/mp4"}`)
	fromHook, err := elevenlabsDriver{}.ParseWebhook(body)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	fromPoll := resultFromGeneration(&elevenlabs.VideoGeneration{
		ID: "gen_1", Status: elevenlabs.VideoCompleted,
		ContentURL: "https://signed/c.mp4", ContentMimeType: "video/mp4",
	})
	if fromHook.Status != fromPoll.Status || fromHook.JobID != fromPoll.JobID {
		t.Errorf("webhook %+v and poll %+v disagree", fromHook, fromPoll)
	}
	if fromHook.Assets[0].URL != fromPoll.Assets[0].URL {
		t.Error("webhook and poll produced different urls")
	}
}

func TestWebhookRejectsRubbish(t *testing.T) {
	d := elevenlabsDriver{}
	for name, body := range map[string]string{
		"not json":    `{`,
		"no id":       `{"status":"completed"}`,
		"odd status":  `{"id":"g","status":"banana"}`,
	} {
		if _, err := d.ParseWebhook([]byte(body)); err == nil {
			t.Errorf("%s: should be rejected", name)
		}
	}
}

func TestParseWebhookRoutesByProviderName(t *testing.T) {
	// The provider comes from the route, never the body — otherwise whoever
	// sends the payload picks the parser, and these arrive from the internet.
	if _, err := ParseWebhook("pexels", []byte(`{"id":"x","status":"completed"}`)); !errors.Is(err, ErrUnsupported) {
		t.Errorf("a provider that sends no callbacks should say so, got %v", err)
	}
	if _, err := ParseWebhook("nope", []byte(`{}`)); !errors.Is(err, ErrNoProvider) {
		t.Errorf("want ErrNoProvider, got %v", err)
	}
}
