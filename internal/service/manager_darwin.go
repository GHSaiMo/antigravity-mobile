//go:build darwin

package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"antigravity-mobile/internal/config"
)

type darwinManager struct {
	homeDir   string
	dataDir   string
	plistPath string
	logPath   string
}

// NewManager returns the macOS launchd (LaunchAgent) service manager.
func NewManager() (Manager, error) {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return nil, fmt.Errorf("resolve home directory: %w", err)
	}
	dataDir := config.GetDataDir()
	return &darwinManager{
		homeDir:   home,
		dataDir:   dataDir,
		plistPath: DarwinPlistPath(home),
		logPath:   DefaultLogPath(dataDir),
	}, nil
}

func (m *darwinManager) BackendName() string { return "macOS launchd (LaunchAgent)" }
func (m *darwinManager) ServiceName() string { return DefaultDarwinLabel }
func (m *darwinManager) ConfigPath() string  { return m.plistPath }
func (m *darwinManager) LogPath() string     { return m.logPath }

func (m *darwinManager) domainTarget() string {
	return fmt.Sprintf("gui/%d", os.Getuid())
}

func (m *darwinManager) serviceTarget() string {
	return fmt.Sprintf("gui/%d/%s", os.Getuid(), DefaultDarwinLabel)
}

func (m *darwinManager) Install(ctx context.Context, cfg InstallConfig) (InstallResult, error) {
	if strings.TrimSpace(cfg.DataDir) == "" {
		cfg.DataDir = m.dataDir
	}
	if strings.TrimSpace(cfg.LogPath) == "" {
		cfg.LogPath = m.logPath
	}
	norm, err := NormalizeInstallConfig(cfg)
	if err != nil {
		return InstallResult{}, err
	}
	args, err := BuildServiceArgs(norm, false)
	if err != nil {
		return InstallResult{}, err
	}

	plistContent, err := RenderDarwinPlist(norm, m.homeDir)
	if err != nil {
		return InstallResult{}, err
	}

	if err := os.MkdirAll(filepath.Dir(m.plistPath), 0755); err != nil {
		return InstallResult{}, fmt.Errorf("create LaunchAgents directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(norm.LogPath), 0700); err != nil {
		return InstallResult{}, fmt.Errorf("create log directory: %w", err)
	}

	// Unload any previously bootstrapped agent cleanly before rewriting the plist.
	m.bootoutBestEffort(ctx)

	tmpPath := m.plistPath + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(plistContent), 0600); err != nil {
		return InstallResult{}, fmt.Errorf("write LaunchAgent plist: %w", err)
	}
	if err := os.Rename(tmpPath, m.plistPath); err != nil {
		_ = os.Remove(tmpPath)
		return InstallResult{}, fmt.Errorf("install LaunchAgent plist: %w", err)
	}

	started := false
	if !norm.NoStart {
		if err := m.Start(ctx); err != nil {
			return InstallResult{}, err
		}
		started = true
	}

	return InstallResult{
		Backend:    m.BackendName(),
		Name:       m.ServiceName(),
		ConfigPath: m.plistPath,
		LogPath:    norm.LogPath,
		BinaryPath: norm.BinaryPath,
		Args:       args,
		Started:    started,
	}, nil
}

func (m *darwinManager) Uninstall(ctx context.Context) (bool, error) {
	_, statErr := os.Stat(m.plistPath)
	existed := statErr == nil

	m.bootoutBestEffort(ctx)
	if existed {
		if err := os.Remove(m.plistPath); err != nil && !os.IsNotExist(err) {
			return false, fmt.Errorf("remove LaunchAgent plist: %w", err)
		}
	}
	return existed, nil
}

func (m *darwinManager) Start(ctx context.Context) error {
	if _, err := os.Stat(m.plistPath); err != nil {
		if os.IsNotExist(err) {
			return ErrNotInstalled
		}
		return err
	}

	// Try modern launchctl bootstrap first; if already loaded, kickstart it.
	if out, err := runCmd(ctx, 10*time.Second, "launchctl", "bootstrap", m.domainTarget(), m.plistPath); err != nil {
		// If already loaded or bootstrap rejected in non-GUI session, try kickstart then legacy load -w.
		if _, kickErr := runCmd(ctx, 10*time.Second, "launchctl", "kickstart", "-k", m.serviceTarget()); kickErr == nil {
			return nil
		}
		if outLegacy, legacyErr := runCmd(ctx, 10*time.Second, "launchctl", "load", "-w", m.plistPath); legacyErr != nil {
			return fmt.Errorf("launchctl bootstrap failed: %w (%s); fallback load failed: %v (%s)",
				err, strings.TrimSpace(out), legacyErr, strings.TrimSpace(outLegacy))
		}
		return nil
	}
	_, _ = runCmd(ctx, 10*time.Second, "launchctl", "kickstart", m.serviceTarget())
	return nil
}

func (m *darwinManager) Stop(ctx context.Context) error {
	if _, err := os.Stat(m.plistPath); err != nil {
		if os.IsNotExist(err) {
			return ErrNotInstalled
		}
		return err
	}
	m.bootoutBestEffort(ctx)
	return nil
}

func (m *darwinManager) Restart(ctx context.Context) error {
	if _, err := os.Stat(m.plistPath); err != nil {
		if os.IsNotExist(err) {
			return ErrNotInstalled
		}
		return err
	}
	if _, err := runCmd(ctx, 10*time.Second, "launchctl", "kickstart", "-k", m.serviceTarget()); err == nil {
		return nil
	}
	m.bootoutBestEffort(ctx)
	time.Sleep(300 * time.Millisecond)
	return m.Start(ctx)
}

func (m *darwinManager) Status(ctx context.Context) (Status, error) {
	st := Status{
		Backend:    m.BackendName(),
		Name:       m.ServiceName(),
		ConfigPath: m.plistPath,
		LogPath:    m.logPath,
		State:      "not installed",
	}

	raw, err := os.ReadFile(m.plistPath)
	if err != nil {
		if os.IsNotExist(err) {
			return st, nil
		}
		return st, err
	}
	st.Installed = true
	st.State = "stopped"
	st.BinaryPath = extractPlistFirstProgramArg(string(raw))

	if out, err := runCmd(ctx, 5*time.Second, "launchctl", "print", m.serviceTarget()); err == nil {
		running, pid, state := parseLaunchctlPrint(out)
		st.Running = running
		st.PID = pid
		if state != "" {
			st.State = state
		}
		st.Detail = "RunAtLoad + KeepAlive"
		return st, nil
	}

	if out, err := runCmd(ctx, 5*time.Second, "launchctl", "list", DefaultDarwinLabel); err == nil {
		if pid := parseLaunchctlListPID(out); pid > 0 {
			st.Running = true
			st.PID = pid
			st.State = "running"
		} else {
			st.State = "loaded (idle)"
		}
	}
	st.Detail = "RunAtLoad + KeepAlive"
	return st, nil
}

func (m *darwinManager) bootoutBestEffort(ctx context.Context) {
	uid := os.Getuid()
	_, _ = runCmd(ctx, 8*time.Second, "launchctl", "bootout", fmt.Sprintf("gui/%d", uid), m.plistPath)
	_, _ = runCmd(ctx, 8*time.Second, "launchctl", "bootout", fmt.Sprintf("gui/%d/%s", uid, DefaultDarwinLabel))
	_, _ = runCmd(ctx, 8*time.Second, "launchctl", "bootout", fmt.Sprintf("user/%d", uid), m.plistPath)
	_, _ = runCmd(ctx, 8*time.Second, "launchctl", "unload", "-w", m.plistPath)
}

func runCmd(ctx context.Context, timeout time.Duration, name string, args ...string) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	out, err := exec.CommandContext(cctx, name, args...).CombinedOutput()
	return string(out), err
}

func extractPlistFirstProgramArg(plist string) string {
	idx := strings.Index(plist, "<key>ProgramArguments</key>")
	if idx < 0 {
		return ""
	}
	rest := plist[idx:]
	start := strings.Index(rest, "<string>")
	if start < 0 {
		return ""
	}
	rest = rest[start+len("<string>"):]
	end := strings.Index(rest, "</string>")
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(rest[:end])
}

func parseLaunchctlPrint(out string) (running bool, pid int, state string) {
	for _, rawLine := range strings.Split(out, "\n") {
		line := strings.TrimSpace(rawLine)
		if strings.HasPrefix(line, "state = ") {
			state = strings.TrimSpace(strings.TrimPrefix(line, "state = "))
			if strings.EqualFold(state, "running") {
				running = true
			}
		} else if strings.HasPrefix(line, "pid = ") {
			if n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "pid = "))); err == nil && n > 0 {
				pid = n
				running = true
			}
		}
	}
	return running, pid, state
}

func parseLaunchctlListPID(out string) int {
	for _, rawLine := range strings.Split(out, "\n") {
		line := strings.TrimSpace(rawLine)
		if strings.Contains(line, `"PID"`) {
			parts := strings.Split(line, "=")
			if len(parts) == 2 {
				val := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(parts[1]), ";"))
				if n, err := strconv.Atoi(val); err == nil && n > 0 {
					return n
				}
			}
		}
	}
	return 0
}
