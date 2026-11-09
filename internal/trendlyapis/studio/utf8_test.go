package studio

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// Firestore speaks protobuf, whose string fields must be valid UTF-8, and it
// refuses the WHOLE write over one bad byte. A model that truncates a
// multi-byte sequence at a token boundary therefore cost a real director plan:
// 121 seconds of frontier-model work, already paid for, thrown away — and the
// run stranded in `planning`, because the failing call was the one meant to
// record the outcome.

// The actual shape that broke it: a multi-byte sequence cut in half.
func TestATruncatedMultiByteSequenceIsMadeSafe(t *testing.T) {
	// "…" is three bytes; keep two of them.
	broken := "the plan summary " + "\xe2\x80" + " and the rest"
	if utf8.ValidString(broken) {
		t.Fatal("fixture is not actually broken")
	}

	got := safeForFirestore(broken)
	if !utf8.ValidString(got) {
		t.Fatal("still invalid after sanitising — Firestore would refuse this")
	}
	if !strings.Contains(got, "the plan summary") || !strings.Contains(got, "and the rest") {
		t.Errorf("sanitising ate the surrounding text: %q", got)
	}
}

// A lone continuation byte, the other common truncation.
func TestALoneContinuationByteIsDropped(t *testing.T) {
	got := safeForFirestore("scene one\xb8scene two")
	if !utf8.ValidString(got) {
		t.Fatal("still invalid")
	}
	if got != "scene onescene two" {
		t.Errorf("got %q", got)
	}
}

// ⚠️ Valid text must come through BYTE FOR BYTE. Emoji, accents and CJK are
// ordinary in a brand's own copy, and a sanitiser that mangled them would
// quietly corrupt every plan to avoid a rare one failing.
func TestValidTextIsUntouched(t *testing.T) {
	for _, s := range []string{
		"A punchy hook — em dash, curly ’quotes’ and an ellipsis…",
		"emoji survive 🎬🔥 unharmed",
		"日本語のナレーション",
		"café, naïve, Zürich",
		"",
	} {
		if got := safeForFirestore(s); got != s {
			t.Errorf("valid text was altered:\n in: %q\nout: %q", s, got)
		}
	}
}

// The cheap path has to be the common one: almost every string is already
// valid, and this runs over whole HTML documents.
func TestAValidStringIsReturnedAsIs(t *testing.T) {
	s := strings.Repeat("<div>fine</div>", 500)
	if safeForFirestore(s) != s {
		t.Error("a valid document should be returned unchanged")
	}
}
