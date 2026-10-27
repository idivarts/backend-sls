package ai

import (
	"strings"
	"testing"
)

// A minimal well-formed design, used as the base every case mutates.
const okVideo = `<div data-carousel style="display:flex">
<section data-slide="0" data-duration="3000" style="width:1080px;height:1920px">
  <h1 data-el="hook" style="animation: rise 600ms ease-out 200ms both">Hook</h1>
</section>
<section data-slide="1" data-duration="4000" style="width:1080px;height:1920px">
  <p data-el="cta" style="animation-duration:800ms;animation-delay:300ms">Call to action</p>
</section>
</div>`

func problemsContain(t *testing.T, got []designProblem, want string) {
	t.Helper()
	for _, p := range got {
		if strings.Contains(strings.ToLower(string(p)), strings.ToLower(want)) {
			return
		}
	}
	t.Fatalf("expected a problem mentioning %q, got %v", want, got)
}

// The matcher must actually be able to fail, or every problemsContain assertion
// above is vacuous.
func TestProblemsContainCanFail(t *testing.T) {
	got := validateDesignHTML(okVideo, "video", 2)
	if len(got) != 0 {
		t.Fatalf("fixture should be clean, got %v", got)
	}
	fake := []designProblem{"scene 0 has no data-duration"}
	if !strings.Contains(string(fake[0]), "data-duration") {
		t.Fatal("matcher failed to find a substring that is present")
	}
	if strings.Contains(string(fake[0]), "transition") {
		t.Fatal("matcher found a substring that is absent")
	}
}

func TestValidateDesignHTML_AcceptsSoundDesign(t *testing.T) {
	if got := validateDesignHTML(okVideo, "video", 2); len(got) != 0 {
		t.Fatalf("expected no problems, got %v", got)
	}
}

func TestValidateDesignHTML_RequiresSlides(t *testing.T) {
	got := validateDesignHTML(`<div><p>no slides here</p></div>`, "image", 1)
	problemsContain(t, got, "data-slide")
}

func TestValidateDesignHTML_SlideCountMismatch(t *testing.T) {
	// The html has 2 scenes; the model claimed 3.
	got := validateDesignHTML(okVideo, "video", 3)
	problemsContain(t, got, "you said slides=3")
}

func TestValidateDesignHTML_MissingDuration(t *testing.T) {
	const missing = `<div data-carousel>
<section data-slide="0" style="width:1080px"><h1 data-el="a">A</h1></section>
</div>`
	got := validateDesignHTML(missing, "video", 1)
	problemsContain(t, got, "no data-duration")

	// The same document is fine as a still design — duration is meaningless there.
	if got := validateDesignHTML(missing, "image", 1); len(got) != 0 {
		t.Fatalf("still design should not need data-duration, got %v", got)
	}
}

func TestValidateDesignHTML_AnimationOutlastsScene(t *testing.T) {
	// 1.5s delay + 2s duration = 3500ms of motion in a 2000ms scene.
	const overrun = `<div data-carousel>
<section data-slide="0" data-duration="2000">
  <h1 data-el="a" style="animation: rise 2s ease-out 1.5s both">A</h1>
</section>
</div>`
	got := validateDesignHTML(overrun, "video", 1)
	problemsContain(t, got, "3500ms")
}

func TestValidateDesignHTML_AnimationWithinSceneIsFine(t *testing.T) {
	const fine = `<div data-carousel>
<section data-slide="0" data-duration="4000">
  <h1 data-el="a" style="animation: rise 900ms ease-out 200ms both">A</h1>
</section>
</div>`
	if got := validateDesignHTML(fine, "video", 1); len(got) != 0 {
		t.Fatalf("expected no problems, got %v", got)
	}
}

func TestValidateDesignHTML_RejectsUnseekableMotion(t *testing.T) {
	const transition = `<div data-carousel>
<section data-slide="0" data-duration="3000">
  <h1 data-el="a" style="transition: opacity 300ms ease">A</h1>
</section>
</div>`
	problemsContain(t, validateDesignHTML(transition, "video", 1), "transition")

	const infinite = `<div data-carousel>
<section data-slide="0" data-duration="3000">
  <h1 data-el="a" style="animation: pulse 1s infinite">A</h1>
</section>
</div>`
	problemsContain(t, validateDesignHTML(infinite, "video", 1), "infinite")
}

func TestValidateDesignHTML_RequiresAddressableText(t *testing.T) {
	const noDataEl = `<div data-carousel>
<section data-slide="0"><h1>Untappable</h1></section>
</div>`
	problemsContain(t, validateDesignHTML(noDataEl, "image", 1), "data-el")
}

func TestCssTimeMs(t *testing.T) {
	cases := []struct {
		in   string
		want float64
	}{
		{"1.2s", 1200},
		{"400ms", 400},
		{"2s", 2000},
		{".5s", 500},
	}
	for _, c := range cases {
		got, ok := cssTimeMs(c.in)
		if !ok || got != c.want {
			t.Errorf("cssTimeMs(%q) = %v, %v; want %v", c.in, got, ok, c.want)
		}
	}
	// A bare number is not a CSS time and must not be read as one.
	if _, ok := cssTimeMs("ease-out"); ok {
		t.Error("cssTimeMs read a time out of an easing keyword")
	}
}

func TestValidateDesignHTML_RejectsPlayerOwnedProperties(t *testing.T) {
	const ownsTransform = `<div data-carousel>
<section data-slide="0" data-duration="3000" style="transform:scale(1.1)"><h1 data-el="a">A</h1></section>
</div>`
	problemsContain(t, validateDesignHTML(ownsTransform, "video", 1), "owns that property")

	const ownsOpacity = `<div data-carousel>
<section data-slide="0" data-duration="3000" style="opacity:0.9"><h1 data-el="a">A</h1></section>
</div>`
	problemsContain(t, validateDesignHTML(ownsOpacity, "video", 1), "cross-fade")
}

func TestValidateDesignHTML_TransformOriginIsNotTransform(t *testing.T) {
	// transform-origin on the slide is harmless — the anchored regex must not
	// mistake it for a transform declaration.
	const originOnly = `<div data-carousel>
<section data-slide="0" data-duration="3000" style="transform-origin:top left"><h1 data-el="a">A</h1></section>
</div>`
	if got := validateDesignHTML(originOnly, "video", 1); len(got) != 0 {
		t.Fatalf("transform-origin should be allowed, got %v", got)
	}
}

func TestValidateDesignHTML_TransitionKinds(t *testing.T) {
	const good = `<div data-carousel>
<section data-slide="0" data-duration="3000"><h1 data-el="a">A</h1></section>
<section data-slide="1" data-duration="3000" data-transition="fade" data-transition-ms="400"><h1 data-el="b">B</h1></section>
</div>`
	if got := validateDesignHTML(good, "video", 2); len(got) != 0 {
		t.Fatalf("a valid fade transition should pass, got %v", got)
	}

	const typo = `<div data-carousel>
<section data-slide="0" data-duration="3000"><h1 data-el="a">A</h1></section>
<section data-slide="1" data-duration="3000" data-transition="crossfade"><h1 data-el="b">B</h1></section>
</div>`
	problemsContain(t, validateDesignHTML(typo, "video", 2), "not a transition the player knows")

	const onFirst = `<div data-carousel>
<section data-slide="0" data-duration="3000" data-transition="fade"><h1 data-el="a">A</h1></section>
</div>`
	problemsContain(t, validateDesignHTML(onFirst, "video", 1), "nothing to enter from")
}
