package mediagen

import (
	"context"
	"fmt"

	"github.com/idivarts/backend-sls/pkg/pexels"
)

// pexelsDriver serves stock photography.
//
// Search is not generation, and modelling it as a different kind of thing was
// tempting. It sits behind the same seam because of how it is USED: the caller
// asks for "a photograph of X" and does not, at the call site, care whether the
// answer is found or made. Which of the two is cheaper, faster or better for a
// given request changes over time, and that should be a configuration decision
// rather than a rewrite.
type pexelsDriver struct{}

func init() { Register(pexelsDriver{}) }

func (pexelsDriver) Name() string { return "pexels" }

func (pexelsDriver) Supports(k Kind) bool { return k == KindStock }

func (pexelsDriver) Submit(ctx context.Context, s Spec) (Result, error) {
	if s.Kind != KindStock {
		return Result{}, fmt.Errorf("%w: pexels serves stock search only", ErrUnsupported)
	}
	count := s.Count
	if count < 1 {
		count = 1
	}

	photos, err := pexels.Search(ctx, s.Prompt, OrientationForAspect(s.AspectRatio), count)
	if err != nil {
		// A missing key is a configuration gap, not a failure of this request.
		// It is reported as a failed Result rather than an error so a caller
		// filling several placeholders does not abandon the others.
		if err == pexels.ErrNotConfigured {
			return Result{Status: StatusFailed, Error: "stock photography is not configured"}, nil
		}
		return Result{Status: StatusFailed, Error: err.Error()}, err
	}

	out := Result{Status: StatusSucceeded}
	for _, p := range photos {
		a := Asset{URL: p.URL, MimeType: "image/jpeg", SourceURL: p.PageURL}
		if p.Photographer != "" {
			a.Attribution = "Photo by " + p.Photographer + " on Pexels"
		}
		out.Assets = append(out.Assets, a)
	}
	if len(out.Assets) == 0 {
		out.Status = StatusFailed
		out.Error = "no photographs matched"
	}
	return out, nil
}

func (pexelsDriver) Get(context.Context, string) (Result, error) {
	return Result{}, ErrNotAsynchronous
}

// OrientationForAspect maps a ratio to a stock-search orientation filter.
// An unknown ratio searches unfiltered rather than guessing portrait.
func OrientationForAspect(ratio string) string {
	switch ratio {
	case "9:16", "4:5":
		return pexels.OrientationPortrait
	case "16:9":
		return pexels.OrientationLandscape
	case "1:1":
		return pexels.OrientationSquare
	}
	return ""
}
