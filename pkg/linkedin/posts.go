package linkedin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// restHeaders sets the headers required by the versioned /rest endpoints
// (Posts, Images). Every versioned call must carry the LinkedIn-Version month
// and the Rest.li protocol version.
func restHeaders(req *http.Request, accessToken string) {
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("LinkedIn-Version", APIVersion)
	req.Header.Set("X-Restli-Protocol-Version", "2.0.0")
}

// commentaryEscaper escapes the "little text" reserved characters that the
// Posts API `commentary` field requires to be backslash-escaped; otherwise the
// API rejects the post with a 400. We deliberately leave '#' unescaped so that
// hashtags in captions still render as hashtags.
var commentaryEscaper = strings.NewReplacer(
	`\`, `\\`,
	`(`, `\(`, `)`, `\)`,
	`[`, `\[`, `]`, `\]`,
	`{`, `\{`, `}`, `\}`,
	`<`, `\<`, `>`, `\>`,
	`@`, `\@`, `|`, `\|`,
	`~`, `\~`, `_`, `\_`, `*`, `\*`,
)

func escapeCommentary(s string) string {
	return commentaryEscaper.Replace(s)
}

// CreateMemberPost publishes a post authored by a member (personal profile) to
// the main feed via the versioned Posts API.
//
//	authorURN — the member URN, e.g. "urn:li:person:abc123"
//	text      — the post body (caption + hashtags); required unless images are set
//	imageURLs — optional images; each is uploaded to LinkedIn before posting.
//	            A single image uses the `media` content; 2+ images use the
//	            `multiImage` content (LinkedIn's native carousel/swipe post).
//
// Returns the created post URN (from the x-restli-id response header).
func CreateMemberPost(accessToken, authorURN, text string, imageURLs []string) (string, error) {
	if strings.TrimSpace(text) == "" && len(imageURLs) == 0 {
		return "", fmt.Errorf("linkedin: post has no text or image")
	}

	post := map[string]interface{}{
		"author":                    authorURN,
		"commentary":                escapeCommentary(text),
		"visibility":                "PUBLIC",
		"lifecycleState":            "PUBLISHED",
		"isReshareDisabledByAuthor": false,
		"distribution": map[string]interface{}{
			"feedDistribution":               "MAIN_FEED",
			"targetEntities":                 []interface{}{},
			"thirdPartyDistributionChannels": []interface{}{},
		},
	}

	if len(imageURLs) > 0 {
		// Upload every image up front so each gets its own image URN.
		imageURNs := make([]string, 0, len(imageURLs))
		for _, u := range imageURLs {
			imageURN, err := uploadImage(accessToken, authorURN, u)
			if err != nil {
				return "", err
			}
			imageURNs = append(imageURNs, imageURN)
		}

		if len(imageURNs) == 1 {
			post["content"] = map[string]interface{}{
				"media": map[string]interface{}{
					"id": imageURNs[0],
				},
			}
		} else {
			// 2+ images → LinkedIn multi-image (carousel) post. Each entry needs
			// an `id`; `altText` is required by the API, so we send an empty one.
			images := make([]map[string]interface{}, 0, len(imageURNs))
			for _, urn := range imageURNs {
				images = append(images, map[string]interface{}{
					"id":      urn,
					"altText": "",
				})
			}
			post["content"] = map[string]interface{}{
				"multiImage": map[string]interface{}{
					"images": images,
				},
			}
		}
	}

	body, err := json.Marshal(post)
	if err != nil {
		return "", fmt.Errorf("linkedin: marshal post: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, RestBaseURL+"/posts", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("linkedin: build post request: %w", err)
	}
	restHeaders(req, accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("linkedin: post request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("linkedin: posts endpoint returned %d: %s", resp.StatusCode, string(respBody))
	}

	// The created post's URN is returned in a response header, not the body.
	postURN := resp.Header.Get("x-restli-id")
	if postURN == "" {
		postURN = resp.Header.Get("x-linkedin-id")
	}
	return postURN, nil
}

// uploadImage registers and uploads a single image via the versioned Images API
// and returns its image URN (urn:li:image:...). owner is the author URN (member
// or organization) that will own the asset.
func uploadImage(accessToken, ownerURN, imageURL string) (string, error) {
	// 1. Initialize the upload to get a pre-signed upload URL + image URN.
	initBody, _ := json.Marshal(map[string]interface{}{
		"initializeUploadRequest": map[string]interface{}{
			"owner": ownerURN,
		},
	})
	req, err := http.NewRequest(http.MethodPost, RestBaseURL+"/images?action=initializeUpload", bytes.NewReader(initBody))
	if err != nil {
		return "", fmt.Errorf("linkedin: build image init request: %w", err)
	}
	restHeaders(req, accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("linkedin: image init request failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("linkedin: image init returned %d: %s", resp.StatusCode, string(body))
	}

	var init struct {
		Value struct {
			UploadURL string `json:"uploadUrl"`
			Image     string `json:"image"`
		} `json:"value"`
	}
	if err := json.Unmarshal(body, &init); err != nil {
		return "", fmt.Errorf("linkedin: parse image init: %w", err)
	}
	if init.Value.UploadURL == "" || init.Value.Image == "" {
		return "", fmt.Errorf("linkedin: image init missing uploadUrl/image: %s", string(body))
	}

	// 2. Download the source image bytes.
	imgResp, err := http.Get(imageURL)
	if err != nil {
		return "", fmt.Errorf("linkedin: fetch source image: %w", err)
	}
	defer imgResp.Body.Close()
	if imgResp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("linkedin: fetch source image returned %d", imgResp.StatusCode)
	}
	imgBytes, err := io.ReadAll(imgResp.Body)
	if err != nil {
		return "", fmt.Errorf("linkedin: read source image: %w", err)
	}

	// 3. Upload the bytes to the pre-signed URL.
	upReq, err := http.NewRequest(http.MethodPut, init.Value.UploadURL, bytes.NewReader(imgBytes))
	if err != nil {
		return "", fmt.Errorf("linkedin: build image upload request: %w", err)
	}
	upReq.Header.Set("Authorization", "Bearer "+accessToken)
	upResp, err := http.DefaultClient.Do(upReq)
	if err != nil {
		return "", fmt.Errorf("linkedin: image upload failed: %w", err)
	}
	defer upResp.Body.Close()
	if upResp.StatusCode != http.StatusOK && upResp.StatusCode != http.StatusCreated {
		ub, _ := io.ReadAll(upResp.Body)
		return "", fmt.Errorf("linkedin: image upload returned %d: %s", upResp.StatusCode, string(ub))
	}

	return init.Value.Image, nil
}
