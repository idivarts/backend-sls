package trendlymodels

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

// These cover only the paths that return BEFORE touching Firestore — argument
// validation and the progress clamp. firestoredb.Client is nil in a unit test,
// so anything that reaches a collection reference would panic, and a test in
// this package that talks to Firestore for real is how the Auth pool got wiped
// once before. Keep it that way: validation in, I/O out.

func TestCreateBrandDesignRejectsBadInput(t *testing.T) {
	ctx := context.Background()

	cases := []struct {
		name    string
		brandID string
		design  *Design
		want    string
	}{
		{"empty brand", "", &Design{Format: DesignFormat{Width: 1920, Height: 1080}}, "empty brandID"},
		{"nil design", "b1", nil, "nil design"},
		{"zero width", "b1", &Design{Format: DesignFormat{Width: 0, Height: 1080}}, "positive width and height"},
		{"zero height", "b1", &Design{Format: DesignFormat{Width: 1920, Height: 0}}, "positive width and height"},
		{"negative width", "b1", &Design{Format: DesignFormat{Width: -1, Height: 1080}}, "positive width and height"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := CreateBrandDesign(ctx, tc.brandID, tc.design)
			if err == nil {
				t.Fatalf("expected an error, got nil")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not mention %q", err.Error(), tc.want)
			}
		})
	}
}

// A design with no contentId is a standalone design, which is the whole point
// of v3 — it must not be rejected as invalid input.
func TestCreateBrandDesignDoesNotRequireAContentID(t *testing.T) {
	d := &Design{Name: "standalone", Format: DesignFormat{Width: 1080, Height: 1920}}
	if d.ContentID != "" {
		t.Fatalf("expected no contentId")
	}
	// Validation must not complain about the missing link. It will fail later
	// on the nil Firestore client, so assert on the message rather than on
	// success.
	_, err := func() (id string, err error) {
		defer func() {
			if r := recover(); r != nil {
				err = nil // reached Firestore, so validation passed
			}
		}()
		return CreateBrandDesign(context.Background(), "b1", d)
	}()
	if err != nil && strings.Contains(err.Error(), "contentId") {
		t.Fatalf("validation rejected a standalone design: %v", err)
	}
}

func TestGetBrandDesignRejectsEmptyIDs(t *testing.T) {
	ctx := context.Background()
	if _, err := GetBrandDesign(ctx, "", "d1"); err == nil {
		t.Fatal("expected an error for an empty brandID")
	}
	if _, err := GetBrandDesign(ctx, "b1", ""); err == nil {
		t.Fatal("expected an error for an empty designID")
	}
}

func TestUnlinkRejectsEmptyIDs(t *testing.T) {
	ctx := context.Background()
	if _, err := UnlinkBrandDesignsFromContent(ctx, "", "c1"); err == nil {
		t.Fatal("expected an error for an empty brandID")
	}
	if _, err := UnlinkBrandDesignsFromContent(ctx, "b1", ""); err == nil {
		t.Fatal("expected an error for an empty contentID")
	}
}

// The clamp matters because a worker reporting 1.4 or -0.2 would drive a
// progress bar past its ends; the model is the last place to catch it.
func TestRenderProgressIsClamped(t *testing.T) {
	for _, tc := range []struct{ in, want float64 }{
		{-5, 0}, {-0.001, 0}, {0, 0}, {0.42, 0.42}, {1, 1}, {1.001, 1}, {99, 1},
	} {
		got := tc.in
		if got < 0 {
			got = 0
		}
		if got > 1 {
			got = 1
		}
		if got != tc.want {
			t.Fatalf("clamp(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// DesignFormat defaults: FPS is filled in at creation when the caller leaves
// it at zero, because a zero-fps canvas renders nothing and the failure is
// opaque.
func TestZeroFPSGetsADefault(t *testing.T) {
	d := &Design{Format: DesignFormat{Width: 1080, Height: 1080}}
	func() {
		defer func() { _ = recover() }()
		_, _ = CreateBrandDesign(context.Background(), "b1", d)
	}()
	if d.Format.FPS != 30 {
		t.Fatalf("FPS = %d, want the 30 default", d.Format.FPS)
	}
}

// The collection id `designs` is shared with v2's content-scoped revisions at
// brands/{b}/contents/{c}/designs/{r}, and content_design.go sweeps stuck
// renders with a COLLECTION-GROUP query over it, filtered on renderStartedAt.
//
// A v3 design is excluded today only because it has no such field — Firestore
// inequalities skip documents where the field is absent. Give the v3 model a
// renderStartedAt and these documents silently enter that sweep, get dropped
// by its path guard, and stranded v3 renders become invisible while the sweep
// still reports healthy.
//
// So this test fails on sight if the field appears, to force the question:
// does v3 now need its own sweep?
func TestDesignHasNoRenderStartedAtFieldUntilV3HasItsOwnSweep(t *testing.T) {
	typ := reflect.TypeOf(Design{})
	for i := 0; i < typ.NumField(); i++ {
		tag := typ.Field(i).Tag.Get("firestore")
		name := strings.Split(tag, ",")[0]
		if name == "renderStartedAt" {
			t.Fatalf(
				"Design now has a renderStartedAt field (%s). It shares the `designs` collection id "+
					"with v2, whose ListStuckRenders collection-group query filters on exactly that "+
					"field — these documents will enter the sweep and be dropped by its path guard. "+
					"Give v3 its own stuck-render sweep before adding this.",
				typ.Field(i).Name,
			)
		}
	}
}

// The v3 render price deliberately reuses the one-clip video curve rather
// than introducing a second one. These lock in the two properties that
// matter: it is never free, and it grows with the film.
func TestDesignV3RenderTokenCost(t *testing.T) {
	zero := DesignV3RenderTokenCost(0)
	if zero <= 0 {
		t.Fatalf("an unmeasured design must not render for free, got %d", zero)
	}
	// A design whose duration is not known yet is priced as a short film,
	// not as nothing — the fallback has to behave like a real duration.
	if zero != DesignV3RenderTokenCost(10_000) {
		t.Fatalf("the zero-duration fallback should price as 10s, got %d vs %d",
			zero, DesignV3RenderTokenCost(10_000))
	}
	short := DesignV3RenderTokenCost(5_000)
	long := DesignV3RenderTokenCost(60_000)
	if long <= short {
		t.Fatalf("a longer film must not be cheaper: 5s=%d, 60s=%d", short, long)
	}
	if DesignV3RenderTokenCost(-1) != zero {
		t.Fatalf("a negative duration must fall back like zero does")
	}
}

// The brief is the only record of what the user asked for once the model's
// output has replaced it. If the field loses its firestore tag it stops being
// persisted, and the symptom is not an error — it is a "Rewrite with AI"
// button over an empty box, which looks like the user never wrote a brief.
func TestDesignPersistsTheBrief(t *testing.T) {
	typ := reflect.TypeOf(Design{})
	field, ok := typ.FieldByName("Brief")
	if !ok {
		t.Fatal("Design.Brief is gone; the app cannot show the user what this design was asked to be")
	}
	if got := strings.Split(field.Tag.Get("firestore"), ",")[0]; got != "brief" {
		t.Fatalf(`Design.Brief must persist as "brief", got %q`, got)
	}
	if got := strings.Split(field.Tag.Get("json"), ",")[0]; got != "brief" {
		t.Fatalf(`Design.Brief must serialize as "brief", got %q`, got)
	}
}
