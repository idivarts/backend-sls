package canva

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// importUpload POSTs raw file bytes with the base64 Import-Metadata header.
func importUpload(accessToken, title, mimeType string, data []byte) (string, error) {
	meta := map[string]string{
		"title_base64": base64.StdEncoding.EncodeToString([]byte(title)),
		"mime_type":    mimeType,
	}
	metaJSON, _ := json.Marshal(meta)

	req, err := http.NewRequest(http.MethodPost, apiBase+"/imports", bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Import-Metadata", string(metaJSON))
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("canva imports returned %s: %s", resp.Status, string(raw))
	}
	var out struct {
		Job struct {
			ID string `json:"id"`
		} `json:"job"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", err
	}
	return out.Job.ID, nil
}

// ImportJob is the polled result of a design import.
type ImportJob struct {
	Status    string   `json:"status"` // in_progress | success | failed
	DesignIDs []string `json:"designIds"`
}

// GetImportJob polls a design-import job.
func GetImportJob(accessToken, jobID string) (*ImportJob, error) {
	var out struct {
		Job struct {
			Status string `json:"status"`
			Result struct {
				Designs []struct {
					ID string `json:"id"`
				} `json:"designs"`
			} `json:"result"`
		} `json:"job"`
	}
	if err := doJSON("GET", "/imports/"+url.PathEscape(jobID), accessToken, nil, &out); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(out.Job.Result.Designs))
	for _, d := range out.Job.Result.Designs {
		ids = append(ids, d.ID)
	}
	return &ImportJob{Status: out.Job.Status, DesignIDs: ids}, nil
}
