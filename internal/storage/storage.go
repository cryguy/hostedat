package storage

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	maxFileSize  = 100 << 20 // 100 MB per file
	maxFileCount = 10000
	MaxZipSize   = 500 << 20 // 500 MB total
)

type Manager struct {
	BasePath string
}

func NewManager(basePath string) *Manager {
	return &Manager{BasePath: basePath}
}

func (m *Manager) ExtractZip(siteID string, version int, reader io.ReaderAt, size int64) error {
	if size > MaxZipSize {
		return fmt.Errorf("zip file too large (max %d bytes)", MaxZipSize)
	}

	zr, err := zip.NewReader(reader, size)
	if err != nil {
		return fmt.Errorf("reading zip: %w", err)
	}

	if len(zr.File) > maxFileCount {
		return fmt.Errorf("zip contains too many files (max %d)", maxFileCount)
	}

	destDir := m.GetDeploymentPath(siteID, version)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("creating deployment dir: %w", err)
	}

	// Detect single top-level directory to flatten
	prefix := detectSingleTopDir(zr.File)

	for _, f := range zr.File {
		name := f.Name
		if prefix != "" {
			name = strings.TrimPrefix(name, prefix)
			if name == "" {
				continue
			}
		}

		// Zip-slip protection
		destPath := filepath.Join(destDir, filepath.FromSlash(name))
		if !strings.HasPrefix(filepath.Clean(destPath), filepath.Clean(destDir)+string(os.PathSeparator)) && filepath.Clean(destPath) != filepath.Clean(destDir) {
			return fmt.Errorf("zip slip detected: %s", name)
		}

		mode := f.FileInfo().Mode()
		if mode&os.ModeSymlink != 0 {
			return fmt.Errorf("zip contains symlink: %s", name)
		}
		if !mode.IsDir() && !mode.IsRegular() {
			return fmt.Errorf("zip contains unsupported file type: %s", name)
		}

		if mode.IsDir() {
			if err := os.MkdirAll(destPath, 0755); err != nil {
				return fmt.Errorf("creating dir %s: %w", name, err)
			}
			continue
		}

		if f.UncompressedSize64 > maxFileSize {
			return fmt.Errorf("file too large: %s (%d bytes)", name, f.UncompressedSize64)
		}

		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return fmt.Errorf("creating parent dir for %s: %w", name, err)
		}

		if err := extractFile(f, destPath); err != nil {
			return err
		}
	}

	return nil
}

func detectSingleTopDir(files []*zip.File) string {
	if len(files) == 0 {
		return ""
	}

	var topDir string
	for _, f := range files {
		parts := strings.SplitN(f.Name, "/", 2)
		if topDir == "" {
			topDir = parts[0]
		} else if parts[0] != topDir {
			return ""
		}
	}

	// Only strip if it's actually a directory prefix
	return topDir + "/"
}

func extractFile(f *zip.File, destPath string) error {
	rc, err := f.Open()
	if err != nil {
		return fmt.Errorf("opening zip entry %s: %w", f.Name, err)
	}
	defer rc.Close()

	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("creating file %s: %w", destPath, err)
	}
	defer out.Close()

	if _, err := io.Copy(out, io.LimitReader(rc, maxFileSize+1)); err != nil {
		return fmt.Errorf("writing file %s: %w", destPath, err)
	}

	return nil
}

func (m *Manager) GetDeploymentPath(siteID string, version int) string {
	return filepath.Join(m.BasePath, siteID, fmt.Sprintf("%d", version))
}

func (m *Manager) DeleteSite(siteID string) error {
	return os.RemoveAll(filepath.Join(m.BasePath, siteID))
}

// ResolveFile tries to find a file at the given request path.
// It checks: exact path, path/index.html, path.html
func (m *Manager) ResolveFile(deploymentPath, requestPath string) (string, bool) {
	// Clean and normalize the path
	requestPath = strings.TrimPrefix(requestPath, "/")
	if requestPath == "" {
		requestPath = "index.html"
	}

	// Try exact path
	fullPath := filepath.Join(deploymentPath, filepath.FromSlash(requestPath))

	// Path traversal protection
	absDeployment, err := filepath.Abs(deploymentPath)
	if err != nil {
		return "", false
	}
	absFullPath, err := filepath.Abs(fullPath)
	if err != nil {
		return "", false
	}
	if !strings.HasPrefix(absFullPath, absDeployment+string(os.PathSeparator)) && absFullPath != absDeployment {
		return "", false
	}

	if isFile(fullPath) {
		return fullPath, true
	}

	// Try path/index.html
	indexPath := filepath.Join(fullPath, "index.html")
	if isFile(indexPath) {
		return indexPath, true
	}

	// Try path.html
	htmlPath := fullPath + ".html"
	if isFile(htmlPath) {
		return htmlPath, true
	}

	return "", false
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
