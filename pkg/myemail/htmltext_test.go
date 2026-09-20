package myemail

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHtmlToText(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "empty input",
			in:   "   \n  ",
			want: "",
		},
		{
			name: "collapses source indentation inside a paragraph",
			in:   "<p>Hello\n      there,\n      friend</p>",
			want: "Hello there, friend",
		},
		{
			name: "br is a single break, block boundary is a paragraph break",
			in:   "<p>one<br>two</p><p>three</p>",
			want: "one\ntwo\n\nthree",
		},
		{
			name: "anchor keeps its destination",
			in:   `<a href="https://trendly.now/c/1">View Collaboration</a>`,
			want: "View Collaboration (https://trendly.now/c/1)",
		},
		{
			name: "anchor whose label is the url is not duplicated",
			in:   `<a href="https://trendly.now">https://trendly.now</a>`,
			want: "https://trendly.now",
		},
		{
			name: "in-page anchor keeps only the label",
			in:   `<a href="#top">Back to top</a>`,
			want: "Back to top",
		},
		{
			name: "script and style content is dropped",
			in:   "<style>.a{color:red}</style><p>Body</p><script>alert(1)</script>",
			want: "Body",
		},
		{
			name: "entities are unescaped",
			in:   "<p>Fish &amp; Chips &mdash; &quot;tasty&quot;</p>",
			want: "Fish & Chips — \"tasty\"",
		},
		{
			name: "comments are stripped",
			in:   "<!-- Dynamic Variables: {{.Name}} --><p>Hi</p>",
			want: "Hi",
		},
		{
			name: "table rows become separate paragraphs",
			in:   "<table><tr><td>Brand: Acme</td></tr><tr><td>Amount: 500</td></tr></table>",
			want: "Brand: Acme\n\nAmount: 500",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := htmlToText(tc.in); got != tc.want {
				t.Errorf("htmlToText()\n got: %q\nwant: %q", got, tc.want)
			}
		})
	}
}

// Every template must yield a non-empty plain-text alternative: SES rejects a
// send whose body is empty, and an HTML-only message scores badly with spam
// filters.
func TestHtmlToTextOnRealTemplates(t *testing.T) {
	files, err := filepath.Glob("../../templates/*.html")
	if err != nil || len(files) == 0 {
		t.Skipf("no templates found: %v", err)
	}

	for _, file := range files {
		name := filepath.Base(file)
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("read %s: %v", name, err)
			}
			if len(strings.TrimSpace(string(raw))) == 0 {
				// templates/payment_order_created.html is committed empty —
				// tracked separately; not a regression in this converter.
				t.Skipf("%s is empty on disk", name)
			}

			got := htmlToText(string(raw))
			if strings.TrimSpace(got) == "" {
				t.Errorf("%s produced empty plain text", name)
			}
			if strings.Contains(got, lineSentinel) {
				t.Errorf("%s leaked the line sentinel into its output", name)
			}
		})
	}
}
