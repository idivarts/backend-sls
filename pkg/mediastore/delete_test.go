package mediastore

import "testing"

// The guard is the whole safety story of this file, so it is tested on the
// arguments a bug would actually produce — not just on the happy shape.
func TestDeletePrefixAcceptsOnlyARevisionDirectory(t *testing.T) {
	ok := []string{
		"design-projects/brand1/design1/revisions/1",
		"design-projects/i1ka6wFxRBNCowqo1r4N/t2JhmM3KAlacOZxy6vDu/revisions/42",
		"design-projects/b-1/d_2/revisions/10",
	}
	for _, p := range ok {
		if !designRevisionPrefix.MatchString(p) {
			t.Fatalf("refused a real revision prefix: %q", p)
		}
	}

	bad := map[string]string{
		"":                               "empty",
		"design-projects":                "the root of every design",
		"design-projects/brand1":         "a whole brand",
		"design-projects/brand1/design1": "a whole design, current revision included",
		"design-projects/brand1/design1/revisions":   "every revision at once",
		"design-projects/brand1/design1/revisions/":  "the same, with a slash",
		"design-projects/brand1/design1/revisions/0": "revision zero does not exist",
		"design-projects/b/d/revisions/1/..":         "traversal",
		"design-projects/b/d/revisions/../../..":     "traversal to the bucket root",
		"uploads":                                    "the media library",
		"uploads/design-projects/b/d/revisions/1":    "somewhere else entirely",
		"design-projects/b/d/revisions/1x":           "not a number",
		"*":                                          "a wildcard",
	}
	for p, why := range bad {
		if designRevisionPrefix.MatchString(p) {
			t.Fatalf("accepted %q (%s)", p, why)
		}
	}
}
