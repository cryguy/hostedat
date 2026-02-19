package seaweedfs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cryguy/hostedat/internal/config"
)

func TestDownloadURL(t *testing.T) {
	tests := []struct {
		name    string
		version string
		goos    string
		goarch  string
		want    string
		wantErr bool
	}{
		{
			name:    "linux/amd64",
			version: "4.13",
			goos:    "linux",
			goarch:  "amd64",
			want:    "https://github.com/seaweedfs/seaweedfs/releases/download/4.13/linux_amd64.tar.gz",
		},
		{
			name:    "linux/arm64",
			version: "4.13",
			goos:    "linux",
			goarch:  "arm64",
			want:    "https://github.com/seaweedfs/seaweedfs/releases/download/4.13/linux_arm64.tar.gz",
		},
		{
			name:    "darwin/amd64",
			version: "4.13",
			goos:    "darwin",
			goarch:  "amd64",
			want:    "https://github.com/seaweedfs/seaweedfs/releases/download/4.13/darwin_amd64.tar.gz",
		},
		{
			name:    "darwin/arm64",
			version: "4.13",
			goos:    "darwin",
			goarch:  "arm64",
			want:    "https://github.com/seaweedfs/seaweedfs/releases/download/4.13/darwin_arm64.tar.gz",
		},
		{
			name:    "windows/amd64",
			version: "4.13",
			goos:    "windows",
			goarch:  "amd64",
			want:    "https://github.com/seaweedfs/seaweedfs/releases/download/4.13/windows_amd64.exe.zip",
		},
		{
			name:    "unsupported platform",
			version: "4.13",
			goos:    "freebsd",
			goarch:  "amd64",
			wantErr: true,
		},
		{
			name:    "unsupported arch",
			version: "4.13",
			goos:    "linux",
			goarch:  "mips",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := downloadURL(tt.version, tt.goos, tt.goarch)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("downloadURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEnsureBinary_ExistingBinary(t *testing.T) {
	// Create a temp directory with a fake binary.
	tmpDir := t.TempDir()
	fakeBinary := filepath.Join(tmpDir, "weed")
	if err := os.WriteFile(fakeBinary, []byte("fake"), 0755); err != nil {
		t.Fatalf("writing fake binary: %v", err)
	}

	cfg := config.ObjectStorageConfig{
		BinaryPath: fakeBinary,
	}

	got, err := EnsureBinary(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != fakeBinary {
		t.Errorf("EnsureBinary() = %q, want %q", got, fakeBinary)
	}
}

func TestEnsureBinary_Download(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping download test in short mode")
	}

	tmpDir := t.TempDir()
	cfg := config.ObjectStorageConfig{
		DataDir: tmpDir,
	}

	got, err := EnsureBinary(cfg)
	if err != nil {
		t.Fatalf("EnsureBinary() error: %v", err)
	}

	// Verify the binary was placed in the expected location.
	if _, err := os.Stat(got); err != nil {
		t.Fatalf("downloaded binary not found at %q: %v", got, err)
	}
}
