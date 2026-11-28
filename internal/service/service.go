package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"antigravity-mobile/internal/config"
	"antigravity-mobile/internal/localtls"
)

const (
	// DefaultDarwinLabel is the macOS LaunchAgent label (matches scripts/uninstall.sh *multigravity*.plist).
	DefaultDarwinLabel = "com.multigravity.gateway"
	// DefaultLinuxUnit is the Linux systemd user service unit (matches scripts/uninstall.sh mgy.service).
	DefaultLinuxUnit = "mgy.service"
	// DefaultWindowsTaskName is the Windows Task Scheduler task name (matches scripts/uninstall.ps1).
	DefaultWindowsTaskName = "MultigravityGateway"
)

var (
	// ErrNotInstalled is returned when start/stop/restart is invoked before the service is installed.
	ErrNotInstalled = errors.New("service is not installed")
	// ErrElevationRequired indicates Administrator privileges are required (Windows Task Scheduler).
	ErrElevationRequired = errors.New("administrator privileges required")
	// ErrUnsupportedPlatform indicates the host OS has no native service manager implementation.
	ErrUnsupportedPlatform = errors.New("unsupported platform")
)

// InstallConfig defines how mgy should be registered with the OS service manager.
type InstallConfig struct {
	BinaryPath       string
	DataDir          string
	LogPath          string
	Mode             string // "run" (default), "hub", or "node"
	Port             int    // 0 means inherit MULTIGRAVITY_PORT / default 58900 dynamically
	Host             string // empty means inherit MULTIGRAVITY_HOST dynamically
	EnableIPv6       bool   // Passes -ipv6 for explicit IPv4/IPv6 dual-stack listening
	Headless         bool   // Hub mode only: passes -headless
	Name             string // Hub/Node mode: passes -name
	HubURL           string // Node mode: passes -hub
	NoDiscovery      bool   // Passes -no-discovery
	WindowsLogonOnly bool   // Windows only: InteractiveToken + AtLogOn instead of S4U + AtStartup
	NoStart          bool   // Register auto-start without starting immediately
}

// InstallResult describes the outcome of installing the native background service.
type InstallResult struct {
	Backend    string
	Name       string
	ConfigPath string
	LogPath    string
	BinaryPath string
	Args       []string
	Started    bool
	Note       string
}

// Status summarizes the state of the OS-managed background service.
type Status struct {
	Backend    string
	Name       string
	ConfigPath string
	LogPath    string
	BinaryPath string
	Installed  bool
	Running    bool
	PID        int
	State      string
	Detail     string
}

// Manager manages the native auto-start background service for the current OS.
type Manager interface {
	BackendName() string
	ServiceName() string
	ConfigPath() string
	LogPath() string
	Install(ctx context.Context, cfg InstallConfig) (InstallResult, error)
	Uninstall(ctx context.Context) (removed bool, err error)
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Restart(ctx context.Context) error
	Status(ctx context.Context) (Status, error)
}

// DefaultLogPath returns the standard service log file path under dataDir.
func DefaultLogPath(dataDir string) string {
	if strings.TrimSpace(dataDir) == "" {
		dataDir = config.GetDataDir()
	}
	return filepath.Join(dataDir, "logs", "service.log")
}

// NormalizeMode validates and normalizes the gateway run mode ("run", "hub", or "node").
func NormalizeMode(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "run", "standalone", "default":
		return "run", nil
	case "hub":
		return "hub", nil
	case "node":
		return "node", nil
	default:
		return "", fmt.Errorf("invalid service mode %q (expected run, hub, or node)", raw)
	}
}

func containsUnsafeControl(s string) bool {
	for _, r := range s {
		if r == 0 || r == '\r' || r == '\n' {
			return true
		}
	}
	return false
}

// NormalizeInstallConfig fills default paths and validates fields before manifest generation.
func NormalizeInstallConfig(cfg InstallConfig) (InstallConfig, error) {
	mode, err := NormalizeMode(cfg.Mode)
	if err != nil {
		return cfg, err
	}
	cfg.Mode = mode

	if cfg.Port < 0 || cfg.Port > 65535 {
		return cfg, fmt.Errorf("invalid port %d (must be 0..65535)", cfg.Port)
	}

	if strings.TrimSpace(cfg.DataDir) == "" {
		cfg.DataDir = config.GetDataDir()
	}
	if strings.TrimSpace(cfg.LogPath) == "" {
		cfg.LogPath = DefaultLogPath(cfg.DataDir)
	}

	for field, val := range map[string]string{
		"binary":  cfg.BinaryPath,
		"dataDir": cfg.DataDir,
		"logPath": cfg.LogPath,
		"host":    cfg.Host,
		"name":    cfg.Name,
		"hub":     cfg.HubURL,
	} {
		if containsUnsafeControl(val) {
			return cfg, fmt.Errorf("field %s contains invalid control characters", field)
		}
	}
	if strings.TrimSpace(cfg.BinaryPath) == "" {
		return cfg, fmt.Errorf("binary path is required")
	}
	return cfg, nil
}

// BuildServiceArgs constructs the CLI arguments passed to the mgy binary by the OS service manager.
func BuildServiceArgs(cfg InstallConfig, includeLogFile bool) ([]string, error) {
	norm, err := NormalizeInstallConfig(cfg)
	if err != nil {
		return nil, err
	}

	args := []string{norm.Mode, "-qr=false"}
	if norm.Port > 0 {
		args = append(args, "-port", strconv.Itoa(norm.Port))
	}
	if h := strings.TrimSpace(norm.Host); h != "" {
		args = append(args, "-host", h)
	}
	if norm.EnableIPv6 {
		args = append(args, "-ipv6")
	}
	if norm.Mode == "hub" && norm.Headless {
		args = append(args, "-headless")
	}
	if (norm.Mode == "hub" || norm.Mode == "node") && strings.TrimSpace(norm.Name) != "" {
		args = append(args, "-name", strings.TrimSpace(norm.Name))
	}
	if norm.Mode == "node" && strings.TrimSpace(norm.HubURL) != "" {
		args = append(args, "-hub", strings.TrimSpace(norm.HubURL))
	}
	if norm.NoDiscovery {
		args = append(args, "-no-discovery")
	}
	if includeLogFile && strings.TrimSpace(norm.LogPath) != "" {
		args = append(args, "-log-file", norm.LogPath)
	}
	return args, nil
}

// ResolveBinaryPath resolves the mgy executable path to register in the service definition.
func ResolveBinaryPath(override string) (string, error) {
	candidate := strings.TrimSpace(override)
	if candidate == "" {
		exe, err := os.Executable()
		if err != nil {
			return "", fmt.Errorf("resolve current executable: %w", err)
		}
		candidate = exe
		if resolved, err := filepath.EvalSymlinks(exe); err == nil && resolved != "" {
			candidate = resolved
		}
		if runtime.GOOS == "windows" {
			candidate = preferWindowsLocalBin(candidate)
		}
	}
	abs, err := filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve binary path %q: %w", candidate, err)
	}
	fi, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("binary not accessible at %s: %w", abs, err)
	}
	if fi.IsDir() {
		return "", fmt.Errorf("binary path is a directory: %s", abs)
	}
	return abs, nil
}

func preferWindowsLocalBin(exePath string) string {
	lower := strings.ToLower(filepath.ToSlash(exePath))
	if !strings.Contains(lower, "/microsoft/windowsapps/") {
		return exePath
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return exePath
	}
	canonical := filepath.Join(home, ".local", "bin", "mgy.exe")
	if fi, err := os.Stat(canonical); err == nil && !fi.IsDir() {
		return canonical
	}
	return exePath
}

// IsInstalled reports whether the native background service is currently registered.
func IsInstalled(ctx context.Context) bool {
	mgr, err := NewManager()
	if err != nil {
		return false
	}
	st, err := mgr.Status(ctx)
	return err == nil && st.Installed
}

// ProbeHealth checks whether a local mgy gateway is answering /healthz on the given port.
func ProbeHealth(port int) (healthy bool, version string) {
	if port <= 0 {
		port = 58900
	}
	client := &http.Client{
		Timeout:   900 * time.Millisecond,
		Transport: localtls.NewLoopbackTransport(),
	}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/healthz", port))
	if err != nil {
		return false, ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, ""
	}
	var payload struct {
		Status  string `json:"status"`
		Version string `json:"version"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&payload); err == nil {
		return true, strings.TrimSpace(payload.Version)
	}
	return true, ""
}

// TailLogs reads up to the last n lines from path (bounded to the trailing 512 KiB).
func TailLogs(path string, n int) ([]string, error) {
	if n <= 0 {
		n = 80
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	const maxRead int64 = 512 * 1024
	start := int64(0)
	if fi.Size() > maxRead {
		start = fi.Size() - maxRead
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	if start > 0 {
		// Drop the partial first line when seeking into the middle of the file.
		if idx := bytes.IndexByte(raw, '\n'); idx >= 0 {
			raw = raw[idx+1:]
		}
	}

	var lines []string
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines, nil
}

// FollowLogs writes the last initialLines from path and streams subsequent appends until ctx is cancelled.
func FollowLogs(ctx context.Context, path string, initialLines int, out io.Writer) error {
	lines, err := TailLogs(path, initialLines)
	if err != nil {
		return err
	}
	for _, line := range lines {
		if _, wErr := fmt.Fprintln(out, line); wErr != nil {
			return wErr
		}
	}

	var offset int64
	if fi, err := os.Stat(path); err == nil {
		offset = fi.Size()
	}

	ticker := time.NewTicker(400 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			fi, err := os.Stat(path)
			if err != nil {
				continue
			}
			if fi.Size() < offset {
				// File was truncated or rotated.
				offset = 0
			}
			if fi.Size() == offset {
				continue
			}
			f, err := os.Open(path)
			if err != nil {
				continue
			}
			if _, err := f.Seek(offset, io.SeekStart); err == nil {
				chunk, rErr := io.ReadAll(io.LimitReader(f, 1024*1024))
				if rErr == nil && len(chunk) > 0 {
					offset += int64(len(chunk))
					if _, wErr := out.Write(chunk); wErr != nil {
						_ = f.Close()
						return wErr
					}
				}
			}
			_ = f.Close()
		}
	}
}
