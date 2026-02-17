//go:build integration

package seaweedfs

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/cryguy/hostedat/internal/config"
)

func TestManagerIntegration_Lifecycle(t *testing.T) {
	weedBin := resolveWeedBinary(t)
	port := mustFreePort(t)

	mgr := NewManager(config.ObjectStorageConfig{
		Enabled:    true,
		Managed:    true,
		DataDir:    t.TempDir(),
		BinaryPath: weedBin,
		S3Endpoint: fmt.Sprintf("http://127.0.0.1:%d", port),
		Region:     "us-east-1",
	})

	if err := mgr.Start(); err != nil {
		t.Fatalf("manager start: %v", err)
	}
	if !mgr.IsHealthy() {
		t.Fatal("manager should report healthy after start")
	}
	if err := mgr.Stop(); err != nil {
		t.Fatalf("manager stop: %v", err)
	}
}

func TestManagerIntegration_StartFailureDiagnostics(t *testing.T) {
	weedBin := resolveWeedBinary(t)
	port := mustFreePort(t)

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatalf("listen for collision: %v", err)
	}
	defer ln.Close()

	mgr := NewManager(config.ObjectStorageConfig{
		Enabled:    true,
		Managed:    true,
		DataDir:    t.TempDir(),
		BinaryPath: weedBin,
		S3Endpoint: fmt.Sprintf("http://127.0.0.1:%d", port),
		Region:     "us-east-1",
	})

	err = mgr.Start()
	if err == nil {
		t.Fatal("expected manager start failure when S3 port is occupied")
	}
	if !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("expected collision diagnostic, got: %v", err)
	}
}

func TestManagerIntegration_IAMFlow(t *testing.T) {
	weedBin := resolveWeedBinary(t)
	port := mustFreePort(t)

	mgr := NewManager(config.ObjectStorageConfig{
		Enabled:    true,
		Managed:    true,
		DataDir:    t.TempDir(),
		BinaryPath: weedBin,
		S3Endpoint: fmt.Sprintf("http://127.0.0.1:%d", port),
		Region:     "us-east-1",
	})

	if err := mgr.Start(); err != nil {
		t.Fatalf("manager start: %v", err)
	}
	t.Cleanup(func() {
		_ = mgr.Stop()
	})

	userName := "itest-user"
	if err := mgr.Client.CreateUser(userName); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	key, err := mgr.Client.CreateAccessKey(userName)
	if err != nil {
		t.Fatalf("CreateAccessKey: %v", err)
	}

	policy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:*","Resource":["arn:aws:s3:::itest-*","arn:aws:s3:::itest-*/*"]}]}`
	if err := mgr.Client.PutUserPolicy(userName, "bucket-access", policy); err != nil {
		t.Fatalf("PutUserPolicy: %v", err)
	}

	if err := mgr.Client.DeleteAccessKey(key.AccessKeyID); err != nil {
		t.Fatalf("DeleteAccessKey: %v", err)
	}
	if err := mgr.Client.DeleteUser(userName); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
}

func resolveWeedBinary(t *testing.T) string {
	t.Helper()

	if env := strings.TrimSpace(os.Getenv("HOSTEDAT_WEED_BIN")); env != "" {
		if _, err := os.Stat(env); err == nil {
			return env
		}
		if _, err := exec.LookPath(env); err == nil {
			return env
		}
		t.Skipf("integration requires HOSTEDAT_WEED_BIN; not found: %s", env)
	}

	if runtime.GOOS == "windows" {
		candidates := []string{"./weed.exe", filepath.Join("..", "..", "weed.exe")}
		for _, p := range candidates {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
		t.Skip("integration requires weed.exe (set HOSTEDAT_WEED_BIN)")
	}

	if _, err := exec.LookPath("weed"); err == nil {
		return "weed"
	}
	t.Skip("integration requires 'weed' in PATH (or set HOSTEDAT_WEED_BIN)")
	return ""
}

func mustFreePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate free port: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}
