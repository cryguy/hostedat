package seaweedfs

import (
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/cryguy/hostedat/internal/config"
)

// Manager handles the SeaweedFS subprocess lifecycle in managed mode.
type Manager struct {
	Config config.ObjectStorageConfig
	Client *Client
	cmd    *exec.Cmd
}

// NewManager creates a new SeaweedFS manager.
func NewManager(cfg config.ObjectStorageConfig) *Manager {
	return &Manager{Config: cfg}
}

// Start starts the SeaweedFS subprocess and waits for health.
func (m *Manager) Start() error {
	if err := os.MkdirAll(m.Config.DataDir, 0755); err != nil {
		return fmt.Errorf("creating data dir: %w", err)
	}

	// Parse S3 port from endpoint.
	s3Port, err := parsePort(m.Config.S3Endpoint)
	if err != nil {
		return fmt.Errorf("parsing S3 endpoint port: %w", err)
	}

	// Pick non-conflicting ports for master and volume services.
	masterPort := s3Port + 1
	volumePort := s3Port + 2
	if err := ensurePortsAvailable(s3Port, masterPort, volumePort); err != nil {
		return err
	}

	weedBinary := m.Config.BinaryPath
	if weedBinary == "" {
		weedBinary = "weed"
	}

	m.cmd = exec.Command(weedBinary, "server", "-s3",
		"-dir="+m.Config.DataDir,
		fmt.Sprintf("-s3.port=%d", s3Port),
		fmt.Sprintf("-master.port=%d", masterPort),
		fmt.Sprintf("-volume.port=%d", volumePort),
		"-ip=127.0.0.1",
	)
	m.cmd.Stdout = os.Stdout
	m.cmd.Stderr = os.Stderr
	if err := m.cmd.Start(); err != nil {
		return fmt.Errorf("starting SeaweedFS: %w", err)
	}

	m.Client = NewClient(m.Config.S3Endpoint)

	if err := m.waitForHealth(30 * time.Second); err != nil {
		_ = m.Stop()
		return fmt.Errorf("SeaweedFS failed to become healthy: %w", err)
	}

	log.Printf("SeaweedFS started (pid %d), S3 at %s", m.cmd.Process.Pid, m.Config.S3Endpoint)
	return nil
}

// Stop gracefully shuts down the SeaweedFS subprocess.
func (m *Manager) Stop() error {
	if m.cmd == nil || m.cmd.Process == nil {
		return nil
	}
	if runtime.GOOS == "windows" {
		if err := m.cmd.Process.Kill(); err != nil {
			return err
		}
		_, _ = m.cmd.Process.Wait()
		return nil
	}

	if err := m.cmd.Process.Signal(os.Interrupt); err != nil {
		if killErr := m.cmd.Process.Kill(); killErr != nil {
			return killErr
		}
		_, _ = m.cmd.Process.Wait()
		return nil
	}
	done := make(chan error, 1)
	go func() { done <- m.cmd.Wait() }()
	select {
	case err := <-done:
		return err
	case <-time.After(10 * time.Second):
		return m.cmd.Process.Kill()
	}
}

// IsHealthy returns true if SeaweedFS responds to health checks.
func (m *Manager) IsHealthy() bool {
	if m.Client == nil {
		return false
	}
	return m.Client.Health() == nil
}

func (m *Manager) waitForHealth(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if m.Client.Health() == nil {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("timed out after %v", timeout)
}

func ensurePortsAvailable(ports ...int) error {
	for _, port := range ports {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			return fmt.Errorf("required SeaweedFS port %d is unavailable: %w", port, err)
		}
		_ = ln.Close()
	}
	return nil
}

func parsePort(endpoint string) (int, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return 0, err
	}
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			return 443, nil
		}
		return 80, nil
	}
	var p int
	_, err = fmt.Sscanf(port, "%d", &p)
	return p, err
}
