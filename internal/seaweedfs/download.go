package seaweedfs

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"

	"github.com/cryguy/hostedat/internal/config"
)

// DefaultWeedVersion is the pinned SeaweedFS release version.
const DefaultWeedVersion = "4.13"

// downloadURL returns the GitHub release download URL for the given version, OS, and architecture.
func downloadURL(version, goos, goarch string) (string, error) {
	var filename string
	switch {
	case goos == "linux" && goarch == "amd64":
		filename = "linux_amd64.tar.gz"
	case goos == "linux" && goarch == "arm64":
		filename = "linux_arm64.tar.gz"
	case goos == "darwin" && goarch == "amd64":
		filename = "darwin_amd64.tar.gz"
	case goos == "darwin" && goarch == "arm64":
		filename = "darwin_arm64.tar.gz"
	case goos == "windows" && goarch == "amd64":
		filename = "windows_amd64.zip"
	default:
		return "", fmt.Errorf("unsupported platform: %s/%s", goos, goarch)
	}
	return fmt.Sprintf("https://github.com/seaweedfs/seaweedfs/releases/download/%s/%s", version, filename), nil
}

// EnsureBinary checks for the weed binary and downloads it from GitHub releases if missing.
// Resolution order:
//  1. If cfg.BinaryPath is set and the file exists, return it.
//  2. If cfg.BinaryPath is set but the file doesn't exist, download to that path.
//  3. If cfg.BinaryPath is empty, download to {dataDir}/bin/weed[.exe].
func EnsureBinary(cfg config.ObjectStorageConfig) (string, error) {
	target := cfg.BinaryPath
	if target == "" {
		dataDir := cfg.DataDir
		if dataDir == "" {
			dataDir = "./data/seaweedfs"
		}
		binaryName := "weed"
		if runtime.GOOS == "windows" {
			binaryName = "weed.exe"
		}
		target = filepath.Join(dataDir, "bin", binaryName)
	}

	// If the binary already exists, return it.
	if _, err := os.Stat(target); err == nil {
		return target, nil
	}

	// Download the binary.
	goos := runtime.GOOS
	goarch := runtime.GOARCH
	dlURL, err := downloadURL(DefaultWeedVersion, goos, goarch)
	if err != nil {
		return "", err
	}

	log.Printf("Downloading SeaweedFS v%s for %s/%s...", DefaultWeedVersion, goos, goarch)

	// Ensure the target directory exists.
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return "", fmt.Errorf("creating binary directory: %w", err)
	}

	// Download to a temp file in the target directory.
	tmpDownload, err := os.CreateTemp(filepath.Dir(target), "weed-download-*")
	if err != nil {
		return "", fmt.Errorf("creating temp download file: %w", err)
	}
	tmpDownloadPath := tmpDownload.Name()
	defer func() { _ = os.Remove(tmpDownloadPath) }()

	resp, err := http.Get(dlURL)
	if err != nil {
		_ = tmpDownload.Close()
		return "", fmt.Errorf("downloading SeaweedFS: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		_ = tmpDownload.Close()
		return "", fmt.Errorf("downloading SeaweedFS: HTTP %d", resp.StatusCode)
	}

	if _, err := io.Copy(tmpDownload, resp.Body); err != nil {
		_ = tmpDownload.Close()
		return "", fmt.Errorf("saving download: %w", err)
	}
	_ = tmpDownload.Close()

	// Extract binary to a temp file, then rename atomically.
	tmpBinary, err := os.CreateTemp(filepath.Dir(target), "weed-binary-*")
	if err != nil {
		return "", fmt.Errorf("creating temp binary file: %w", err)
	}
	tmpBinaryPath := tmpBinary.Name()
	_ = tmpBinary.Close()
	defer func() { _ = os.Remove(tmpBinaryPath) }()

	if goos == "windows" {
		err = extractZip(tmpDownloadPath, tmpBinaryPath)
	} else {
		err = extractTarGz(tmpDownloadPath, tmpBinaryPath)
	}
	if err != nil {
		return "", fmt.Errorf("extracting SeaweedFS binary: %w", err)
	}

	// Make executable on Unix.
	if goos != "windows" {
		if err := os.Chmod(tmpBinaryPath, 0755); err != nil {
			return "", fmt.Errorf("setting binary permissions: %w", err)
		}
	}

	// Atomic rename to final path.
	if err := os.Rename(tmpBinaryPath, target); err != nil {
		return "", fmt.Errorf("moving binary to final path: %w", err)
	}

	log.Printf("SeaweedFS binary installed at %s", target)
	return target, nil
}

// extractTarGz extracts the weed binary from a .tar.gz archive.
func extractTarGz(archivePath, destPath string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer func() { _ = gr.Close() }()

	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if filepath.Base(hdr.Name) == "weed" && hdr.Typeflag == tar.TypeReg {
			out, err := os.Create(destPath)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				_ = out.Close()
				return err
			}
			return out.Close()
		}
	}
	return fmt.Errorf("weed binary not found in archive")
}

// extractZip extracts the weed.exe binary from a .zip archive.
func extractZip(archivePath, destPath string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()

	for _, f := range r.File {
		if filepath.Base(f.Name) == "weed.exe" {
			rc, err := f.Open()
			if err != nil {
				return err
			}
			defer func() { _ = rc.Close() }()

			out, err := os.Create(destPath)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, rc); err != nil {
				_ = out.Close()
				return err
			}
			return out.Close()
		}
	}
	return fmt.Errorf("weed.exe not found in archive")
}
