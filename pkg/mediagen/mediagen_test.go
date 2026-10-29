package mediagen

import (
	"context"
	"errors"
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
