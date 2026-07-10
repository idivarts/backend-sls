package scenegraph

import (
	"strings"
	"testing"
)

func sampleDoc() *Document {
	return SeedImageScene(SizePost, "Summer Sale", "up to 40% off", BrandKit{})
}

func TestSeedIsValid(t *testing.T) {
	if err := ValidateDocument(sampleDoc()); err != nil {
		t.Fatalf("seed image scene invalid: %v", err)
	}
	if err := ValidateDocument(SeedVideoScene(SizeReel, "Big News", "Shop now", BrandKit{})); err != nil {
		t.Fatalf("seed video scene invalid: %v", err)
	}
}

func TestParseOps(t *testing.T) {
	arr := []byte(`[{"op":"setText","id":"el_head","text":"Winter Sale"}]`)
	ops, err := ParseOps(arr)
	if err != nil || len(ops) != 1 || ops[0].Op != OpSetText {
		t.Fatalf("array parse failed: %v %v", ops, err)
	}
	obj := []byte(`{"ops":[{"op":"moveToZone","id":"el_head","zone":"top"}]}`)
	ops, err = ParseOps(obj)
	if err != nil || len(ops) != 1 || ops[0].Op != OpMoveToZone {
		t.Fatalf("object parse failed: %v %v", ops, err)
	}
}

func TestApplyDoesNotMutateInput(t *testing.T) {
	d := sampleDoc()
	orig := d.FindElement("el_head").Text
	res, err := Apply(d, []Op{{Op: OpSetText, ID: "el_head", Text: "Changed"}})
	if err != nil {
		t.Fatal(err)
	}
	if d.FindElement("el_head").Text != orig {
		t.Fatalf("input document was mutated")
	}
	if res.Document.FindElement("el_head").Text != "Changed" {
		t.Fatalf("op not applied to copy")
	}
}

func TestApplyOps(t *testing.T) {
	d := sampleDoc()
	ops := []Op{
		{Op: OpSetText, ID: "el_head", Text: "Winter Sale"},
		{Op: OpMoveToZone, ID: "el_head", Zone: ZoneTop},
		{Op: OpRestyle, ID: "el_head", Style: &Style{Scale: 1.4, Color: "brand.navy"}},
		{Op: OpDuplicate, ID: "el_sub"},
		{Op: OpDelete, ID: "el_sub"},
		{Op: OpSetBackground, Background: &Background{Kind: "color", Value: "#000000"}},
		{Op: OpAddElement, Element: &Element{Type: ElText, Zone: ZoneCenter, Text: "New"}},
	}
	res, err := Apply(d, ops)
	if err != nil {
		t.Fatal(err)
	}
	nd := res.Document
	if el := nd.FindElement("el_head"); el.Text != "Winter Sale" || el.Zone != ZoneTop || el.Style.Scale != 1.4 {
		t.Fatalf("setText/move/restyle failed: %+v", el)
	}
	if nd.Background.Kind != "color" || nd.Background.Value != "#000000" {
		t.Fatalf("setBackground failed: %+v", nd.Background)
	}
	// el_sub duplicated then original deleted -> exactly one el_sub-derived text remains.
	if nd.FindElement("el_sub") != nil {
		t.Fatalf("delete of el_sub failed")
	}
	for _, r := range res.Results {
		if !r.Applied && !r.Pending {
			t.Fatalf("op not applied: %s (%s)", r.Describe, r.Warning)
		}
	}
}

func TestRegenerateElementIsPending(t *testing.T) {
	d := sampleDoc()
	res, _ := Apply(d, []Op{{Op: OpRegenerateElement, ID: "el_hero", Instruction: "make it pop"}})
	if len(res.Results) != 1 || !res.Results[0].Pending {
		t.Fatalf("regenerateElement should be pending, got %+v", res.Results)
	}
}

func TestValidateOpsRejectsUnknownTargets(t *testing.T) {
	d := sampleDoc()
	problems := ValidateOps(d, []Op{
		{Op: OpSetText, ID: "nope", Text: "x"},
		{Op: "frobnicate", ID: "el_head"},
	})
	if len(problems) != 2 {
		t.Fatalf("expected 2 problems, got %v", problems)
	}
}

func TestLockedElementNotDeleted(t *testing.T) {
	d := sampleDoc()
	d.FindElement("el_logo").Locked = true
	res, _ := Apply(d, []Op{{Op: OpDelete, ID: "el_logo"}})
	if res.Results[0].Applied {
		t.Fatalf("locked element should not be deleted")
	}
}

func TestLayoutBoxesNormalized(t *testing.T) {
	l := ComputeLayout(sampleDoc())
	if len(l.Boxes) == 0 {
		t.Fatal("no boxes computed")
	}
	for _, b := range l.Boxes {
		if b.Norm.X < 0 || b.Norm.Y < 0 || b.Norm.X+b.Norm.W > 1.001 || b.Norm.Y+b.Norm.H > 1.001 {
			t.Fatalf("box %s out of normalized bounds: %+v", b.ID, b.Norm)
		}
	}
	if _, ok := l.BoxFor("el_head"); !ok {
		t.Fatal("el_head not placed")
	}
}

func TestRenderSVG(t *testing.T) {
	svg := RenderSVG(sampleDoc(), DefaultResolver())
	if !strings.HasPrefix(svg, "<svg") || !strings.Contains(svg, "Summer") || !strings.HasSuffix(svg, "</svg>") {
		t.Fatalf("unexpected svg: %.120s", svg)
	}
}

func TestVideoOps(t *testing.T) {
	d := SeedVideoScene(SizeReel, "Big News", "Shop now", BrandKit{})
	res, err := Apply(d, []Op{
		{Op: OpSetDuration, SceneID: "sc1", Seconds: 4},
		{Op: OpReorderScene, SceneID: "sc2", Index: 0},
		{Op: OpEnableCaptions, Source: "voiceover"},
		{Op: OpSetAudio, Type: "music", Brief: "upbeat lofi"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Document.Timeline.Scenes[0].ID != "sc2" {
		t.Fatalf("reorder failed: %v", res.Document.Timeline.Scenes)
	}
	if !res.Document.Timeline.Captions.Enabled {
		t.Fatalf("captions not enabled")
	}
}
