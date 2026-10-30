package mediagen

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/idivarts/backend-sls/pkg/unsplash"
)

// unsplashDriver serves stock photography, beside the Pexels driver rather than
// instead of it.
//
// Pexels paused new API key issuance around 30 September 2026, which left a
// correct, licensing-clean implementation stranded behind a key that can no
// longer be self-served. Both stay registered: "paused" is not "gone", and if
// keys reopen the switch back is a variable. This is the first time the seam
// pays for itself — the stock provider changed and nothing above this package
// had to know.
//
// What is NOT shared with the Pexels driver is the licence handling, because
// the two licences genuinely differ. These assets are hotlinked rather than
// copied, report their own use, and refuse to be fed to a model.
type unsplashDriver struct{}

func init() { Register(unsplashDriver{}) }

func (unsplashDriver) Name() string { return "unsplash" }

func (unsplashDriver) Supports(k Kind) bool { return k == KindStock }

func (unsplashDriver) Submit(ctx context.Context, s Spec) (Result, error) {
	if s.Kind != KindStock {
		return Result{}, fmt.Errorf("%w: unsplash serves stock search only", ErrUnsupported)
	}
	count := s.Count
	if count < 1 {
		count = 1
	}

	photos, err := unsplash.Search(ctx, s.Prompt, orientationFor(s.AspectRatio), count)
	if err != nil {
		// A missing key is a configuration gap, not a failure of this request.
		// Reported as a failed Result rather than an error so a caller filling
		// several placeholders does not abandon the others.
		if err == unsplash.ErrNotConfigured {
			return Result{Status: StatusFailed, Error: "stock photography is not configured"}, nil
		}
		return Result{Status: StatusFailed, Error: err.Error()}, err
	}

	out := Result{Status: StatusSucceeded}
	for _, p := range photos {
		out.Assets = append(out.Assets, Asset{
			URL:      p.URL,
			MimeType: "image/jpeg",
			Alt:      p.Alt,
			// Required by the API terms rather than chosen: the app must use
			// the URL it was given, and copying the bytes is a material breach.
			Hotlink: true,
			// §12 routes any use "in connection with ... artificial
			// intelligence purposes" to a separate agreement we do not hold.
			NoAIInput:   true,
			Attribution: creditLine(p.Photographer),
			CreatorURL:  p.PhotographerURL,
			SourceURL:   p.PageURL,
			// Reported only if this photo is actually placed; see ReportUsed.
			TrackingRef: p.DownloadLocation,
		})
	}
	if len(out.Assets) == 0 {
		out.Status = StatusFailed
		out.Error = "no photographs matched"
	}
	return out, nil
}

func (unsplashDriver) Get(context.Context, string) (Result, error) {
	return Result{}, ErrNotAsynchronous
}

// ReportUsed fires the mandatory download ping. Guideline 2: an app must
// request a photo's download_location when it does "something similar to a
// download", which placing one into a design is.
func (unsplashDriver) ReportUsed(ctx context.Context, a Asset) error {
	return unsplash.TrackDownload(ctx, a.TrackingRef)
}

// RestrictedRef refuses an Unsplash photo as a generation reference.
//
// The message is shown to a user, so it says what they can do instead rather
// than citing a clause at them.
func (unsplashDriver) RestrictedRef(url string) (bool, string) {
	if !unsplash.IsStockURL(url) {
		return false, ""
	}
	return true, "Stock photos can't be used as a generation reference — upload your own image, or use one you generated."
}

// creditLine is the attribution text. Unsplash requires crediting BOTH the
// photographer and Unsplash itself, so neither half is optional.
func creditLine(photographer string) string {
	photographer = strings.TrimSpace(photographer)
	if photographer == "" {
		return "Photo on Unsplash"
	}
	return "Photo by " + photographer + " on Unsplash"
}

// orientationFor maps a ratio to Unsplash's own orientation vocabulary, which
// is not Pexels' — "squarish", not "square". An unknown ratio searches
// unfiltered rather than guessing portrait.
func orientationFor(ratio string) string {
	switch ratio {
	case "9:16", "4:5":
		return unsplash.OrientationPortrait
	case "16:9":
		return unsplash.OrientationLandscape
	case "1:1":
		return unsplash.OrientationSquare
	}
	return ""
}

// StockProvider names the driver that should serve stock search right now.
//
// Explicit configuration wins; otherwise the first provider that actually holds
// a key does. The fallback is what stops a half-configured stage from silently
// having no stock photography at all — which is exactly the state Phase 4
// shipped into when Pexels stopped issuing keys, and it took a while to notice
// because nothing said so.
func StockProvider() string {
	if v := strings.TrimSpace(strings.ToLower(os.Getenv("STOCK_PROVIDER"))); v != "" {
		return v
	}
	if unsplash.IsConfigured() {
		return "unsplash"
	}
	return "pexels"
}
