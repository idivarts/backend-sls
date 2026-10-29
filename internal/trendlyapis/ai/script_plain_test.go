package ai

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The property that matters most: plain markdown is returned byte-identical.
// The Phase 1 corpus and every check built against it are only meaningful if
// the HTML path cannot perturb the markdown path.
func TestPlainScriptLeavesMarkdownAlone(t *testing.T) {
	for _, dir := range []string{"clean", "flagged"} {
		paths, err := filepath.Glob(filepath.Join("testdata", "devscripts", dir, "*.md"))
		if err != nil {
			t.Fatalf("glob %s: %v", dir, err)
		}
		if len(paths) == 0 {
			t.Fatalf("no fixtures in %s", dir)
		}
		for _, p := range paths {
			b, err := os.ReadFile(p)
			if err != nil {
				t.Fatalf("read %s: %v", p, err)
			}
			if got := plainScript(string(b)); got != string(b) {
				t.Errorf("%s: markdown was rewritten\n--- before ---\n%s\n--- after ---\n%s",
					filepath.Base(p), string(b), got)
			}
		}
	}
}

var (
	reMDBold    = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	reMDItalic  = regexp.MustCompile(`\*([^*]+)\*`)
	reMDHeading = regexp.MustCompile(`^(#{1,6})\s+(.*)$`)
)

// markdownToHTML is the kind of markup the RichTextEditor produces: block tags
// per line, <strong>/<em> for emphasis, <li> for bullets. Deliberately naive —
// it exists to prove the two spellings reach the same verdict, not to be a
// markdown implementation.
func markdownToHTML(md string) string {
	var sb strings.Builder
	var inList bool
	closeList := func() {
		if inList {
			sb.WriteString("</ul>")
			inList = false
		}
	}
	for _, raw := range strings.Split(md, "\n") {
		line := strings.TrimRight(raw, " \t")
		inline := func(s string) string {
			s = reMDBold.ReplaceAllString(s, "<strong>$1</strong>")
			return reMDItalic.ReplaceAllString(s, "<em>$1</em>")
		}
		switch {
		case strings.TrimSpace(line) == "":
			closeList()
			sb.WriteString("<p></p>")
		case reMDHeading.MatchString(line):
			closeList()
			m := reMDHeading.FindStringSubmatch(line)
			n := len(m[1])
			sb.WriteString("<h" + string(rune('0'+n)) + ">" + inline(m[2]) + "</h" + string(rune('0'+n)) + ">")
		case strings.HasPrefix(strings.TrimSpace(line), "- "):
			if !inList {
				sb.WriteString("<ul>")
				inList = true
			}
			sb.WriteString("<li>" + inline(strings.TrimPrefix(strings.TrimSpace(line), "- ")) + "</li>")
		default:
			closeList()
			sb.WriteString("<p>" + inline(line) + "</p>")
		}
	}
	closeList()
	return sb.String()
}

// The regression this file exists for: the same script, typed in the editor
// rather than streamed from the model, must produce the same problems. Before
// plainScript it produced a different set on every file — the labels the
// validator keys on were wrapped in tags it had never seen.
func TestHTMLAndMarkdownAgreeOnTheCorpus(t *testing.T) {
	cases := []struct {
		dir    string
		format string
	}{{"clean", "reel"}, {"flagged", "reel"}}
	for _, c := range cases {
		paths, _ := filepath.Glob(filepath.Join("testdata", "devscripts", c.dir, "*.md"))
		for _, p := range paths {
			b, err := os.ReadFile(p)
			if err != nil {
				t.Fatalf("read %s: %v", p, err)
			}
			md := string(b)
			wantIDs := strings.Join(problemIDs(ValidateScript(md, c.format)), ",")
			gotIDs := strings.Join(problemIDs(ValidateScript(markdownToHTML(md), c.format)), ",")
			if wantIDs != gotIDs {
				t.Errorf("%s: markdown gave [%s], the same script as HTML gave [%s]",
					filepath.Base(p), wantIDs, gotIDs)
			}
		}
	}
}

func TestPlainScriptMapping(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{
			name: "emphasis becomes the markdown the validator reads",
			in:   "<p><strong>VO:</strong> &quot;Here&#39;s why.&quot;</p>",
			want: `**VO:** "Here's why."`,
		},
		{
			name: "heading keeps its level",
			in:   "<h2>Hook</h2><p><strong>VO:</strong> Line.</p>",
			want: "## Hook\n\n**VO:** Line.",
		},
		{
			name: "list items become bullets",
			in:   "<ul><li>First beat</li><li>Second beat</li></ul>",
			want: "- First beat\n- Second beat",
		},
		{
			name: "br is a line break, not a new block",
			in:   "<p>One<br>Two</p>",
			want: "One\nTwo",
		},
		{
			name: "empty paragraphs collapse to one blank line",
			in:   "<p>One</p><p></p><p></p><p>Two</p>",
			want: "One\n\nTwo",
		},
		{
			name: "spans and attributes are dropped, their text kept",
			in:   `<p class="x"><span style="color:red">On-screen text:</span> Wait</p>`,
			want: "On-screen text: Wait",
		},
		{
			name: "an escaped tag in the copy survives as text",
			in:   "<p>Use &lt;b&gt; for bold</p>",
			want: "Use <b> for bold",
		},
		{
			name: "comments are removed",
			in:   "<p>Keep</p><!-- drop this --><p>This</p>",
			want: "Keep\n\nThis",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := plainScript(c.in); got != c.want {
				t.Errorf("plainScript(%q)\n got %q\nwant %q", c.in, got, c.want)
			}
		})
	}
}

// Running it twice must not change the result: the HTTP scorer converts, and
// then ValidateScript converts again on the way to the caps.
func TestPlainScriptIsIdempotent(t *testing.T) {
	in := markdownToHTML(goodScript)
	once := plainScript(in)
	if twice := plainScript(once); twice != once {
		t.Errorf("second pass changed the output\n once: %q\ntwice: %q", once, twice)
	}
}
