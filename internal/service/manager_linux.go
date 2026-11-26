//go:build linux

package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"antigravity-mobile/internal/config"
)

type linuxManager struct {
	homeDir  string
	dataDir  string
	unitPath string
	logPath  string
}

// NewManager returns the Linux systemd (--user) service manager.
func NewManager() (Manager, error) {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return nil, fmt.Errorf("resolve home directory: %w", err)
	}
	dataDir := config.GetDataDir()
	return &linuxManager{
		homeDir:  home,
		dataDir:  dataDir,
		unitPath: LinuxUnitPath(home),
		logPath:  DefaultLogPath(dataDir),
	}, nil
}

func (m *linuxManager) BackendName() string { return "Linux systemd (--user)" }
func (m *linuxManager) ServiceName() string { return DefaultLinuxUnit }
func (m *linuxManager) ConfigPath() string  { return m.unitPath }
func (m *linuxManager) LogPath() string     { return m.logPath }

func (m *linuxManager) Install(ctx context.Context, cfg InstallConfig) (InstallResult, error) {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return InstallResult{}, fmt.Errorf("systemctl not found in PATH: %w", err)
	}
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

	unitContent, err := RenderLinuxUnit(norm, m.homeDir)
	if err != nil {
		return InstallResult{}, err
	}

	if err := os.MkdirAll(filepath.Dir(m.unitPath), 0755); err != nil {
		return InstallResult{}, fmt.Errorf("create systemd user directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(norm.LogPath), 0700); err != nil {
		return InstallResult{}, fmt.Errorf("create log directory: %w", err)
	}

	tmpPath := m.unitPath + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(unitContent), 0600); err != nil {
		return InstallResult{}, fmt.Errorf("write systemd unit: %w", err)
	}
	if err := os.Rename(tmpPath, m.unitPath); err != nil {
		_ = os.Remove(tmpPath)
		return InstallResult{}, fmt.Errorf("install systemd unit: %w", err)
	}

	if out, err := runSystemctl(ctx, "daemon-reload"); err != nil {
		return InstallResult{}, fmt.Errorf("systemctl --user daemon-reload failed: %w (%s)", err, strings.TrimSpace(out))
	}

	started := false
	if norm.NoStart {
		if out, err := runSystemctl(ctx, "enable", DefaultLinuxUnit); err != nil {
			return InstallResult{}, fmt.Errorf("systemctl --user enable failed: %w (%s)", err, strings.TrimSpace(out))
		}
	} else {
		if out, err := runSystemctl(ctx, "enable", "--now", DefaultLinuxUnit); err != nil {
			return InstallResult{}, fmt.Errorf("systemctl --user enable --now failed: %w (%s)", err, strings.TrimSpace(out))
		}
		// Ensure updated flags/binary take effect even if unit was already active.
		_, _ = runSystemctl(ctx, "restart", DefaultLinuxUnit)
		started = true
	}

	note := ""
	if u, err := user.Current(); err == nil && u.Username != "" {
		lingerPath := filepath.Join("/var/lib/systemd/linger", u.Username)
		if _, err := os.Stat(lingerPath); os.IsNotExist(err) {
			note = fmt.Sprintf("For boot-before-login persistence on headless servers, run: loginctl enable-linger %s", u.Username)
		}
	}

	return InstallResult{
		Backend:    m.BackendName(),
		Name:       m.ServiceName(),
		ConfigPath: m.unitPath,
		LogPath:    norm.LogPath,
		BinaryPath: norm.BinaryPath,
		Args:       args,
		Started:    started,
		Note:       note,
	}, nil
}

func (m *linuxManager) Uninstall(ctx context.Context) (bool, error) {
	_, statErr := os.Stat(m.unitPath)
	existed := statErr == nil

	_, _ = runSystemctl(ctx, "disable", "--now", DefaultLinuxUnit)
	if existed {
		if err := os.Remove(m.unitPath); err != nil && !os.IsNotExist(err) {
			return false, fmt.Errorf("remove systemd unit: %w", err)
		}
	}
	_, _ = runSystemctl(ctx, "daemon-reload")
	return existed, nil
}

func (m *linuxManager) Start(ctx context.Context) error {
	if _, err := os.Stat(m.unitPath); err != nil {
		if os.IsNotExist(err) {
			return ErrNotInstalled
		}
		return err
	}
	if out, err := runSystemctl(ctx, "start", DefaultLinuxUnit); err != nil {
		return fmt.Errorf("systemctl --user start failed: %w (%s)", err, strings.TrimSpace(out))
	}
	return nil
}

func (m *linuxManager) Stop(ctx context.Context) error {
	if _, err := os.Stat(m.unitPath); err != nil {
		if os.IsNotExist(err) {
			return ErrNotInstalled
		}
		return err
	}
	if out, err := runSystemctl(ctx, "stop", DefaultLinuxUnit); err != nil {
		return fmt.Errorf("systemctl --user stop failed: %w (%s)", err, strings.TrimSpace(out))
	}
	return nil
}

func (m *linuxManager) Restart(ctx context.Context) error {
	if _, err := os.Stat(m.unitPath); err != nil {
		if os.IsNotExist(err) {
			return ErrNotInstalled
		}
		return err
	}
	if out, err := runSystemctl(ctx, "restart", DefaultLinuxUnit); err != nil {
		return fmt.Errorf("systemctl --user restart failed: %w (%s)", err, strings.TrimSpace(out))
	}
	return nil
}

func (m *linuxManager) Status(ctx context.Context) (Status, error) {
	st := Status{
		Backend:    m.BackendName(),
		Name:       m.ServiceName(),
		ConfigPath: m.unitPath,
		LogPath:    m.logPath,
		State:      "not installed",
	}

	raw, err := os.ReadFile(m.unitPath)
	if err != nil {
		if os.IsNotExist(err) {
			return st, nil
		}
		return st, err
	}
	st.Installed = true
	st.State = "inactive"
	st.BinaryPath = extractSystemdExecBinary(string(raw))

	out, err := runSystemctl(ctx, "show", "--no-pager",
		"-p", "ActiveState",
		"-p", "SubState",
		"-p", "MainPID",
		"-p", "UnitFileState",
		DefaultLinuxUnit)
	if err == nil {
		var activeState, subState, unitState string
		for _, line := range strings.Split(out, "\n") {
			k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
			if !ok {
				continue
			}
			switch k {
			case "ActiveState":
				activeState = v
			case "SubState":
				subState = v
			case "MainPID":
				if n, err := strconv.Atoi(v); err == nil && n > 0 {
					st.PID = n
				}
			case "UnitFileState":
				unitState = v
			}
		}
		if activeState != "" {
			st.State = activeState
			if subState != "" {
				st.State = fmt.Sprintf("%s (%s)", activeState, subState)
			}
			st.Running = activeState == "active" || activeState == "activating"
		}
		if unitState != "" {
			st.Detail = "unit=" + unitState + ", Restart=always"
		}
	}
	return st, nil
}

func runSystemctl(ctx context.Context, args ...string) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	fullArgs := append([]string{"--user"}, args...)
	out, err := exec.CommandContext(cctx, "systemctl", fullArgs...).CombinedOutput()
	return string(out), err
}

func extractSystemdExecBinary(unit string) string {
	for _, rawLine := range strings.Split(unit, "\n") {
		line := strings.TrimSpace(rawLine)
		if !strings.HasPrefix(line, "ExecStart=") {
			continue
		}
		val := strings.TrimSpace(strings.TrimPrefix(line, "ExecStart="))
		if strings.HasPrefix(val, `"`) {
			if end := strings.Index(val[1:], `"`); end >= 0 {
				return val[1 : 1+end]
			}
		}
		if fields := strings.Fields(val); len(fields) > 0 {
			return fields[0]
		}
	}
	return ""
}
