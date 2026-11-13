package design

// Putting the brand on the film.
//
// ── What this translates, and into what ───────────────────────────────────
// There is no Hyperframes brand-kit object to populate. `.hyperframes/
// brand-kit.json` appears once in their whole package, in a path policy, with
// nothing reading it. Branding reaches a Hyperframes film through the project
// itself: root CSS variables, @font-face with real font files, real asset
// files, and what the authoring model is told. So that is what this builds.
//
// ── Why the variables are injected rather than requested ──────────────────
// The model is told the variables exist and asked to use them. It is NOT
// trusted to emit them: a film whose palette depends on the model remembering
// to write a :root block is a film that is off-brand on the run where it
// forgets, and nobody would know which run that was. The block is injected
// into the authored document afterwards, so it is there whatever the model
// did. If the model also wrote one, ours lands first and theirs wins on
// equal specificity — which is the right way round, because a model that
// wrote a considered palette for a scene should not be overridden by tokens.
//
// ── Why assets are fetched at authoring time ──────────────────────────────
// A render must work with no network: that is proved under `--network none`
// and it is why GSAP is vendored. A logo referenced by URL would break that
// promise quietly — the render would succeed everywhere it was tested and
// fail where it mattered. So the bytes are fetched HERE, in a Lambda that
// does have network, and travel with the project as files.

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/hfproject"
)

// maxAssetBytes bounds one fetched asset.
//
// The project as a whole is capped at 4 MiB and a film needs room for its own
// markup, so an 8 MB hero logo is refused rather than allowed to crowd out
// the document that references it. 512 KiB is generous for a logo or a woff2.
const maxAssetBytes = 512 << 10

// assetFetchTimeout is per asset, not for the set.
//
// Deliberately short. These are nice-to-haves on the authoring path: a brand
// whose font host is slow should get a film with a fallback face, not an
// authoring run that times out and charges them for nothing.
const assetFetchTimeout = 8 * time.Second

// brandKit is what a brand's design system becomes, in project terms.
type brandKit struct {
	// Prompt is appended to the authoring system prompt.
	Prompt string
	// CSS is a <style> block injected into the authored document's head.
	CSS string
	// Files are assets fetched at authoring time, at their project paths.
	Files []hfproject.File
}

// buildBrandKit turns a brand's design system into its three project pieces.
//
// Returns nil when the brand has no design system, which is the common case
// today and must stay unremarkable: authoring then behaves exactly as it did
// before this existed.
func buildBrandKit(ctx context.Context, ds *trendlymodels.BrandDesignSystem) *brandKit {
	if ds == nil {
		return nil
	}

	var files []hfproject.File
	var faces []string
	var vars []string

	// ── Colours ───────────────────────────────────────────────────────────
	// By role where a role is given, and numbered as well, so a palette
	// entry with no role is still reachable rather than silently dropped.
	roleSeen := map[string]bool{}
	for i, c := range ds.Palette {
		hex := strings.TrimSpace(c.Hex)
		if hex == "" {
			continue
		}
		if role := sanitiseToken(c.Role); role != "" && !roleSeen[role] {
			vars = append(vars, fmt.Sprintf("--brand-%s:%s", role, hex))
			roleSeen[role] = true
		}
		vars = append(vars, fmt.Sprintf("--brand-color-%d:%s", i+1, hex))
	}

	// ── Fonts ─────────────────────────────────────────────────────────────
	// A family name alone is not enough for video. The renderer resolves a
	// Google family itself — it fetches the faces at compile time, which is
	// why Inter works today — but a custom URL resolves nowhere, and the
	// film silently falls back to a system face. So a font with a URL is
	// fetched and given a real @font-face against a local file.
	for _, f := range ds.Fonts {
		role, family := sanitiseToken(f.Role), strings.TrimSpace(f.Family)
		if role == "" || family == "" {
			continue
		}
		stack := fmt.Sprintf("'%s',%s", family, fallbackOr(f.Fallback))
		vars = append(vars, fmt.Sprintf("--brand-font-%s:%s", role, stack))

		if url := strings.TrimSpace(f.URL); url != "" {
			name := fmt.Sprintf("assets/fonts/%s%s", slug(family), fontExt(url))
			if body, ok := fetchAsset(ctx, url); ok && !hasFile(files, name) {
				files = append(files, hfproject.File{Path: name, Content: string(body)})
				faces = append(faces, fmt.Sprintf(
					"@font-face{font-family:'%s';src:url('%s');font-display:block;%s}",
					family, name, weightDecl(f.Weight)))
			}
		}
	}

	// ── Logo ──────────────────────────────────────────────────────────────
	// The primary variant wins; otherwise the first one with a URL. Exposed
	// both as a variable for CSS backgrounds and as a path the prompt names,
	// because an <img> is what a model reaches for and var() is not valid
	// in a src attribute.
	logoPath := ""
	if u := pickLogo(ds.Logos); u != "" {
		name := "assets/brand-logo" + imageExt(u)
		if body, ok := fetchAsset(ctx, u); ok {
			files = append(files, hfproject.File{Path: name, Content: string(body)})
			logoPath = name
			vars = append(vars, fmt.Sprintf("--brand-logo:url('%s')", name))
		}
	}

	if len(vars) == 0 && len(faces) == 0 {
		return nil
	}

	css := strings.Join(faces, "")
	if len(vars) > 0 {
		css += ":root{" + strings.Join(vars, ";") + "}"
	}

	return &brandKit{
		Prompt: brandPromptBlock(ds, vars, logoPath),
		CSS:    "<style>" + css + "</style>",
		Files:  files,
	}
}

// brandPromptBlock tells the model what it has been given and what to do with
// it. Written as instructions about tokens that ALREADY EXIST in the document,
// because that is true by the time the film renders.
func brandPromptBlock(ds *trendlymodels.BrandDesignSystem, vars []string, logoPath string) string {
	var b strings.Builder
	b.WriteString("\n\nTHIS BRAND'S KIT — the film must look like it belongs to this brand.\n")
	b.WriteString("A <style> block defining these CSS variables is already in the document's head.\n")
	b.WriteString("Use them with var(). Do not invent colours or font families that are not here.\n\n")
	for _, v := range vars {
		b.WriteString("  " + v + "\n")
	}
	if logoPath != "" {
		b.WriteString(fmt.Sprintf("\nThe brand's logo is a file in the project at %s.\n", logoPath))
		b.WriteString("Show it where a logo belongs — usually the sign-off. Reference it with a\n")
		b.WriteString("relative src, never a remote URL.\n")
	}

	if im := ds.Imagery; im != nil {
		// The LOOK and BANNED lines are how the Hyperframes skill itself
		// steers a film's look, so the brand's imagery rules are written in
		// that shape rather than as loose prose.
		if look := joinNonEmpty(", ", im.MoodKeywords...); look != "" {
			b.WriteString("\nLOOK: " + look)
			if t := strings.TrimSpace(im.ColorTreatment); t != "" {
				b.WriteString(". Treatment: " + t)
			}
			if c := strings.TrimSpace(im.Composition); c != "" {
				b.WriteString(". Composition: " + c)
			}
			b.WriteString("\n")
		}
		if banned := joinNonEmpty(", ", im.Avoid...); banned != "" {
			b.WriteString("BANNED: " + banned + "\n")
		}
	}

	if id := ds.Identity; id != nil {
		if tag := strings.TrimSpace(id.Tagline); tag != "" {
			b.WriteString("\nThe brand's tagline is " + quote(tag) + ".\n")
		}
		if p := joinNonEmpty(", ", id.Personality...); p != "" {
			b.WriteString("Its personality: " + p + ".\n")
		}
	}

	// Voice governs the WORDS, and a film is mostly words on screen. The
	// full voice block already exists for the chat path; only the parts that
	// change what appears on a frame are worth the tokens here.
	if v := ds.Voice; v != nil {
		if adj := joinNonEmpty(", ", v.Adjectives...); adj != "" {
			b.WriteString("Its voice: " + adj + ".\n")
		}
		if v.EmojiPolicy == "none" {
			b.WriteString("Use no emoji.\n")
		}
	}

	return b.String()
}

// ── helpers ───────────────────────────────────────────────────────────────

// fetchAsset gets the bytes, or reports that it could not.
//
// Every failure is non-fatal by design: the film is still worth making
// without the logo. The caller simply omits what did not arrive.
func fetchAsset(ctx context.Context, url string) ([]byte, bool) {
	if !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "http://") {
		return nil, false
	}
	ctx, cancel := context.WithTimeout(ctx, assetFetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, false
	}
	// Read one byte past the cap so an oversized asset is detected rather
	// than silently truncated into a corrupt file.
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxAssetBytes+1))
	if err != nil || len(body) == 0 || len(body) > maxAssetBytes {
		return nil, false
	}
	return body, true
}

func pickLogo(logos []trendlymodels.DSLogo) string {
	first := ""
	for _, l := range logos {
		u := strings.TrimSpace(l.URL)
		if u == "" {
			continue
		}
		if l.Variant == "primary" {
			return u
		}
		if first == "" {
			first = u
		}
	}
	return first
}

// sanitiseToken keeps a role usable as a CSS custom-property name. A role is
// brand-authored text, so it can carry spaces, case or punctuation that would
// produce a variable nothing can reference.
func sanitiseToken(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '_' || r == '-':
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

func slug(s string) string {
	if t := sanitiseToken(s); t != "" {
		return t
	}
	return "font"
}

func fallbackOr(fb string) string {
	if f := strings.TrimSpace(fb); f != "" {
		return f
	}
	return "sans-serif"
}

func weightDecl(w string) string {
	if w = strings.TrimSpace(w); w != "" {
		return "font-weight:" + w + ";"
	}
	return ""
}

// fontExt and imageExt keep the extension the URL carries when it is one we
// recognise, because the renderer and the browser both go by it.
func fontExt(u string) string {
	switch strings.ToLower(path.Ext(stripQuery(u))) {
	case ".woff2":
		return ".woff2"
	case ".woff":
		return ".woff"
	case ".ttf":
		return ".ttf"
	case ".otf":
		return ".otf"
	}
	return ".woff2"
}

func imageExt(u string) string {
	switch e := strings.ToLower(path.Ext(stripQuery(u))); e {
	case ".svg", ".png", ".jpg", ".jpeg", ".webp", ".gif":
		return e
	}
	return ".png"
}

func stripQuery(u string) string {
	if i := strings.IndexAny(u, "?#"); i >= 0 {
		return u[:i]
	}
	return u
}

func hasFile(files []hfproject.File, p string) bool {
	for _, f := range files {
		if f.Path == p {
			return true
		}
	}
	return false
}

func joinNonEmpty(sep string, parts ...string) string {
	var out []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, sep)
}

func quote(s string) string { return "\"" + strings.ReplaceAll(s, "\"", "'") + "\"" }
