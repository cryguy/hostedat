package client

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Deployment struct {
	ID         string    `json:"id"`
	SiteID     string    `json:"site_id"`
	Version    int       `json:"version"`
	FileHash   string    `json:"file_hash"`
	UploadedAt time.Time `json:"uploaded_at"`
}

func (c *Client) Deploy(siteID, dirPath string) (*Deployment, error) {
	// Zip the directory in memory
	zipData, err := zipDirectory(dirPath)
	if err != nil {
		return nil, fmt.Errorf("failed to zip directory: %w", err)
	}

	// Build multipart form
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "deploy.zip")
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(part, bytes.NewReader(zipData)); err != nil {
		return nil, err
	}
	writer.Close()

	resp, err := c.do("POST", "/api/v1/sites/"+siteID+"/deploy", &body, writer.FormDataContentType())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var deployment Deployment
	if err := json.NewDecoder(resp.Body).Decode(&deployment); err != nil {
		return nil, err
	}
	return &deployment, nil
}

func zipDirectory(dirPath string) ([]byte, error) {
	dirPath, err := filepath.Abs(dirPath)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(dirPath)
	if err != nil {
		return nil, fmt.Errorf("directory not found: %s", dirPath)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("not a directory: %s", dirPath)
	}

	var buf bytes.Buffer
	w := zip.NewWriter(&buf)

	err = filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		// Relative path with forward slashes (zip standard)
		relPath, err := filepath.Rel(dirPath, path)
		if err != nil {
			return err
		}
		relPath = strings.ReplaceAll(relPath, string(filepath.Separator), "/")

		f, err := w.Create(relPath)
		if err != nil {
			return err
		}

		src, err := os.Open(path)
		if err != nil {
			return err
		}
		defer src.Close()

		_, err = io.Copy(f, src)
		return err
	})
	if err != nil {
		return nil, err
	}

	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
