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
	maxFileSize          = 25 << 20  // 25 MB per file (matches Cloudflare Pages)
	maxFileCount         = 20000     // 20k files (matches Cloudflare Pages)
	MaxZipSize           = 25 << 20  // 25 MB compressed upload
	maxTotalUncompressed = 256 << 20 // 256 MB aggregate uncompressed limit
)

type Manager struct {
	BasePath string
}

func NewManager(basePath string) *Manager {
	return &Manager{BasePath: basePath}
}

func (m *Manager) ExtractZip(siteID string, deployKey string, reader io.ReaderAt, size int64) (extractErr error) {
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

	destDir := m.GetDeploymentPath(siteID, deployKey)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("creating deployment dir: %w", err)
	}

	// Clean up partially extracted files on any error.
	defer func() {
		if extractErr != nil {
			_ = os.RemoveAll(destDir)
		}
	}()

	// Detect single top-level directory to flatten
	prefix := detectSingleTopDir(zr.File)

	var totalUncompressed uint64

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

		// Early reject based on header (untrusted, but avoids wasted work)
		if f.UncompressedSize64 > maxFileSize {
			return fmt.Errorf("file too large: %s (%d bytes)", name, f.UncompressedSize64)
		}

		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return fmt.Errorf("creating parent dir for %s: %w", name, err)
		}

		// Track actual bytes written (not header-reported) for aggregate limit
		written, err := extractFile(f, destPath)
		if err != nil {
			return err
		}
		totalUncompressed += uint64(written)
		if totalUncompressed > maxTotalUncompressed {
			return fmt.Errorf("zip total uncompressed size exceeds limit (%d bytes)", maxTotalUncompressed)
		}
	}

	return nil
}

func detectSingleTopDir(files []*zip.File) string {
	if len(files) == 0 {
		return ""
	}

	var topDir string
	hasDirEntry := false
	for _, f := range files {
		parts := strings.SplitN(f.Name, "/", 2)
		if topDir == "" {
			topDir = parts[0]
		} else if parts[0] != topDir {
			return ""
		}
		// Track whether any entry actually lives inside this directory.
		if len(parts) == 2 && parts[1] != "" {
			hasDirEntry = true
		}
	}

	// Only strip if all entries share the same top-level directory and at
	// least one entry exists inside it (i.e., it's truly a wrapper directory).
	if !hasDirEntry {
		return ""
	}

	return topDir + "/"
}

func extractFile(f *zip.File, destPath string) (int64, error) {
	rc, err := f.Open()
	if err != nil {
		return 0, fmt.Errorf("opening zip entry %s: %w", f.Name, err)
	}
	defer func() { _ = rc.Close() }()

	out, err := os.Create(destPath)
	if err != nil {
		return 0, fmt.Errorf("creating file %s: %w", destPath, err)
	}

	written, err := io.Copy(out, io.LimitReader(rc, maxFileSize+1))
	if err != nil {
		_ = out.Close()
		return 0, fmt.Errorf("writing file %s: %w", destPath, err)
	}
	if written > maxFileSize {
		_ = out.Close()
		return 0, fmt.Errorf("file exceeds maximum size during extraction: %s", f.Name)
	}

	if err := out.Close(); err != nil {
		return 0, fmt.Errorf("closing file %s: %w", destPath, err)
	}
	return written, nil
}

func (m *Manager) GetDeploymentPath(siteID string, deployKey string) string {
	return filepath.Join(m.BasePath, siteID, deployKey)
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

// HasWorkerScript checks if a _worker.js file exists in the deployment.
func (m *Manager) HasWorkerScript(siteID string, deployKey string) bool {
	workerPath := filepath.Join(m.GetDeploymentPath(siteID, deployKey), "_worker.js")
	return isFile(workerPath)
}

// GetWorkerScript reads the _worker.js source from a deployment.
func (m *Manager) GetWorkerScript(siteID string, deployKey string) (string, error) {
	workerPath := filepath.Join(m.GetDeploymentPath(siteID, deployKey), "_worker.js")
	data, err := os.ReadFile(workerPath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// GetWorkerBytecodeDir returns the path to the .worker directory for a deployment.
func (m *Manager) GetWorkerBytecodeDir(siteID string, deployKey string) string {
	return filepath.Join(m.GetDeploymentPath(siteID, deployKey), ".worker")
}
