package mediagen

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeProvider struct {
	name   string
	kinds  map[Kind]bool
	submit func(Spec) (Result, error)
}

func (f fakeProvider) Name() string         { return f.name }
func (f fakeProvider) Supports(k Kind) bool { return f.kinds[k] }
func (f fakeProvider) Submit(_ context.Context, s Spec) (Result, error) {
	if f.submit != nil {
		return f.submit(s)
	}
	return Result{Status: StatusSucceeded}, nil
}
func (f fakeProvider) Get(context.Context, string) (Result, error) {
	return Result{}, ErrNotAsynchronous
}

func TestRealDriversAreRegistered(t *testing.T) {
	// Importing the package must be enough — a driver registered lazily could
	// be missing on the one code path that needs it.
	for _, want := range []string{"openrouter", "pexels"} {
		if _, ok := ByName(want); !ok {
			t.Errorf("driver %q is not registered; have %v", want, Registered())
		}
	}
}

func TestSubmitRejectsUnknownProvider(t *testing.T) {
	_, err := Submit(context.Background(), "nope", Spec{Kind: KindImage})
	if !errors.Is(err, ErrNoProvider) {
		t.Errorf("want ErrNoProvider, got %v", err)
	}
}

func TestSubmitRejectsAProviderThatDoesNotServeTheKind(t *testing.T) {
	// Routing by provider name alone would send a video request to an image
	// driver and get back something the caller never asked for.
	Register(fakeProvider{name: "images-only", kinds: map[Kind]bool{KindImage: true}})
	_, err := Submit(context.Background(), "images-only", Spec{Kind: KindVideo})
	if !errors.Is(err, ErrNoProvider) {
		t.Errorf("want ErrNoProvider for an unsupported kind, got %v", err)
	}
}

func TestSubmitRoutesToTheNamedProvider(t *testing.T) {
	var got Spec
	Register(fakeProvider{
		name:  "recorder",
		kinds: map[Kind]bool{KindImage: true},
		submit: func(s Spec) (Result, error) {
			got = s
			return Result{Status: StatusSucceeded, Assets: []Asset{{URL: "x"}}}, nil
		},
	})
	res, err := Submit(context.Background(), "recorder", Spec{Kind: KindImage, Prompt: "a cat"})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if got.Prompt != "a cat" {
		t.Errorf("spec did not reach the driver, got %+v", got)
	}
	if res.Status != StatusSucceeded || len(res.Assets) != 1 {
		t.Errorf("result not passed through, got %+v", res)
	}
}

func TestOpenRouterRejectsVideoOnlyFields(t *testing.T) {
	// Silently ignoring a start frame would hand back a plain image the user
	// did not ask for and has already paid for.
	d := openrouterDriver{}
	for name, s := range map[string]Spec{
		"start frame":  {Kind: KindImage, StartFrame: &AssetRef{URL: "u"}},
		"end frame":    {Kind: KindImage, EndFrame: &AssetRef{URL: "u"}},
		"source video": {Kind: KindImage, SourceVideo: &AssetRef{URL: "u"}},
		"wrong kind":   {Kind: KindVideo},
	} {
		if _, err := d.Submit(context.Background(), s); !errors.Is(err, ErrUnsupported) {
			t.Errorf("%s: want ErrUnsupported, got %v", name, err)
		}
	}
}

func TestSynchronousDriversHaveNoJobs(t *testing.T) {
	for _, d := range []Provider{openrouterDriver{}, pexelsDriver{}} {
		if _, err := d.Get(context.Background(), "job"); !errors.Is(err, ErrNotAsynchronous) {
			t.Errorf("%s: want ErrNotAsynchronous, got %v", d.Name(), err)
		}
	}
}

func TestSizeForAspect(t *testing.T) {
	cases := map[string]string{
		"1:1":     "1024x1024",
		"4:5":     "1024x1280",
		"16:9":    "1792x1024",
		"9:16":    "1024x1792",
		"unknown": "1024x1024",
	}
	for in, want := range cases {
		if got := SizeForAspect(in); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}

func TestOrientationForAspect(t *testing.T) {
	cases := map[string]string{
		"9:16":    "portrait",
		"4:5":     "portrait",
		"16:9":    "landscape",
		"1:1":     "square",
		"unknown": "",
	}
	for in, want := range cases {
		if got := OrientationForAspect(in); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}

// ── The licence gate ────────────────────────────────────────────────────────

// guardedProvider stands in for a stock driver whose terms forbid model input.
type guardedProvider struct {
	fakeProvider
	restricted string
}

func (g guardedProvider) RestrictedRef(url string) (bool, string) {
	if url == g.restricted {
		return true, "Stock photos can't be used as a generation reference."
	}
	return false, ""
}

func TestSubmitRefusesARestrictedReferenceInEverySlot(t *testing.T) {
	const bad = "https://images.unsplash.com/photo-1"
	Register(guardedProvider{
		fakeProvider: fakeProvider{name: "guarded-stock", kinds: map[Kind]bool{KindStock: true}},
		restricted:   bad,
	})
	Register(fakeProvider{name: "gen", kinds: map[Kind]bool{KindImage: true, KindVideo: true}})

	// Every slot that reaches a model, not just ImageRefs: a start frame and a
	// source video are model input in exactly the same sense, and the
	// restriction is about the content rather than the field it arrived in.
	specs := map[string]Spec{
		"image ref":    {Kind: KindImage, ImageRefs: []AssetRef{{URL: bad}}},
		"start frame":  {Kind: KindVideo, StartFrame: &AssetRef{URL: bad}},
		"end frame":    {Kind: KindVideo, EndFrame: &AssetRef{URL: bad}},
		"source video": {Kind: KindVideo, SourceVideo: &AssetRef{URL: bad}},
	}
	for name, s := range specs {
		_, err := Submit(context.Background(), "gen", s)
		if !errors.Is(err, ErrRestrictedRef) {
			t.Errorf("%s: want ErrRestrictedRef, got %v", name, err)
		}
		// The message is shown to a user, so it has to survive the wrapping.
		if err != nil && !strings.Contains(err.Error(), "Stock photos can't be used") {
			t.Errorf("%s: the reason did not reach the caller: %v", name, err)
		}
	}
}

func TestSubmitAllowsAnUnrestrictedReference(t *testing.T) {
	Register(guardedProvider{
		fakeProvider: fakeProvider{name: "guarded-stock2", kinds: map[Kind]bool{KindStock: true}},
		restricted:   "https://images.unsplash.com/photo-9",
	})
	Register(fakeProvider{name: "gen2", kinds: map[Kind]bool{KindImage: true}})

	_, err := Submit(context.Background(), "gen2", Spec{
		Kind:      KindImage,
		ImageRefs: []AssetRef{{URL: "https://cdn.example.com/my-own-upload.png"}},
	})
	if err != nil {
		t.Errorf("a user's own upload must still be usable as a reference: %v", err)
	}
}

func TestRestrictedRefIgnoresEmptyInput(t *testing.T) {
	if bad, _ := RestrictedRef(""); bad {
		t.Error("an empty url is not a restricted reference")
	}
	if bad, _ := RestrictedRefs(nil); bad {
		t.Error("no references is not a restricted reference")
	}
}

// ── Usage reporting ─────────────────────────────────────────────────────────

type reportingProvider struct {
	fakeProvider
	reported *[]string
}

func (r reportingProvider) ReportUsed(_ context.Context, a Asset) error {
	*r.reported = append(*r.reported, a.TrackingRef)
	return nil
}

func TestReportUsedOnlyFiresForAnAssetThatCarriesAHandle(t *testing.T) {
	var reported []string
	Register(reportingProvider{
		fakeProvider: fakeProvider{name: "reporter", kinds: map[Kind]bool{KindStock: true}},
		reported:     &reported,
	})

	// Searching is not using. An asset with no tracking handle must not report.
	if err := ReportUsed(context.Background(), "reporter", Asset{URL: "x"}); err != nil {
		t.Errorf("no handle should be a silent no-op, got %v", err)
	}
	if len(reported) != 0 {
		t.Fatalf("reported %v with no handle", reported)
	}

	if err := ReportUsed(context.Background(), "reporter", Asset{URL: "x", TrackingRef: "loc-1"}); err != nil {
		t.Errorf("report: %v", err)
	}
	if len(reported) != 1 || reported[0] != "loc-1" {
		t.Errorf("reported %v, want [loc-1]", reported)
	}
}

func TestReportUsedIsANoOpForAProviderThatDoesNotReport(t *testing.T) {
	Register(fakeProvider{name: "silent", kinds: map[Kind]bool{KindStock: true}})
	if err := ReportUsed(context.Background(), "silent", Asset{TrackingRef: "loc"}); err != nil {
		t.Errorf("want a no-op, got %v", err)
	}
}

// ── Provider selection ──────────────────────────────────────────────────────

func TestStockProviderPrefersExplicitConfiguration(t *testing.T) {
	t.Setenv("STOCK_PROVIDER", "Pexels")
	t.Setenv("UNSPLASH_ACCESS_KEY", "a-key")
	if got := StockProvider(); got != "pexels" {
		t.Errorf("explicit config should win and normalise, got %q", got)
	}
}

func TestStockProviderFallsBackToWhicheverHoldsAKey(t *testing.T) {
	t.Setenv("STOCK_PROVIDER", "")

	t.Setenv("UNSPLASH_ACCESS_KEY", "a-key")
	if got := StockProvider(); got != "unsplash" {
		t.Errorf("with an Unsplash key and no config, got %q", got)
	}

	// Pexels paused new key issuance, so this is the state a stage lands in
	// when nobody has provisioned anything — it must still name a registered
	// driver rather than an empty string.
	t.Setenv("UNSPLASH_ACCESS_KEY", "")
	if got := StockProvider(); got != "pexels" {
		t.Errorf("with no keys at all, got %q", got)
	}
}

// Both drivers stay registered: "paused" is not "gone", and a reopened Pexels
// key should be a variable rather than a rewrite.
func TestBothStockDriversStayRegistered(t *testing.T) {
	for _, name := range []string{"pexels", "unsplash"} {
		p, ok := ByName(name)
		if !ok {
			t.Fatalf("%s is not registered; have %v", name, Registered())
		}
		if !p.Supports(KindStock) {
			t.Errorf("%s does not serve stock", name)
		}
	}
}

func TestUnsplashAssetsDeclareTheirLicence(t *testing.T) {
	d := unsplashDriver{}
	bad, why := d.RestrictedRef("https://images.unsplash.com/photo-1")
	if !bad {
		t.Fatal("an Unsplash photo must be refused as a generation reference")
	}
	// The message is for a user, so it must say what to do instead.
	if !strings.Contains(why, "upload your own") {
		t.Errorf("the reason should offer a way forward, got %q", why)
	}
	if bad, _ := d.RestrictedRef("https://cdn.example.com/mine.png"); bad {
		t.Error("a user's own image must not be refused")
	}
}

func TestUnsplashOrientationUsesItsOwnVocabulary(t *testing.T) {
	// "squarish", not Pexels' "square". The wrong word is accepted silently and
	// returns unfiltered results, which reads as "orientation is broken".
	cases := map[string]string{"1:1": "squarish", "9:16": "portrait", "4:5": "portrait", "16:9": "landscape", "": ""}
	for in, want := range cases {
		if got := orientationFor(in); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}

func TestUnsplashCreditNamesBothThePhotographerAndUnsplash(t *testing.T) {
	if got := creditLine("Ada"); got != "Photo by Ada on Unsplash" {
		t.Errorf("got %q", got)
	}
	// Unsplash must still be named when the photographer is not.
	if got := creditLine("  "); got != "Photo on Unsplash" {
		t.Errorf("got %q", got)
	}
}
