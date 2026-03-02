package seaweedfs

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/cryguy/hostedat/internal/config"
)

// Manager handles the SeaweedFS subprocess lifecycle in managed mode.
type Manager struct {
	Config config.ObjectStorageConfig
	Client *Client // IAM client (pointed at the S3/IAM endpoint)
	cmd    *exec.Cmd

	// FilerEndpoint is the filer HTTP address.
	FilerEndpoint string

	s3HealthClient *Client // used to check S3 readiness

	// AccessKeyID and SecretAccessKey are the admin credentials used to
	// authenticate with the managed SeaweedFS instance. They are either
	// read from config, loaded from a persisted s3.config.json, or
	// generated on first run.
	AccessKeyID     string
	SecretAccessKey string
}

// s3ConfigFile is the SeaweedFS S3 config structure.
type s3ConfigFile struct {
	Identities []s3Identity `json:"identities"`
}

type s3Identity struct {
	Name        string         `json:"name"`
	Credentials []s3Credential `json:"credentials"`
	Actions     []string       `json:"actions"`
}

type s3Credential struct {
	AccessKey string `json:"accessKey"`
	SecretKey string `json:"secretKey"`
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

	// Pick non-conflicting ports for master, volume, and filer services.
	// IAM is embedded in the S3 server (no separate port needed).
	masterPort := s3Port + 1
	volumePort := s3Port + 2
	filerPort := s3Port + 3

	// Kill any stale SeaweedFS process left behind by a previous crash.
	m.cleanupStalePID(s3Port)

	if err := ensurePortsAvailable(s3Port, masterPort, volumePort, filerPort); err != nil {
		return err
	}

	// Ensure S3 credentials exist (from config, persisted file, or generated).
	s3ConfigPath, err := m.ensureS3Config()
	if err != nil {
		return fmt.Errorf("setting up S3 credentials: %w", err)
	}

	weedBinary, err := EnsureBinary(m.Config)
	if err != nil {
		return fmt.Errorf("ensuring weed binary: %w", err)
	}

	args := []string{"server", "-s3",
		"-dir=" + m.Config.DataDir,
		fmt.Sprintf("-s3.port=%d", s3Port),
		fmt.Sprintf("-master.port=%d", masterPort),
		fmt.Sprintf("-volume.port=%d", volumePort),
		fmt.Sprintf("-filer.port=%d", filerPort),
		"-s3.iam.readOnly=false",
		"-s3.port.iceberg=0",
		"-volume.max=0",
		"-ip=127.0.0.1",
		"-s3.config=" + s3ConfigPath,
	}
	if m.Config.DomainName != "" {
		args = append(args, "-s3.domainName="+m.Config.DomainName)
	}
	m.cmd = exec.Command(weedBinary, args...)
	m.cmd.Stdout = os.Stdout
	m.cmd.Stderr = os.Stderr
	if err := m.cmd.Start(); err != nil {
		return fmt.Errorf("starting SeaweedFS: %w", err)
	}
	m.writePID()

	m.FilerEndpoint = fmt.Sprintf("http://127.0.0.1:%d", filerPort)

	// The IAM API is embedded in the S3 server, so the IAM client
	// points at the same endpoint and signs requests with SigV4.
	m.Client = NewClientWithAuth(m.Config.S3Endpoint, m.AccessKeyID, m.SecretAccessKey, m.Config.Region)

	// Wait for the S3 endpoint (not IAM) to become healthy, since S3
	// is what callers (minio client) actually connect to.
	m.s3HealthClient = NewClient(m.Config.S3Endpoint)
	if err := m.waitForHealth(45 * time.Second); err != nil {
		_ = m.Stop()
		return fmt.Errorf("SeaweedFS failed to become healthy: %w", err)
	}

	log.Printf("SeaweedFS started (pid %d), S3 at %s", m.cmd.Process.Pid, m.Config.S3Endpoint)
	return nil
}

// ensureS3Config ensures S3 admin credentials are available and the
// s3.config.json file exists. Credentials are resolved in order:
// 1. From the application config (auth.access_key_id / secret_access_key)
// 2. From an existing persisted s3.config.json in the data directory
// 3. Generated fresh and persisted for subsequent restarts
func (m *Manager) ensureS3Config() (string, error) {
	configPath := filepath.Join(m.Config.DataDir, "s3.config.json")

	// If credentials are in the application config, use those.
	if m.Config.Auth.AccessKeyID != "" && m.Config.Auth.SecretAccessKey != "" {
		m.AccessKeyID = m.Config.Auth.AccessKeyID
		m.SecretAccessKey = m.Config.Auth.SecretAccessKey
		return m.writeS3Config(configPath)
	}

	// Try loading from an existing persisted config.
	if data, err := os.ReadFile(configPath); err == nil {
		var cfg s3ConfigFile
		if err := json.Unmarshal(data, &cfg); err == nil {
			for _, id := range cfg.Identities {
				if id.Name == "admin" && len(id.Credentials) > 0 {
					m.AccessKeyID = id.Credentials[0].AccessKey
					m.SecretAccessKey = id.Credentials[0].SecretKey
					log.Printf("Loaded existing S3 credentials from %s", configPath)
					return configPath, nil
				}
			}
		}
	}

	// Generate new credentials.
	accessKey, err := randomHex(16)
	if err != nil {
		return "", fmt.Errorf("generating access key: %w", err)
	}
	secretKey, err := randomHex(32)
	if err != nil {
		return "", fmt.Errorf("generating secret key: %w", err)
	}
	m.AccessKeyID = accessKey
	m.SecretAccessKey = secretKey
	log.Printf("Generated new S3 admin credentials (persisted to %s)", configPath)
	return m.writeS3Config(configPath)
}

func (m *Manager) writeS3Config(path string) (string, error) {
	cfg := s3ConfigFile{
		Identities: []s3Identity{
			{
				Name: "admin",
				Credentials: []s3Credential{
					{
						AccessKey: m.AccessKeyID,
						SecretKey: m.SecretAccessKey,
					},
				},
				Actions: []string{
					"Admin",
					"Read",
					"Write",
					"List",
					"Tagging",
					"Lock",
				},
			},
		},
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return "", err
	}
	return path, nil
}

// Stop gracefully shuts down the SeaweedFS subprocess.
func (m *Manager) Stop() error {
	defer m.removePID()

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

func (m *Manager) pidFilePath() string {
	return filepath.Join(m.Config.DataDir, "weed.pid")
}

func (m *Manager) writePID() {
	_ = os.WriteFile(m.pidFilePath(), []byte(fmt.Sprintf("%d", m.cmd.Process.Pid)), 0600)
}

func (m *Manager) removePID() {
	_ = os.Remove(m.pidFilePath())
}

// cleanupStalePID kills a SeaweedFS process left behind by a previous crash.
// It only kills if both a PID file exists AND the S3 port is still occupied,
// avoiding false positives from reused PIDs.
func (m *Manager) cleanupStalePID(s3Port int) {
	pidPath := m.pidFilePath()
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return // No PID file — clean state.
	}
	defer func() { _ = os.Remove(pidPath) }()

	var pid int
	if _, err := fmt.Sscanf(string(data), "%d", &pid); err != nil || pid <= 0 {
		return
	}

	// Only kill if the S3 port is actually occupied — if the port is free,
	// the old process already exited and we just clean up the stale PID file.
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", s3Port))
	if err == nil {
		_ = ln.Close()
		return
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return
	}

	log.Printf("Killing stale SeaweedFS process (pid %d) from previous run", pid)
	if runtime.GOOS == "windows" {
		_ = proc.Kill()
	} else {
		_ = proc.Signal(os.Interrupt)
		time.Sleep(2 * time.Second)
		_ = proc.Kill()
	}
	// Best-effort wait — fails for non-child processes, which is fine.
	_, _ = proc.Wait()

	// Wait briefly for the port to be released.
	for i := 0; i < 10; i++ {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", s3Port))
		if err == nil {
			_ = ln.Close()
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// IsHealthy returns true if the SeaweedFS S3 endpoint responds to health checks.
func (m *Manager) IsHealthy() bool {
	if m.s3HealthClient == nil {
		return false
	}
	return m.s3HealthClient.Health() == nil
}

func (m *Manager) waitForHealth(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if m.s3HealthClient.Health() == nil {
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

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
