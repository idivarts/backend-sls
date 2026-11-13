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
	"net"
	"net/http"
	"net/url"
	"path"
	"regexp"
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

// maxKitBytes bounds what the WHOLE kit may add to a project.
//
// hfproject.Build refuses a project over 4 MiB in total, and it refuses it
// AFTER the model has been called and charged. A brand with six font files
// could walk a design into that failure with a message about a limit they
// never set. So the kit stops adding assets at its own, smaller budget and
// the film goes out with the fonts that fit.
const maxKitBytes = 1 << 20

// hexRe is what may reach a stylesheet as a colour.
//
// The design system is written client-side straight to Firestore, so nothing
// has validated these values before they get here. Interpolated unchecked
// into a <style> block, a value like `red}</style><script>…` escapes the
// element — and the document it escapes into is published to a CDN origin
// shared by every brand's projects. Three, four, six or eight hex digits
// covers #RGB through #RRGGBBAA; anything else is not a colour.
var hexRe = regexp.MustCompile(`^#?[0-9A-Fa-f]{3,8}$`)

// familyRe is the same argument for a font family: letters, digits, spaces
// and hyphens are a typeface name, and a quote or a brace is an escape.
var familyRe = regexp.MustCompile(`^[A-Za-z0-9 \-]{1,64}$`)

// weightRe keeps font-weight to what CSS accepts there.
var weightRe = regexp.MustCompile(`^([1-9]00|normal|bold|lighter|bolder)$`)

// fallbackRe allows a short generic stack — "sans-serif", "Georgia, serif" —
// and nothing with a brace, a quote or a semicolon in it.
var fallbackRe = regexp.MustCompile(`^[A-Za-z0-9 \-,]{1,64}$`)

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
		hex := safeHex(c.Hex)
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
	budget := maxKitBytes
	for _, f := range ds.Fonts {
		role, family := sanitiseToken(f.Role), safeFamily(f.Family)
		if role == "" {
			continue
		}
		fallback := fallbackOr(f.Fallback)
		if family == "" {
			vars = append(vars, fmt.Sprintf("--brand-font-%s:%s", role, fallback))
			continue
		}
		weight := safeWeight(f.Weight)

		// ── A family is only named once it is DECLARED ────────────────────
		// `check` refuses a family it cannot resolve — "used without
		// @font-face declaration … not in the auto-resolved font list" —
		// and that is a hard failure, so a brand whose heading face is not
		// one of the handful the renderer knows could not author at all.
		// Found on dev with Space Grotesk, which is a Google family and
		// still not auto-resolved.
		//
		// So every family we put in a stack gets a real @font-face against
		// a real file, and a family we could not fetch is left out of its
		// own token rather than breaking the gate. The token itself is
		// always defined: the prompt lists these names and a var() pointing
		// at nothing is an invalid declaration, not a fallback.
		src, ext := strings.TrimSpace(f.URL), ""
		if src != "" {
			ext = fontExt(src)
		} else if g, gext, found := googleFontFile(ctx, family, weight); found {
			src, ext = g, gext
		}
		if src == "" {
			vars = append(vars, fmt.Sprintf("--brand-font-%s:%s", role, fallback))
			continue
		}

		// The WEIGHT is part of the name. Keyed on the family alone, a brand
		// using one family at two weights wrote one file and dropped the
		// other, and every weight then rendered from whichever face won.
		name := fmt.Sprintf("assets/fonts/%s-%s%s", slug(family), weightSlug(weight), ext)
		if !hasFile(files, name) {
			body, ok := fetchAsset(ctx, src)
			if !ok || len(body) > budget {
				vars = append(vars, fmt.Sprintf("--brand-font-%s:%s", role, fallback))
				continue
			}
			budget -= len(body)
			files = append(files, hfproject.File{Path: name, Content: string(body)})
			faces = append(faces, fmt.Sprintf(
				"@font-face{font-family:'%s';src:url('%s');font-display:block;%s}",
				family, name, weightDecl(weight)))
		}
		vars = append(vars, fmt.Sprintf("--brand-font-%s:'%s',%s", role, family, fallback))
	}

	// ── Logo ──────────────────────────────────────────────────────────────
	// The primary variant wins; otherwise the first one with a URL. Exposed
	// both as a variable for CSS backgrounds and as a path the prompt names,
	// because an <img> is what a model reaches for and var() is not valid
	// in a src attribute.
	logoPath := ""
	if u := pickLogo(ds.Logos); u != "" {
		name := "assets/brand-logo" + imageExt(u)
		if body, ok := fetchAsset(ctx, u); ok && len(body) <= budget {
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

// assetClient re-checks the destination on every redirect.
//
// Checking only the URL we were given is not enough: a public host is free
// to answer with a 302 to 169.254.169.254, and the client would follow it.
var assetClient = &http.Client{
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("too many redirects")
		}
		if !assetURLAllowed(req.URL.String()) {
			return fmt.Errorf("redirect to a non-public address")
		}
		return nil
	},
}

// assetURLAllowed is publicHTTPS, behind a variable so the tests can serve
// an asset from a loopback httptest server — which the real guard refuses,
// and should. Nothing else reassigns it.
var assetURLAllowed = publicHTTPS

// publicHTTPS says whether a URL is one this Lambda may fetch.
//
// ── Why this is not just a scheme check ───────────────────────────────────
// The URL comes from a design-system document the brand writes itself, and
// this runs inside a Lambda with an execution role. Fetching whatever it is
// handed makes the authoring job a proxy into our own network: the metadata
// address, anything in the VPC, a localhost control port. Worse, the body is
// then written into the project and published to a public CDN, so the reply
// is readable by anyone with the design's link.
//
// So: https only, and the host must resolve to addresses that are ALL public
// unicast. Every resolved address is checked, because a hostname under the
// caller's control can answer with one public and one private address.
func publicHTTPS(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" {
		return false
	}
	ips, err := net.LookupIP(u.Hostname())
	if err != nil || len(ips) == 0 {
		return false
	}
	for _, ip := range ips {
		if !ip.IsGlobalUnicast() ||
			ip.IsPrivate() ||
			ip.IsLoopback() ||
			ip.IsLinkLocalUnicast() ||
			ip.IsLinkLocalMulticast() ||
			ip.IsInterfaceLocalMulticast() {
			return false
		}
	}
	return true
}

// fetchAsset gets the bytes, or reports that it could not.
//
// Every failure is non-fatal by design: the film is still worth making
// without the logo. The caller simply omits what did not arrive.
func fetchAsset(ctx context.Context, raw string) ([]byte, bool) {
	if !assetURLAllowed(raw) {
		return nil, false
	}
	ctx, cancel := context.WithTimeout(ctx, assetFetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, false
	}
	resp, err := assetClient.Do(req)
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

// googleFontsCSS is the stylesheet endpoint, behind a variable so a test can
// point it at a local server.
var googleFontsCSS = "https://fonts.googleapis.com/css2"

// woff2Re pulls the file URLs out of that stylesheet.
//
// Any https host, not gstatic specifically: the URL is fetched through the
// same guard as every other asset, so pinning the host here would add no
// safety and would make the resolver impossible to test without reaching
// the real endpoint.
var woff2Re = regexp.MustCompile(`url\((https?://[^)\s]+\.woff2)\)`)

// googleFontFile finds a real file for a family the brand named but gave no
// URL for.
//
// ── Why ask Google rather than trust the renderer ─────────────────────────
// The renderer resolves a SHORT list of families by itself. Everything else
// fails `check` outright, so "it is a Google font, it will be fine" is not
// true: Space Grotesk is a Google font and is not on that list. Asking for
// the stylesheet gives us a real file for any family they publish, which is
// most of what a brand will name, and it keeps the render air-gapped because
// the bytes travel with the project.
//
// The browser User-Agent is not decoration: without it Google serves ttf,
// which is several times the size for the same glyphs.
func googleFontFile(ctx context.Context, family, weight string) (string, string, bool) {
	q := url.Values{}
	spec := strings.ReplaceAll(family, " ", "+")
	if weight != "" && weight != "normal" && weight != "bold" {
		spec += ":wght@" + weight
	}
	q.Set("display", "block")
	// family carries a '+' that must not be escaped into %2B, so it is
	// appended rather than passed through Values.Encode.
	endpoint := fmt.Sprintf("%s?family=%s&%s", googleFontsCSS, spec, q.Encode())

	ctx, cancel := context.WithTimeout(ctx, assetFetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", "", false
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) "+
		"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	resp, err := assetClient.Do(req)
	if err != nil {
		return "", "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", false
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 256<<10))
	if err != nil {
		return "", "", false
	}
	m := woff2Re.FindAllStringSubmatch(string(body), -1)
	if len(m) == 0 {
		return "", "", false
	}
	// Google emits one @font-face per subset and puts latin last, so the
	// final match is the one an English-language film actually needs.
	return m[len(m)-1][1], ".woff2", true
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
	if f := strings.TrimSpace(fb); fallbackRe.MatchString(f) {
		return f
	}
	return "sans-serif"
}

// safeHex returns the colour, or "" if the value is not one.
func safeHex(h string) string {
	h = strings.TrimSpace(h)
	if !hexRe.MatchString(h) {
		return ""
	}
	if !strings.HasPrefix(h, "#") {
		h = "#" + h
	}
	return h
}

func safeFamily(f string) string {
	f = strings.TrimSpace(f)
	if !familyRe.MatchString(f) {
		return ""
	}
	return f
}

func safeWeight(w string) string {
	w = strings.ToLower(strings.TrimSpace(w))
	if !weightRe.MatchString(w) {
		return ""
	}
	return w
}

func weightSlug(w string) string {
	if w == "" {
		return "regular"
	}
	return w
}

func weightDecl(w string) string {
	if w != "" {
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
