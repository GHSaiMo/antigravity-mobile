//go:build windows

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
	"syscall"
	"time"

	"antigravity-mobile/internal/config"
	"antigravity-mobile/internal/procutil"
)

var (
	modShell32Service   = syscall.NewLazyDLL("shell32.dll")
	procIsUserAnAdminFn = modShell32Service.NewProc("IsUserAnAdmin")
)

type windowsManager struct {
	dataDir  string
	taskName string
	logPath  string
}

// NewManager returns the Windows Task Scheduler service manager.
func NewManager() (Manager, error) {
	dataDir := config.GetDataDir()
	return &windowsManager{
		dataDir:  dataDir,
		taskName: DefaultWindowsTaskName,
		logPath:  DefaultLogPath(dataDir),
	}, nil
}

func (m *windowsManager) BackendName() string { return "Windows Task Scheduler" }
func (m *windowsManager) ServiceName() string { return m.taskName }
func (m *windowsManager) ConfigPath() string  { return `\` + m.taskName }
func (m *windowsManager) LogPath() string     { return m.logPath }

func isWindowsElevated() bool {
	if procIsUserAnAdminFn.Find() != nil {
		return false
	}
	r, _, _ := procIsUserAnAdminFn.Call()
	return r != 0
}

func resolveWindowsUserID(ctx context.Context) (string, error) {
	if u, err := user.Current(); err == nil && strings.TrimSpace(u.Username) != "" {
		return strings.TrimSpace(u.Username), nil
	}
	out, err := runHiddenCmd(ctx, 5*time.Second, "whoami.exe")
	if err == nil {
		if trimmed := strings.TrimSpace(out); trimmed != "" {
			return trimmed, nil
		}
	}
	if domain, name := strings.TrimSpace(os.Getenv("USERDOMAIN")), strings.TrimSpace(os.Getenv("USERNAME")); name != "" {
		if domain != "" {
			return domain + `\` + name, nil
		}
		return name, nil
	}
	return "", fmt.Errorf("unable to determine current Windows user identity")
}

func findConhostPath() string {
	sysRoot := strings.TrimSpace(os.Getenv("SystemRoot"))
	if sysRoot == "" {
		sysRoot = `C:\Windows`
	}
	candidate := filepath.Join(sysRoot, "System32", "conhost.exe")
	if fi, err := os.Stat(candidate); err == nil && !fi.IsDir() {
		return candidate
	}
	return ""
}

func (m *windowsManager) Install(ctx context.Context, cfg InstallConfig) (InstallResult, error) {
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
	args, err := BuildServiceArgs(norm, true)
	if err != nil {
		return InstallResult{}, err
	}

	if err := os.MkdirAll(norm.DataDir, 0700); err != nil {
		return InstallResult{}, fmt.Errorf("create data directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(norm.LogPath), 0700); err != nil {
		return InstallResult{}, fmt.Errorf("create log directory: %w", err)
	}

	userID, err := resolveWindowsUserID(ctx)
	if err != nil {
		return InstallResult{}, err
	}

	conhostPath := findConhostPath()
	spec := WindowsTaskSpec{
		UserID:      userID,
		LogonOnly:   norm.WindowsLogonOnly,
		ConhostPath: conhostPath,
	}

	note := ""
	if err := m.registerTaskXML(ctx, norm, spec); err != nil {
		if !isWindowsElevated() {
			return InstallResult{}, fmt.Errorf("%w: %v", ErrElevationRequired, err)
		}
		if !norm.WindowsLogonOnly {
			// Some Microsoft Account / local-policy environments reject S4U; fall back cleanly to
			// InteractiveToken + HighestAvailable + AtLogOn (wrapped with conhost --headless to keep it invisible).
			fallbackSpec := WindowsTaskSpec{
				UserID:      userID,
				LogonOnly:   true,
				ConhostPath: conhostPath,
			}
			if fbErr := m.registerTaskXML(ctx, norm, fallbackSpec); fbErr != nil {
				return InstallResult{}, fmt.Errorf("register scheduled task failed (S4U: %v; Interactive fallback: %v)", err, fbErr)
			}
			note = "S4U registration was rejected by Windows account policy; automatically fell back to Interactive AtLogOn (headless)."
		} else {
			return InstallResult{}, err
		}
	}

	started := false
	if !norm.NoStart {
		// Stop any previous instance owned by the task before starting cleanly.
		_, _ = runHiddenCmd(ctx, 8*time.Second, "schtasks.exe", "/end", "/tn", m.taskName)
		time.Sleep(300 * time.Millisecond)
		if err := m.Start(ctx); err != nil {
			return InstallResult{}, err
		}
		started = true
	}

	return InstallResult{
		Backend:    m.BackendName(),
		Name:       m.ServiceName(),
		ConfigPath: m.ConfigPath(),
		LogPath:    norm.LogPath,
		BinaryPath: norm.BinaryPath,
		Args:       args,
		Started:    started,
		Note:       note,
	}, nil
}

func (m *windowsManager) registerTaskXML(ctx context.Context, cfg InstallConfig, spec WindowsTaskSpec) error {
	xmlText, err := RenderWindowsTaskXML(cfg, spec)
	if err != nil {
		return err
	}
	tmpFile, err := os.CreateTemp("", "mgy-task-*.xml")
	if err != nil {
		return fmt.Errorf("create temp task XML: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	payload := EncodeUTF16LEWithBOM(xmlText)
	if _, err := tmpFile.Write(payload); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("write temp task XML: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp task XML: %w", err)
	}

	out, err := runHiddenCmd(ctx, 15*time.Second, "schtasks.exe", "/create", "/tn", m.taskName, "/xml", tmpPath, "/f")
	if err != nil {
		return fmt.Errorf("schtasks /create failed: %w (%s)", err, strings.TrimSpace(out))
	}
	return nil
}

func (m *windowsManager) Uninstall(ctx context.Context) (bool, error) {
	if _, err := m.queryTaskXML(ctx); err != nil {
		return false, nil
	}
	_, _ = runHiddenCmd(ctx, 8*time.Second, "schtasks.exe", "/end", "/tn", m.taskName)
	time.Sleep(300 * time.Millisecond)

	out, err := runHiddenCmd(ctx, 12*time.Second, "schtasks.exe", "/delete", "/tn", m.taskName, "/f")
	if err != nil {
		if !isWindowsElevated() {
			return false, fmt.Errorf("%w: schtasks /delete failed: %v (%s)", ErrElevationRequired, err, strings.TrimSpace(out))
		}
		return false, fmt.Errorf("schtasks /delete failed: %w (%s)", err, strings.TrimSpace(out))
	}
	return true, nil
}

func (m *windowsManager) Start(ctx context.Context) error {
	if _, err := m.queryTaskXML(ctx); err != nil {
		return ErrNotInstalled
	}
	out, err := runHiddenCmd(ctx, 12*time.Second, "schtasks.exe", "/run", "/tn", m.taskName)
	if err != nil {
		if !isWindowsElevated() {
			return fmt.Errorf("%w: schtasks /run failed: %v (%s)", ErrElevationRequired, err, strings.TrimSpace(out))
		}
		return fmt.Errorf("schtasks /run failed: %w (%s)", err, strings.TrimSpace(out))
	}
	return nil
}

func (m *windowsManager) Stop(ctx context.Context) error {
	if _, err := m.queryTaskXML(ctx); err != nil {
		return ErrNotInstalled
	}
	out, err := runHiddenCmd(ctx, 12*time.Second, "schtasks.exe", "/end", "/tn", m.taskName)
	if err != nil {
		// schtasks /end returns non-zero when task is already stopped; verify current state before failing.
		if state := m.queryTaskState(ctx); state != "" && !strings.EqualFold(state, "Running") {
			return nil
		}
		if !isWindowsElevated() {
			return fmt.Errorf("%w: schtasks /end failed: %v (%s)", ErrElevationRequired, err, strings.TrimSpace(out))
		}
		return fmt.Errorf("schtasks /end failed: %w (%s)", err, strings.TrimSpace(out))
	}
	// Wait briefly for Task Scheduler to terminate the job object cleanly.
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		if state := m.queryTaskState(ctx); !strings.EqualFold(state, "Running") {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	return nil
}

func (m *windowsManager) Restart(ctx context.Context) error {
	if _, err := m.queryTaskXML(ctx); err != nil {
		return ErrNotInstalled
	}
	if err := m.Stop(ctx); err != nil {
		return err
	}
	time.Sleep(300 * time.Millisecond)
	return m.Start(ctx)
}

func (m *windowsManager) Status(ctx context.Context) (Status, error) {
	st := Status{
		Backend:    m.BackendName(),
		Name:       m.ServiceName(),
		ConfigPath: m.ConfigPath(),
		LogPath:    m.logPath,
		State:      "not installed",
	}

	rawXML, err := m.queryTaskXML(ctx)
	if err != nil {
		return st, nil
	}
	st.Installed = true
	st.State = "Ready"

	if meta, pErr := ParseWindowsTaskXMLMetadata(rawXML); pErr == nil {
		st.BinaryPath = meta.Command
		var parts []string
		if meta.LogonType != "" {
			parts = append(parts, meta.LogonType)
		}
		if meta.RunLevel != "" {
			parts = append(parts, meta.RunLevel)
		}
		switch {
		case meta.HasBootTrigger && meta.HasLogonTrigger:
			parts = append(parts, "Boot+Logon")
		case meta.HasBootTrigger:
			parts = append(parts, "Boot")
		case meta.HasLogonTrigger:
			parts = append(parts, "Logon")
		}
		st.Detail = strings.Join(parts, " · ")
	}

	if state := m.queryTaskState(ctx); state != "" {
		st.State = state
		st.Running = strings.EqualFold(state, "Running")
	}

	if st.Running {
		st.PID = findOtherMgyPID(ctx)
	}
	return st, nil
}

func (m *windowsManager) queryTaskXML(ctx context.Context) ([]byte, error) {
	cctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	cmd := procutil.HideWindow(exec.CommandContext(cctx, "schtasks.exe", "/query", "/tn", m.taskName, "/xml"))
	return cmd.Output()
}

func (m *windowsManager) queryTaskState(ctx context.Context) string {
	script := fmt.Sprintf("(Get-ScheduledTask -TaskName '%s' -ErrorAction Stop).State.ToString()", m.taskName)
	out, err := runHiddenCmd(ctx, 6*time.Second, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func findOtherMgyPID(ctx context.Context) int {
	selfPID := os.Getpid()
	script := fmt.Sprintf("(Get-CimInstance Win32_Process -Filter \"Name='mgy.exe'\" | Where-Object { $_.ProcessId -ne %d } | Select-Object -First 1 -ExpandProperty ProcessId)", selfPID)
	out, err := runHiddenCmd(ctx, 5*time.Second, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	if err != nil {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil || n <= 0 || n == selfPID {
		return 0
	}
	return n
}

func runHiddenCmd(ctx context.Context, timeout time.Duration, name string, args ...string) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := procutil.HideWindow(exec.CommandContext(cctx, name, args...))
	out, err := cmd.CombinedOutput()
	return string(out), err
}
