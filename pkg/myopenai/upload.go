package myopenai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
)

type FileResponse struct {
	ID        string `json:"id"`
	Object    string `json:"object"`
	Bytes     int    `json:"bytes"`
	CreatedAt int64  `json:"created_at"`
	FileName  string `json:"filename"`
	Purpose   string `json:"purpose"`
}

func UploadImage(fileURL string) (*FileResponse, error) {
	url := fmt.Sprintf("%s/files", baseURL)

	// Retrieve the file data from the provided URL
	resp, err := http.Get(fileURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Create a new buffer to store the form data
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)

	// Add the purpose field to the form data
	writer.WriteField("purpose", "vision")

	// Get the file name from the URL
	fileName := fileURL[strings.LastIndex(fileURL, "/")+1:]

	// Create a form file field for the file data
	part, err := writer.CreateFormFile("file", fileName)
	if err != nil {
		return nil, err
	}

	// Copy the file data to the form file field
	_, err = io.Copy(part, resp.Body)
	if err != nil {
		return nil, err
	}

	// Close the multipart writer
	err = writer.Close()
	if err != nil {
		return nil, err
	}

	// Create a new HTTP request with the form data
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return nil, err
	}

	// Set the authorization header with the API key
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// Send the HTTP request
	client := &http.Client{}
	resp, err = client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Print the response status
	fmt.Println("Response Status:", resp.Status)
	if resp.StatusCode != http.StatusAccepted {
		return nil, fmt.Errorf("%s - Error code mismatch", resp.Status)
	}

	rbytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	file := &FileResponse{}
	err = json.Unmarshal(rbytes, file)
	if err != nil {
		return nil, err
	}

	return file, nil
}
