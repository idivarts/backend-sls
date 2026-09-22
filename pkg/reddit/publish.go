package reddit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
)

// SubmitOptions describes a post to create via /api/submit.
type SubmitOptions struct {
	Subreddit string   // target subreddit name (without the r/ prefix)
	Title     string   // post title
	Kind      string   // "self" (text), "link", or "image"
	Text      string   // body text for kind=="self"
	URL       string   // external URL for kind=="link"
	ImageURLs []string // image source URLs for kind=="image" (only the first is used)
	FlairID   string   // optional flair template id
	NSFW      bool     // mark the post NSFW
}

// jsonEnvelope is Reddit's common { "json": { "errors": [...], "data": {...} } }
// response wrapper. Reddit returns SOME errors inside a 200-OK body here, so
// content-mutating calls must inspect Errors even on success.
type jsonEnvelope struct {
	JSON struct {
		Errors [][]interface{} `json:"errors"`
		Data   json.RawMessage `json:"data"`
	} `json:"json"`
}

// errorsToErr converts Reddit's [[code, msg, field], ...] errors array into a
// single Go error, or nil when there are none.
func errorsToErr(errs [][]interface{}) error {
	if len(errs) == 0 {
		return nil
	}
	parts := make([]string, 0, len(errs))
	for _, e := range errs {
		s := make([]string, 0, len(e))
		for _, f := range e {
			if f == nil {
				continue
			}
			s = append(s, fmt.Sprintf("%v", f))
		}
		parts = append(parts, strings.Join(s, ": "))
	}
	return fmt.Errorf("reddit: api error: %s", strings.Join(parts, "; "))
}

// formPost performs an authenticated x-www-form-urlencoded POST to an OAuth API
// path. api_type=json is always appended so Reddit returns structured errors.
// It returns the raw body and also checks json.errors (returning an error if
// non-empty).
func formPost(accessToken, apiPath string, form url.Values) ([]byte, error) {
	form.Set("api_type", "json")
	req, err := authedRequest(http.MethodPost, apiPath, accessToken, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	body, _, err := doAuthed(req)
	if err != nil {
		return body, err
	}
	// Reddit surfaces some errors inside a 200 body under json.errors.
	var env jsonEnvelope
	if json.Unmarshal(body, &env) == nil {
		if err := errorsToErr(env.JSON.Errors); err != nil {
			return body, err
		}
	}
	return body, nil
}

// Submit creates a post in a subreddit. Kind is "self" (text), "link", or
// "image". On success it returns the new post's fullname (t3_…) and its
// permalink/url. Requires the submit scope.
func Submit(accessToken string, opt SubmitOptions) (postFullname string, permalink string, err error) {
	form := url.Values{}
	form.Set("sr", opt.Subreddit)
	form.Set("title", opt.Title)
	form.Set("kind", opt.Kind)
	if opt.FlairID != "" {
		form.Set("flair_id", opt.FlairID)
	}
	if opt.NSFW {
		form.Set("nsfw", "true")
	}

	switch opt.Kind {
	case "self":
		form.Set("text", opt.Text)
	case "link":
		form.Set("url", opt.URL)
	case "image":
		if len(opt.ImageURLs) == 0 {
			return "", "", fmt.Errorf("reddit: submit image: no image URLs provided")
		}
		// Upload the first image via the media asset lease to obtain a
		// reddit-hosted asset URL, then submit kind=image with that URL.
		assetURL, uerr := uploadMediaAsset(accessToken, opt.ImageURLs[0])
		if uerr != nil {
			// NOTE: image upload (media asset lease + S3 multipart PUT) failed;
			// fall back to a link post pointing at the original image URL so the
			// submission still succeeds.
			form.Set("kind", "link")
			form.Set("url", opt.ImageURLs[0])
		} else {
			form.Set("url", assetURL)
		}
	default:
		return "", "", fmt.Errorf("reddit: submit: unsupported kind %q", opt.Kind)
	}

	body, err := formPost(accessToken, "/api/submit", form)
	if err != nil {
		return "", "", err
	}

	// json.data carries { name, url, ... } for the created submission.
	var env struct {
		JSON struct {
			Data struct {
				Name string `json:"name"`
				URL  string `json:"url"`
			} `json:"data"`
		} `json:"json"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return "", "", fmt.Errorf("reddit: parse submit response: %w", err)
	}
	return env.JSON.Data.Name, env.JSON.Data.URL, nil
}

// uploadMediaAsset uploads an image to Reddit's media host using the asset
// lease flow and returns the resulting reddit-hosted asset URL (suitable for
// kind=image submissions).
//
// Flow:
//  1. POST /api/media/asset.json (filepath + mimetype) → S3 upload lease
//     (args.action = S3 endpoint, args.fields = required form fields).
//  2. Download the source image bytes.
//  3. POST a multipart/form-data request to the S3 endpoint with the leased
//     fields followed by the file part.
//  4. The final asset URL is action + "/" + the leased "key" field value.
func uploadMediaAsset(accessToken, imageURL string) (assetURL string, err error) {
	name := path.Base(imageURL)
	if i := strings.IndexAny(name, "?#"); i >= 0 {
		name = name[:i]
	}
	if name == "" || name == "." || name == "/" {
		name = "image.jpg"
	}

	// 1. Request the upload lease.
	leaseForm := url.Values{}
	leaseForm.Set("filepath", name)
	leaseForm.Set("mimetype", "image/jpeg")
	leaseBody, err := formPost(accessToken, "/api/media/asset.json", leaseForm)
	if err != nil {
		return "", fmt.Errorf("reddit: media asset lease: %w", err)
	}

	var lease struct {
		Args struct {
			Action string `json:"action"`
			Fields []struct {
				Name  string `json:"name"`
				Value string `json:"value"`
			} `json:"fields"`
		} `json:"args"`
		Asset struct {
			AssetID string `json:"asset_id"`
		} `json:"asset"`
	}
	if err := json.Unmarshal(leaseBody, &lease); err != nil {
		return "", fmt.Errorf("reddit: parse media asset lease: %w", err)
	}
	action := lease.Args.Action
	if strings.HasPrefix(action, "//") {
		action = "https:" + action
	}
	if action == "" {
		return "", fmt.Errorf("reddit: media asset lease missing action: %s", string(leaseBody))
	}

	// 2. Download the source image.
	imgResp, err := http.Get(imageURL)
	if err != nil {
		return "", fmt.Errorf("reddit: download image: %w", err)
	}
	defer imgResp.Body.Close()
	if imgResp.StatusCode < 200 || imgResp.StatusCode >= 300 {
		return "", fmt.Errorf("reddit: download image returned %d", imgResp.StatusCode)
	}
	imgBytes, err := io.ReadAll(imgResp.Body)
	if err != nil {
		return "", fmt.Errorf("reddit: read image bytes: %w", err)
	}

	// 3. Build the multipart upload to S3 (leased fields first, then the file).
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	var key string
	for _, f := range lease.Args.Fields {
		if f.Name == "key" {
			key = f.Value
		}
		if err := mw.WriteField(f.Name, f.Value); err != nil {
			return "", fmt.Errorf("reddit: write upload field: %w", err)
		}
	}
	part, err := mw.CreateFormFile("file", name)
	if err != nil {
		return "", fmt.Errorf("reddit: create file part: %w", err)
	}
	if _, err := part.Write(imgBytes); err != nil {
		return "", fmt.Errorf("reddit: write file part: %w", err)
	}
	if err := mw.Close(); err != nil {
		return "", fmt.Errorf("reddit: close multipart writer: %w", err)
	}

	upReq, err := http.NewRequest(http.MethodPost, action, &buf)
	if err != nil {
		return "", fmt.Errorf("reddit: build upload request: %w", err)
	}
	upReq.Header.Set("Content-Type", mw.FormDataContentType())
	upReq.Header.Set("User-Agent", UserAgent)
	upResp, err := http.DefaultClient.Do(upReq)
	if err != nil {
		return "", fmt.Errorf("reddit: upload to S3: %w", err)
	}
	defer upResp.Body.Close()
	upRespBody, _ := io.ReadAll(upResp.Body)
	if upResp.StatusCode < 200 || upResp.StatusCode >= 300 {
		return "", fmt.Errorf("reddit: S3 upload returned %d: %s", upResp.StatusCode, string(upRespBody))
	}

	// 4. The hosted asset URL is action + "/" + key.
	if key == "" {
		return "", fmt.Errorf("reddit: media asset lease missing key field")
	}
	return strings.TrimRight(action, "/") + "/" + key, nil
}

// itoa is a small helper kept local to avoid importing strconv at call sites
// scattered across this package's form builders.
func itoa(n int) string { return strconv.Itoa(n) }
