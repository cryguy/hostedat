package worker

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Pinned versions of unenv and its dependencies.
const (
	unenvVersion           = "1.10.0"
	patheVersion           = "2.0.3"
	consolaVersion         = "3.4.2"
	defuVersion            = "6.1.4"
	nodeFetchNativeVersion = "1.6.6"
	mimeVersion            = "3.0.0"

	maxPolyfillDownloadSize = 50 * 1024 * 1024 // 50 MB
)

// polyfillPackages maps package names to their registry tarball URLs.
var polyfillPackages = []struct {
	name    string
	version string
}{
	{"unenv", unenvVersion},
	{"pathe", patheVersion},
	{"consola", consolaVersion},
	{"defu", defuVersion},
	{"node-fetch-native", nodeFetchNativeVersion},
	{"mime", mimeVersion},
}

// polyfillHashes maps download URLs to expected SHA-256 hex digests.
// Empty map means integrity checking is opt-in (hashes added as packages are pinned).
var polyfillHashes = map[string]string{
	// Hashes will be populated when package versions are pinned.
	// Example: "https://registry.npmjs.org/unenv/-/unenv-1.10.0.tgz": "abcdef...",
}

// EnsureUnenv downloads unenv and its dependencies from the npm registry
// into {dataDir}/polyfills/node_modules/ if not already present.
// Returns the path to the unenv package directory.
func EnsureUnenv(dataDir string) (string, error) {
	// Resolve to absolute path so esbuild can find the polyfills.
	absDataDir, err := filepath.Abs(dataDir)
	if err != nil {
		return "", fmt.Errorf("resolving data dir: %w", err)
	}
	nodeModules := filepath.Join(absDataDir, "polyfills", "node_modules")
	unenvDir := filepath.Join(nodeModules, "unenv")
	checkDir := filepath.Join(unenvDir, "runtime", "node")

	// Fast path: already downloaded.
	if info, err := os.Stat(checkDir); err == nil && info.IsDir() {
		return unenvDir, nil
	}

	log.Printf("Downloading unenv polyfills...")

	// Download + extract to a temp directory, then rename atomically.
	tmpDir, err := os.MkdirTemp(absDataDir, "polyfills-tmp-*")
	if err != nil {
		// absDataDir itself may not exist yet; create it and retry.
		if mkErr := os.MkdirAll(absDataDir, 0755); mkErr != nil {
			return "", fmt.Errorf("creating data dir %s: %w", absDataDir, mkErr)
		}
		tmpDir, err = os.MkdirTemp(absDataDir, "polyfills-tmp-*")
		if err != nil {
			return "", fmt.Errorf("creating temp dir: %w", err)
		}
	}
	defer func() { _ = os.RemoveAll(tmpDir) }() // clean up on any failure path

	tmpNodeModules := filepath.Join(tmpDir, "node_modules")
	if err := os.MkdirAll(tmpNodeModules, 0755); err != nil {
		return "", fmt.Errorf("creating temp node_modules: %w", err)
	}

	for _, pkg := range polyfillPackages {
		url := fmt.Sprintf("https://registry.npmjs.org/%s/-/%s-%s.tgz", pkg.name, pkg.name, pkg.version)
		destDir := filepath.Join(tmpNodeModules, pkg.name)
		if err := downloadAndExtract(url, destDir); err != nil {
			return "", fmt.Errorf("downloading %s@%s: %w", pkg.name, pkg.version, err)
		}
	}

	// Atomic move: ensure parent exists, then rename.
	finalDir := filepath.Join(absDataDir, "polyfills")
	if err := os.MkdirAll(filepath.Dir(finalDir), 0755); err != nil {
		return "", fmt.Errorf("creating parent dir: %w", err)
	}

	// Remove any existing partial polyfills directory.
	_ = os.RemoveAll(finalDir)

	if err := os.Rename(tmpDir, finalDir); err != nil {
		return "", fmt.Errorf("moving polyfills into place: %w", err)
	}

	log.Printf("Unenv polyfills installed to %s", finalDir)
	return filepath.Join(finalDir, "node_modules", "unenv"), nil
}

// downloadAndExtract fetches an npm tarball and extracts it to destDir,
// stripping the leading "package/" prefix that npm tarballs use.
func downloadAndExtract(url, destDir string) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("fetching %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetching %s: HTTP %d", url, resp.StatusCode)
	}

	// Read with size limit
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxPolyfillDownloadSize+1))
	if err != nil {
		return fmt.Errorf("reading %s: %w", url, err)
	}
	if int64(len(body)) > maxPolyfillDownloadSize {
		return fmt.Errorf("polyfill download too large: %s (>%d bytes)", url, maxPolyfillDownloadSize)
	}

	// Integrity check (if hash is configured for this URL)
	if expectedHash, ok := polyfillHashes[url]; ok {
		actualHash := sha256.Sum256(body)
		if hex.EncodeToString(actualHash[:]) != expectedHash {
			return fmt.Errorf("integrity check failed for %s: expected %s, got %s", url, expectedHash, hex.EncodeToString(actualHash[:]))
		}
	}

	// Decompress from memory
	gz, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("decompressing: %w", err)
	}
	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading tar: %w", err)
		}

		// Strip the "package/" prefix.
		name := hdr.Name
		if i := strings.IndexByte(name, '/'); i >= 0 {
			name = name[i+1:]
		}
		if name == "" {
			continue
		}

		target := filepath.Join(destDir, filepath.FromSlash(name))

		// Prevent path traversal.
		if !strings.HasPrefix(target, destDir+string(filepath.Separator)) && target != destDir {
			continue
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			f, err := os.Create(target)
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				_ = f.Close()
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}
		}
	}

	return nil
}
