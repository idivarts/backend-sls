package mediagen

import "testing"

// Both cases come from one real 422, which named both fields at once:
//
//	start_frame.inline_base64.mime_type: Input should be 'image/jpeg',
//	'image/png', 'image/webp', 'image/heic' or 'image/heif'
//
// A frame uploaded to S3 without an explicit content type comes back as
// `binary/octet-stream`, and forwarding that refused every pinned start frame.

func pngBytes() []byte {
	// PNG signature + enough of an IHDR for DetectContentType.
	return append([]byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}, make([]byte, 64)...)
}

func jpegBytes() []byte {
	return append([]byte{0xff, 0xd8, 0xff, 0xe0}, make([]byte, 64)...)
}

func TestFrameMimeTypeTrustsTheBytesOverTheHeader(t *testing.T) {
	cases := []struct {
		name   string
		data   []byte
		header string
		want   string
	}{
		{"png with a useless header", pngBytes(), "binary/octet-stream", "image/png"},
		{"png with no header at all", pngBytes(), "", "image/png"},
		{"jpeg with a charset suffix", jpegBytes(), "image/jpeg; charset=binary", "image/jpeg"},
		// The bytes say PNG; a header claiming otherwise does not get to win.
		{"a lying header", pngBytes(), "image/gif", "image/png"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := frameMimeType(c.data, c.header)
			if !ok || got != c.want {
				t.Fatalf("frameMimeType(%q) = %q,%v; want %q,true", c.header, got, ok, c.want)
			}
		})
	}
}

func TestFrameMimeTypeFallsBackToAnAcceptableHeader(t *testing.T) {
	// DetectContentType knows nothing about HEIC, so a header the API accepts
	// is still worth taking when the bytes are unrecognised.
	got, ok := frameMimeType(make([]byte, 64), "image/heic")
	if !ok || got != "image/heic" {
		t.Fatalf("got %q,%v; want image/heic,true", got, ok)
	}
}

func TestFrameMimeTypeRefusesWhatTheModelWouldRefuse(t *testing.T) {
	// A GIF is a real image and still not on the list; refusing here names the
	// problem, where the provider's 422 reaches the user as "couldn't start
	// this clip" and reaches nobody as a reason.
	gif := append([]byte("GIF89a"), make([]byte, 64)...)
	if _, ok := frameMimeType(gif, "image/gif"); ok {
		t.Fatal("a GIF must be refused: the model's list does not include it")
	}
	if _, ok := frameMimeType(make([]byte, 64), "binary/octet-stream"); ok {
		t.Fatal("unrecognised bytes with a useless header must be refused")
	}
}
