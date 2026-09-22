package canva

import (
	"fmt"
	"net/url"
)

// ---- Assets (seed a design with our rendered draft) ----

// UploadURLAsset asks Canva to ingest a publicly-reachable URL (our CloudFront
// draft) as an asset. Returns the async job id; poll GetURLAssetJob.
func UploadURLAsset(accessToken, assetURL, name string) (string, error) {
	var out struct {
		Job struct {
			ID string `json:"id"`
		} `json:"job"`
	}
	err := doJSON("POST", "/url-asset-uploads", accessToken, map[string]interface{}{
		"url":  assetURL,
		"name": name,
	}, &out)
	if err != nil {
		return "", err
	}
	return out.Job.ID, nil
}

// URLAssetJob is the polled result of a URL asset upload.
type URLAssetJob struct {
	Status  string `json:"status"` // in_progress | success | failed
	AssetID string
}

// GetURLAssetJob polls a URL asset upload job.
func GetURLAssetJob(accessToken, jobID string) (*URLAssetJob, error) {
	var out struct {
		Job struct {
			Status string `json:"status"`
			Asset  struct {
				ID string `json:"id"`
			} `json:"asset"`
		} `json:"job"`
	}
	if err := doJSON("GET", "/url-asset-uploads/"+url.PathEscape(jobID), accessToken, nil, &out); err != nil {
		return nil, err
	}
	return &URLAssetJob{Status: out.Job.Status, AssetID: out.Job.Asset.ID}, nil
}

// ---- Designs ----

// Design is a Canva design summary.
type Design struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	URLs  struct {
		EditURL string `json:"edit_url"`
		ViewURL string `json:"view_url"`
	} `json:"urls"`
	Thumbnail struct {
		URL string `json:"url"`
	} `json:"thumbnail"`
}

// CreateCustomDesign creates a custom-sized design, optionally seeded with an
// asset (a flat image of our draft). Returns the design with its edit_url.
func CreateCustomDesign(accessToken string, width, height int, title, assetID string) (*Design, error) {
	body := map[string]interface{}{
		"design_type": map[string]interface{}{
			"type":   "custom",
			"width":  width,
			"height": height,
		},
	}
	if title != "" {
		body["title"] = title
	}
	if assetID != "" {
		body["asset_id"] = assetID
	}
	var out struct {
		Design Design `json:"design"`
	}
	if err := doJSON("POST", "/designs", accessToken, body, &out); err != nil {
		return nil, err
	}
	return &out.Design, nil
}

// ListDesigns lists the user's designs (owned), optionally filtered by query.
func ListDesigns(accessToken, query string) ([]Design, error) {
	path := "/designs?ownership=owned"
	if query != "" {
		path += "&query=" + url.QueryEscape(query)
	}
	var out struct {
		Items []Design `json:"items"`
	}
	if err := doJSON("GET", path, accessToken, nil, &out); err != nil {
		return nil, err
	}
	return out.Items, nil
}

// GetDesign fetches a single design (used to resolve the edit_url on return).
func GetDesign(accessToken, designID string) (*Design, error) {
	var out struct {
		Design Design `json:"design"`
	}
	if err := doJSON("GET", "/designs/"+url.PathEscape(designID), accessToken, nil, &out); err != nil {
		return nil, err
	}
	return &out.Design, nil
}

// ---- Exports (pull the finished design back) ----

// CreateExport starts an export job. format is png|jpg|mp4|gif|pdf.
func CreateExport(accessToken, designID, format string) (string, error) {
	var out struct {
		Job struct {
			ID string `json:"id"`
		} `json:"job"`
	}
	err := doJSON("POST", "/exports", accessToken, map[string]interface{}{
		"design_id": designID,
		"format":    map[string]interface{}{"type": format},
	}, &out)
	if err != nil {
		return "", err
	}
	return out.Job.ID, nil
}

// ExportJob is the polled result of an export. URLs are valid for 24h.
type ExportJob struct {
	Status string   `json:"status"` // in_progress | success | failed
	URLs   []string `json:"urls"`
}

// GetExport polls an export job.
func GetExport(accessToken, jobID string) (*ExportJob, error) {
	var out struct {
		Job struct {
			Status string   `json:"status"`
			URLs   []string `json:"urls"`
		} `json:"job"`
	}
	if err := doJSON("GET", "/exports/"+url.PathEscape(jobID), accessToken, nil, &out); err != nil {
		return nil, err
	}
	return &ExportJob{Status: out.Job.Status, URLs: out.Job.URLs}, nil
}

// ---- Design import (editable-layer handoff via PPTX) ----

// ImportDesign starts a design-import job from a file (e.g. our scene→PPTX so
// text/shapes stay editable layers in Canva). mimeType e.g.
// application/vnd.openxmlformats-officedocument.presentationml.presentation.
// Uses the raw-bytes + base64 metadata-header convention of the Connect import
// API.
func ImportDesign(accessToken, title, mimeType string, data []byte) (string, error) {
	jobID, err := importUpload(accessToken, title, mimeType, data)
	if err != nil {
		return "", fmt.Errorf("canva import: %w", err)
	}
	return jobID, nil
}
