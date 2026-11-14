package hfproject

import "testing"

func TestRevisionPrefix(t *testing.T) {
	if got, want := RevisionPrefix("b", "d", 3), "design-projects/b/d/revisions/3"; got != want {
		t.Fatalf("RevisionPrefix = %q, want %q", got, want)
	}
	// Zero and below mean the legacy root: a design written before
	// revisions existed still points there, and nothing may move its files
	// out from under it.
	for _, n := range []int{0, -1} {
		if got, want := RevisionPrefix("b", "d", n), Prefix("b", "d"); got != want {
			t.Fatalf("RevisionPrefix(%d) = %q, want the legacy prefix %q", n, got, want)
		}
	}
}

// Every revision stays under the design root, because the render task role
// is scoped to design-projects/{brand}/{design} and a prefix outside it is
// a render that cannot read its own project.
func TestRevisionPrefixStaysUnderTheDesignRoot(t *testing.T) {
	root := Prefix("b", "d")
	for n := 1; n < 50; n++ {
		p := RevisionPrefix("b", "d", n)
		if len(p) <= len(root) || p[:len(root)+1] != root+"/" {
			t.Fatalf("revision %d sits at %q, outside %q", n, p, root)
		}
	}
}
