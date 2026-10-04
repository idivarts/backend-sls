package branch

import (
	"encoding/json"
	"testing"
)

// LinkData has a custom marshaller that has to flatten Extra into the same JSON
// object as the `$`-prefixed control params, because Branch accepts only one flat
// `data` dict.
func TestLinkDataMarshalFlattensExtra(t *testing.T) {
	raw, err := json.Marshal(LinkData{
		OGTitle:      "October content calendar",
		DeeplinkPath: "/share/tok123",
		DesktopURL:   "https://brands.trendly.now/share/tok123",
		Extra: map[string]string{
			"shareToken": "tok123",
			"shareType":  "calendarMonth",
			"month":      "", // empty values are dropped, not sent as ""
		},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	for key, want := range map[string]string{
		"$og_title":      "October content calendar",
		"$deeplink_path": "/share/tok123",
		"$desktop_url":   "https://brands.trendly.now/share/tok123",
		"shareToken":     "tok123",
		"shareType":      "calendarMonth",
	} {
		if got[key] != want {
			t.Errorf("%s = %v, want %q", key, got[key], want)
		}
	}
	if _, present := got["month"]; present {
		t.Error("empty Extra value should be omitted")
	}
	if _, present := got["Extra"]; present {
		t.Error("Extra must be flattened, not nested")
	}
	// Unset optional control params must not be sent at all — Branch treats an
	// empty $og_image_url as a real (broken) image.
	if _, present := got["$og_image_url"]; present {
		t.Error("unset control param should be omitted")
	}
}

// A modelled control param must win over an Extra key of the same name, so a
// caller cannot accidentally override the routing we set.
func TestLinkDataExtraCannotShadowControlParam(t *testing.T) {
	raw, err := json.Marshal(LinkData{
		DeeplinkPath: "/share/real",
		Extra:        map[string]string{"$deeplink_path": "/share/spoofed"},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["$deeplink_path"] != "/share/real" {
		t.Errorf("$deeplink_path = %v, want %q", got["$deeplink_path"], "/share/real")
	}
}

func TestConfiguredRequiresOnlyKey(t *testing.T) {
	if (Credentials{}).Configured() {
		t.Error("empty credentials should not report configured")
	}
	// The secret is needed only for read/update — its absence must not disable
	// link creation.
	if !(Credentials{Key: "key_test_abc"}).Configured() {
		t.Error("a key alone should be enough to create links")
	}
}
