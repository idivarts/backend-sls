package director

import (
	"context"
	"strings"
	"testing"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
)

// Everything here is a path that returns BEFORE the library read, or is pure
// string building. Resolution against Firestore is covered on dev rather than
// here — but the refusals below are the ones that decide whether a user's
// money gets spent on a brief whose files are wrong, so they are worth
// pinning without a database.

func TestAttachUserMediaRefusesMoreThanTheCap(t *testing.T) {
	refs := make([]trendlymodels.RunAttachment, MaxAttachments+1)
	for i := range refs {
		refs[i] = trendlymodels.RunAttachment{Ref: "a", MediaID: "m"}
	}

	var m Manifest
	problems := m.AttachUserMedia(context.Background(), "brand1", refs)

	if !Fatal(problems) {
		t.Fatalf("over the cap should be fatal, got %v", problems)
	}
	// The MESSAGE is asserted, not just fatality. Without the cap this call
	// would fall through to the library read, fail there for want of a
	// database, and return a fatal problem anyway — so a test that only
	// checked `Fatal` would pass with the cap deleted.
	if len(problems) != 1 || !strings.Contains(problems[0].Message, "at most 4") {
		t.Fatalf("the refusal should name the cap, got %v", problems)
	}
	if len(m.Attachments) != 0 {
		t.Fatalf("nothing should be attached when the whole set is refused")
	}
}

func TestAttachUserMediaIsSilentWithNoAttachments(t *testing.T) {
	var m Manifest
	if p := m.AttachUserMedia(context.Background(), "brand1", nil); p != nil {
		t.Fatalf("no attachments should produce no problems, got %v", p)
	}
}

func TestAttachUserMediaRefusesAHandleWithNoFile(t *testing.T) {
	// Returns before any library read, because `ids` ends up empty.
	var m Manifest
	problems := m.AttachUserMedia(context.Background(), "brand1",
		[]trendlymodels.RunAttachment{{Ref: "attachment1", MediaID: "   "}})

	if !Fatal(problems) {
		t.Fatalf("a handle with no file behind it should be fatal, got %v", problems)
	}
	// Again the message, not just fatality: drop the blank check and this
	// reaches the library read and fails there instead, for the wrong reason.
	if len(problems) != 1 || !strings.Contains(problems[0].Message, "no handle or no file") {
		t.Fatalf("the refusal should say the attachment was incomplete, got %v", problems)
	}
}

func TestAttachmentBlockNamesEveryHandle(t *testing.T) {
	out := AttachmentBlock([]Attachment{
		{Ref: "attachment1", MediaID: "abc", Kind: "image", Aspect: "9:16", Describes: "a face"},
		{Ref: "attachment2", MediaID: "def", Kind: "image"},
	})

	for _, want := range []string{"@attachment1", "abc", "a face", "@attachment2", "def"} {
		if !strings.Contains(out, want) {
			t.Errorf("the block should name %q:\n%s", want, out)
		}
	}
	// The whole point of the section: without this the planner treats them as
	// two more rows in a list of forty.
	if !strings.Contains(out, "OUTRANK") {
		t.Errorf("the block must say the user's own files outrank library picks:\n%s", out)
	}
}

func TestAttachmentBlockWarnsOnlyWhenAVideoIsAttached(t *testing.T) {
	// The planner gets stills, never frames. A planner that assumes it has
	// watched the clip will describe shots that are not in it — but saying so
	// when nothing is a video is noise that trains it to skim the block.
	withVideo := AttachmentBlock([]Attachment{{Ref: "a1", MediaID: "v", Kind: "video", DurationMs: 8000}})
	if !strings.Contains(withVideo, "NOT seen inside") {
		t.Errorf("an attached video must carry the not-seen caveat:\n%s", withVideo)
	}

	imagesOnly := AttachmentBlock([]Attachment{{Ref: "a1", MediaID: "i", Kind: "image"}})
	if strings.Contains(imagesOnly, "NOT seen inside") {
		t.Errorf("images alone must not carry the video caveat:\n%s", imagesOnly)
	}
}

func TestAttachmentBlockIsEmptyWithNoAttachments(t *testing.T) {
	// BuildUserMessage writes this unconditionally, so an empty set must add
	// literally nothing to the prompt.
	if out := AttachmentBlock(nil); out != "" {
		t.Fatalf("no attachments should add nothing to the prompt, got %q", out)
	}
}

func TestBuildUserMessageCarriesTheAttachments(t *testing.T) {
	m := Manifest{Attachments: []Attachment{
		{Ref: "attachment1", MediaID: "abc123", Kind: "image", Describes: "a face"},
	}}

	msg, err := BuildUserMessage(m, "@attachment1 is the face of the video")
	if err != nil {
		t.Fatalf("BuildUserMessage: %v", err)
	}
	if !strings.Contains(msg, "FILES THE USER ATTACHED") {
		t.Errorf("the attachment block must reach the planner:\n%s", msg)
	}
	// Above the manifest, not buried in it: the manifest is forty rows long.
	if strings.Index(msg, "FILES THE USER ATTACHED") > strings.Index(msg, "CAPABILITY MANIFEST") {
		t.Errorf("the attachments should come before the manifest, not after it")
	}
}
