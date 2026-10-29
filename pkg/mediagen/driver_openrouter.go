package mediagen

import (
	"context"
	"fmt"

	"github.com/idivarts/backend-sls/pkg/openrouter"
)

// openrouterDriver serves image generation through the same path the product
// has always used. It is the first driver deliberately: moving today's
// behaviour behind the seam with no change in what it does is what proves the
// seam fits before anything depends on it.
type openrouterDriver struct{}

func init() { Register(openrouterDriver{}) }

func (openrouterDriver) Name() string { return "openrouter" }

func (openrouterDriver) Supports(k Kind) bool { return k == KindImage }

func (d openrouterDriver) Submit(ctx context.Context, s Spec) (Result, error) {
	if s.Kind != KindImage {
		return Result{}, fmt.Errorf("%w: openrouter serves images only", ErrUnsupported)
	}
	// Keyframes and source video are video concepts. Rejecting rather than
	// ignoring them: a caller that sets a start frame and gets a plain image
	// back has been silently misunderstood, and has paid for it.
	if s.StartFrame != nil || s.EndFrame != nil || s.SourceVideo != nil {
		return Result{}, fmt.Errorf("%w: openrouter images take no keyframes or source video", ErrUnsupported)
	}

	count := s.Count
	if count < 1 {
		count = 1
	}

	refs := make([]string, 0, len(s.ImageRefs))
	for _, r := range s.ImageRefs {
		if r.URL != "" {
			refs = append(refs, r.URL)
		}
	}

	resp, usage, err := openrouter.GenerateImage(ctx, openrouter.ImageRequest{
		Model:       s.ModelID,
		Prompt:      s.Prompt,
		Size:        SizeForAspect(s.AspectRatio),
		N:           count,
		InputImages: refs,
	})
	if err != nil {
		return Result{Status: StatusFailed, Error: err.Error()}, err
	}

	out := Result{Status: StatusSucceeded}
	for _, d := range resp.Data {
		// B64 and URL are both "where the bytes are right now". The caller
		// hosts them; this driver does not, because hosting is one decision
		// that must be identical for every provider.
		switch {
		case d.URL != "":
			out.Assets = append(out.Assets, Asset{URL: d.URL, MimeType: "image/png"})
		case d.B64JSON != "":
			out.Assets = append(out.Assets, Asset{URL: "data:image/png;base64," + d.B64JSON, MimeType: "image/png"})
		}
	}
	if len(out.Assets) == 0 {
		out.Status = StatusFailed
		out.Error = "the model returned no image"
		return out, nil
	}
	out.Usage = usageFrom(usage)
	return out, nil
}

func (openrouterDriver) Get(context.Context, string) (Result, error) {
	// Images come back inside Submit, so there is never a job to poll.
	return Result{}, ErrNotAsynchronous
}

func usageFrom(u *openrouter.Usage) *Usage {
	if u == nil {
		return nil
	}
	return &Usage{CostUSD: u.Cost}
}

// SizeForAspect maps a ratio to the pixel size the image models are asked for.
//
// Here rather than in a handler because every driver needs the same answer, and
// two call sites disagreeing about what "4:5" means is how a carousel ends up
// with one slide a different shape from the rest.
func SizeForAspect(ratio string) string {
	switch ratio {
	case "1:1":
		return "1024x1024"
	case "4:5":
		return "1024x1280"
	case "16:9":
		return "1792x1024"
	case "9:16":
		return "1024x1792"
	default:
		return "1024x1024"
	}
}
