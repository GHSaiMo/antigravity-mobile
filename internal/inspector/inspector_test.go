package inspector

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestRegexParsing(t *testing.T) {
	samplePs := `89479 /Applications/Antigravity.app/Contents/Resources/bin/language_server --standalone --override_ide_name antigravity --csrf_token 5aacbcc0-cf40-4b94-8f17-986485a8d0d5 --app_data_dir antigravity`
	
	matches := csrfRegex.FindStringSubmatch(samplePs)
	if len(matches) < 2 {
		t.Fatalf("expected csrf token match, got none")
	}
	expected := "5aacbcc0-cf40-4b94-8f17-986485a8d0d5"
	if matches[1] != expected {
		t.Errorf("expected %s, got %s", expected, matches[1])
	}

	sampleLsof := `COMMAND     PID    USER   FD   TYPE             DEVICE SIZE/OFF NODE NAME
language_ 89479 user        7u  IPv4 0xe146a107dbd3a758      0t0  TCP 127.0.0.1:62226 (LISTEN)
language_ 89479 user        8u  IPv4 0x43b49ab6df4a4122      0t0  TCP 127.0.0.1:62227 (LISTEN)
`
	portMatches := lsofRegex.FindAllStringSubmatch(sampleLsof, -1)
	if len(portMatches) != 2 {
		t.Fatalf("expected 2 port matches, got %d", len(portMatches))
	}
	if portMatches[0][1] != "62226" {
		t.Errorf("expected port 62226, got %s", portMatches[0][1])
	}
	if portMatches[1][1] != "62227" {
		t.Errorf("expected port 62227, got %s", portMatches[1][1])
	}
}

func TestLiveScan(t *testing.T) {
	insp := NewInspector(10 * time.Second)
	info := insp.Scan()
	if info == nil {
		t.Skip("Antigravity language_server not running, skipping live scan test")
	}
	if info.Port == 0 {
		t.Fatalf("invalid scanned info: %+v", info)
	}
	t.Logf("Live discovery succeeded: Port=%d, CSRF=%s, PID=%d", info.Port, info.CSRFToken, info.PID)
}

func TestDaemonDiscovery(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/exa.language_server_pb.LanguageServerService/GetStatus" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("{}"))
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	u, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("failed to parse url: %v", err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatalf("failed to parse port: %v", err)
	}

	tmpDir := t.TempDir()
	daemonDirOverride = tmpDir
	t.Cleanup(func() {
		daemonDirOverride = ""
	})

	jsonContent := fmt.Sprintf(`{
  "pid": 99999,
  "httpsPort": %d,
  "httpPort": 50005,
  "lspPort": 41067,
  "lsVersion": "1.11.0",
  "csrfToken": ""
}`, port)

	jsonPath := filepath.Join(tmpDir, "ls_e3b0c44298fc1c14.json")
	if err := os.WriteFile(jsonPath, []byte(jsonContent), 0600); err != nil {
		t.Fatalf("failed to write mock discovery json: %v", err)
	}

	insp := NewInspector(10 * time.Second)
	info := insp.Scan()
	if info == nil {
		t.Fatalf("expected discovery info, got nil")
	}
	if info.PID != 99999 {
		t.Errorf("expected PID 99999, got %d", info.PID)
	}
	if info.Port != port {
		t.Errorf("expected port %d, got %d", port, info.Port)
	}
	if !info.IsHealthy {
		t.Errorf("expected healthy instance")
	}
}

func TestClassifyLanguageServerCmdline(t *testing.T) {
	t.Setenv("MGY_EXCLUDE_HEADLESS", "")
	t.Setenv("MULTIGRAVITY_EXCLUDE_HEADLESS", "")
	t.Setenv("MGY_EXCLUDE_PROCESS_PATTERNS", "")
	t.Setenv("MULTIGRAVITY_EXCLUDE_PROCESS_PATTERNS", "")

	desktopCmd := `/Applications/Antigravity.app/Contents/Resources/bin/language_server --standalone --override_ide_name antigravity --subclient_type hub --override_ide_version 2.31.0 --https_server_port 0 --csrf_token e98a623c-afc0-4805-a10f-dc563a14b882 --host_bridge_url=http://127.0.0.1:58221`
	ok, prio := ClassifyLanguageServerCmdline(desktopCmd)
	if !ok {
		t.Fatalf("expected desktop language_server command line to be accepted")
	}
	if prio < 7 {
		t.Errorf("expected high priority for desktop GUI instance, got %d", prio)
	}

	// Headless language_server is accepted by default at lower priority than Desktop GUI,
	// and can be excluded via MGY_EXCLUDE_HEADLESS or MGY_EXCLUDE_PROCESS_PATTERNS.
	headlessCmd := `/usr/local/bin/language_server --standalone --headless --http_server_port=50005`
	okHeadless, prioHeadless := ClassifyLanguageServerCmdline(headlessCmd)
	if !okHeadless {
		t.Errorf("expected standalone --headless language_server to be accepted by default")
	}
	if prioHeadless >= prio {
		t.Errorf("expected headless priority (%d) < desktop priority (%d)", prioHeadless, prio)
	}

	// Antigravity CLI (agy / antigravity-cli) with --csrf_token or remote-control serve.
	agyRemoteCmd := `4210 /usr/local/bin/agy --csrf_token=5aacbcc0-cf40-4b94-8f17-986485a8d0d5 remote-control serve`
	okAgy, prioAgy := ClassifyLanguageServerCmdline(agyRemoteCmd)
	if !okAgy {
		t.Fatalf("expected agy remote-control serve command line to be accepted")
	}
	if prioAgy >= prio {
		t.Errorf("expected agy CLI priority (%d) < desktop priority (%d)", prioAgy, prio)
	}

	agyServeOnly := `/home/user/.local/bin/agy remote-control serve`
	if ok, _ := ClassifyLanguageServerCmdline(agyServeOnly); !ok {
		t.Errorf("expected agy remote-control serve without explicit csrf flag to be accepted")
	}

	antigravityCLICmd := `"/opt/my dir/antigravity-cli" --csrf_token 5aacbcc0-cf40-4b94-8f17-986485a8d0d5`
	if ok, _ := ClassifyLanguageServerCmdline(antigravityCLICmd); !ok {
		t.Errorf("expected quoted antigravity-cli with --csrf_token to be accepted")
	}

	winAgyCmd := `"C:\Program Files\Antigravity\agy.exe" --csrf_token=5aacbcc0-cf40-4b94-8f17-986485a8d0d5 remote-control serve`
	if ok, _ := ClassifyLanguageServerCmdline(winAgyCmd); !ok {
		t.Errorf("expected Windows agy.exe with --csrf_token to be accepted")
	}

	// Bare interactive agy or wrapper scripts must be rejected.
	if ok, _ := ClassifyLanguageServerCmdline(`/usr/local/bin/agy --version`); ok {
		t.Errorf("expected bare agy command without --csrf_token or remote-control serve to be rejected")
	}
	if ok, _ := ClassifyLanguageServerCmdline(`/usr/bin/python3 helper.py agy --csrf_token=5aacbcc0-cf40-4b94-8f17-986485a8d0d5`); ok {
		t.Errorf("expected python wrapper containing agy to be rejected")
	}

	multicallCmd := `/Applications/Antigravity.app/Contents/Resources/bin/language_server multicall schedule 0 23 * * * agentapi new-conversation`
	if ok, _ := ClassifyLanguageServerCmdline(multicallCmd); ok {
		t.Errorf("expected multicall schedule helper to be rejected")
	}

	// Opt-in exclusion via MGY_EXCLUDE_HEADLESS.
	t.Run("exclude_headless_env", func(t *testing.T) {
		t.Setenv("MGY_EXCLUDE_HEADLESS", "1")
		if ok, _ := ClassifyLanguageServerCmdline(headlessCmd); ok {
			t.Errorf("expected --headless language_server to be rejected when MGY_EXCLUDE_HEADLESS=1")
		}
		if ok, _ := ClassifyLanguageServerCmdline(desktopCmd); !ok {
			t.Errorf("expected desktop language_server to remain accepted when MGY_EXCLUDE_HEADLESS=1")
		}
	})

	// Opt-in exclusion via MGY_EXCLUDE_PROCESS_PATTERNS (case-insensitive, comma-separated).
	t.Run("exclude_patterns_env", func(t *testing.T) {
		t.Setenv("MGY_EXCLUDE_PROCESS_PATTERNS", " custom-bot , isolated-worker ")
		botCmd := `/srv/custom-bot/data/antigravity-ls/bin/language_server --standalone --persistent_mode --http_server_port=50232`
		if ok, _ := ClassifyLanguageServerCmdline(botCmd); ok {
			t.Errorf("expected process matching MGY_EXCLUDE_PROCESS_PATTERNS to be rejected")
		}
		if ok, _ := ClassifyLanguageServerCmdline(agyRemoteCmd); !ok {
			t.Errorf("expected unrelated agy process to remain accepted")
		}
	})
}

func TestDaemonDiscoveryAntigravityCLIFallbackAndDedup(t *testing.T) {
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/exa.language_server_pb.LanguageServerService/GetStatus" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("{}"))
			return
		}
		http.NotFound(w, r)
	})

	cliSrv := httptest.NewTLSServer(okHandler)
	defer cliSrv.Close()
	cliURL, _ := url.Parse(cliSrv.URL)
	cliPort, _ := strconv.Atoi(cliURL.Port())

	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("ANTIGRAVITY_APP_DATA_DIR", "")

	cliDaemonDir := filepath.Join(tmpHome, ".gemini", "antigravity-cli", "daemon")
	if err := os.MkdirAll(cliDaemonDir, 0700); err != nil {
		t.Fatalf("mkdir cli daemon dir: %v", err)
	}
	cliJSON := fmt.Sprintf(`{"pid": 51001, "httpsPort": %d, "lsVersion": "2.40.0", "csrfToken": "cli-token"}`, cliPort)
	if err := os.WriteFile(filepath.Join(cliDaemonDir, "ls_cli.json"), []byte(cliJSON), 0600); err != nil {
		t.Fatalf("write cli discovery json: %v", err)
	}

	insp := NewInspector(10 * time.Second)
	info := insp.Scan()
	if info == nil {
		t.Fatalf("expected discovery from ~/.gemini/antigravity-cli/daemon, got nil")
	}
	if info.PID != 51001 || info.Port != cliPort || info.CSRFToken != "cli-token" {
		t.Fatalf("unexpected CLI discovery info: %+v", info)
	}
	wantAppDir := filepath.Join(tmpHome, ".gemini", "antigravity-cli")
	if info.AppDataDir != wantAppDir {
		t.Errorf("AppDataDir = %q, want %q", info.AppDataDir, wantAppDir)
	}

	// Symlink ~/.gemini/antigravity -> ~/.gemini/antigravity-cli must deduplicate dirs cleanly.
	desktopDir := filepath.Join(tmpHome, ".gemini", "antigravity")
	if err := os.Symlink(wantAppDir, desktopDir); err == nil {
		dirs := DaemonDiscoveryDirs()
		if len(dirs) != 1 {
			t.Errorf("expected symlinked daemon dirs to deduplicate to 1 entry, got %v", dirs)
		}
		_ = os.Remove(desktopDir)
	}

	// When both Desktop and CLI daemon files are live, Desktop must take precedence.
	desktopSrv := httptest.NewTLSServer(okHandler)
	defer desktopSrv.Close()
	desktopURL, _ := url.Parse(desktopSrv.URL)
	desktopPort, _ := strconv.Atoi(desktopURL.Port())

	desktopDaemonDir := filepath.Join(tmpHome, ".gemini", "antigravity", "daemon")
	if err := os.MkdirAll(desktopDaemonDir, 0700); err != nil {
		t.Fatalf("mkdir desktop daemon dir: %v", err)
	}
	desktopJSON := fmt.Sprintf(`{"pid": 52002, "httpsPort": %d, "lsVersion": "2.40.1", "csrfToken": "desktop-token"}`, desktopPort)
	if err := os.WriteFile(filepath.Join(desktopDaemonDir, "hub_desktop.json"), []byte(desktopJSON), 0600); err != nil {
		t.Fatalf("write desktop discovery json: %v", err)
	}

	info2 := insp.Scan()
	if info2 == nil || info2.PID != 52002 || info2.Port != desktopPort {
		t.Fatalf("expected Desktop daemon (PID 52002, port %d) to take precedence over CLI, got %+v", desktopPort, info2)
	}
	if info2.AppDataDir != filepath.Join(tmpHome, ".gemini", "antigravity") {
		t.Errorf("AppDataDir = %q, want desktop dir", info2.AppDataDir)
	}
}

func TestDaemonPreemptsFallbackInstance(t *testing.T) {
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/exa.language_server_pb.LanguageServerService/GetStatus" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("{}"))
			return
		}
		http.NotFound(w, r)
	})

	fallbackSrv := httptest.NewTLSServer(okHandler)
	defer fallbackSrv.Close()
	fallbackURL, _ := url.Parse(fallbackSrv.URL)
	fallbackPort, _ := strconv.Atoi(fallbackURL.Port())

	desktopSrv := httptest.NewTLSServer(okHandler)
	defer desktopSrv.Close()
	desktopURL, _ := url.Parse(desktopSrv.URL)
	desktopPort, _ := strconv.Atoi(desktopURL.Port())

	tmpDir := t.TempDir()
	daemonDirOverride = tmpDir
	t.Cleanup(func() {
		daemonDirOverride = ""
	})

	insp := NewInspector(10 * time.Second)
	// Simulate inspector having previously latched onto a still-healthy fallback instance (e.g. PID 76967)
	insp.current = &InstanceInfo{
		PID:          76967,
		Port:         fallbackPort,
		CSRFToken:    "",
		DiscoveredAt: time.Now().Add(-time.Minute),
		IsHealthy:    true,
	}

	// Now the desktop client finishes starting up and writes hub_*.json
	hubJSON := fmt.Sprintf(`{
  "pid": 47822,
  "httpsPort": %d,
  "httpPort": 58223,
  "lsVersion": "2.31.0",
  "csrfToken": "e98a623c-afc0-4805-a10f-dc563a14b882"
}`, desktopPort)
	if err := os.WriteFile(filepath.Join(tmpDir, "hub_58223.json"), []byte(hubJSON), 0600); err != nil {
		t.Fatalf("failed to write hub discovery json: %v", err)
	}

	info := insp.Scan()
	if info == nil {
		t.Fatalf("expected Scan to return instance info")
	}
	if info.PID != 47822 || info.Port != desktopPort {
		t.Fatalf("expected daemon hub instance (PID 47822, port %d) to preempt fallback (PID 76967, port %d), got PID=%d port=%d",
			desktopPort, fallbackPort, info.PID, info.Port)
	}
}

func TestInstanceInfoOmitsCSRFTokenFromJSON(t *testing.T) {
	inst := InstanceInfo{
		PID:       1234,
		Port:      50000,
		CSRFToken: "super-secret-csrf-token",
		IsHealthy: true,
	}
	raw, err := json.Marshal(inst)
	if err != nil {
		t.Fatalf("marshal InstanceInfo: %v", err)
	}
	if strings.Contains(string(raw), "super-secret-csrf-token") || strings.Contains(string(raw), "csrf_token") {
		t.Fatalf("CSRFToken must not be serialized in JSON: %s", string(raw))
	}
}

func TestExtractCmdExecutableBase_ProxyWrappers(t *testing.T) {
	cases := []struct {
		name     string
		cmdline  string
		wantBase string
		wantOK   bool
	}{
		{
			name:     "proxychains4_desktop_ls",
			cmdline:  `4210 /opt/homebrew/bin/proxychains4 -q -f /tmp/pc.conf /Applications/Antigravity.app/Contents/Resources/bin/language_server --csrf_token=5aacbcc0-cf40-4b94-8f17-986485a8d0d5`,
			wantBase: "language_server",
			wantOK:   true,
		},
		{
			name:     "graftcp_space_path_ls",
			cmdline:  `4211 graftcp -a 127.0.0.1 -p 2233 /Applications/Antigravity IDE.app/Contents/Resources/bin/language_server --csrf_token 5aacbcc0-cf40-4b94-8f17-986485a8d0d5`,
			wantBase: "language_server",
			wantOK:   true,
		},
		{
			name:     "env_proxy_vars_agy",
			cmdline:  `4212 /usr/bin/env HTTP_PROXY=http://127.0.0.1:7890 NO_PROXY=127.0.0.1 /usr/local/bin/agy --csrf_token=5aacbcc0-cf40-4b94-8f17-986485a8d0d5 remote-control serve`,
			wantBase: "agy",
			wantOK:   true,
		},
		{
			name:     "python_wrapper_still_rejected",
			cmdline:  `4213 /usr/bin/python3 helper.py /Applications/Antigravity.app/Contents/Resources/bin/language_server --csrf_token=5aacbcc0-cf40-4b94-8f17-986485a8d0d5`,
			wantBase: "python3",
			wantOK:   false,
		},
		{
			name:     "shell_wrapper_still_rejected",
			cmdline:  `4214 /bin/sh -c "/Applications/Antigravity.app/Contents/Resources/bin/language_server --csrf_token=5aacbcc0-cf40-4b94-8f17-986485a8d0d5"`,
			wantBase: "sh",
			wantOK:   false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotBase := ExtractCmdExecutableBase(tc.cmdline)
			if gotBase != tc.wantBase {
				t.Errorf("ExtractCmdExecutableBase() = %q, want %q", gotBase, tc.wantBase)
			}
			if gotOK := IsAntigravityProcessBase(gotBase); gotOK != tc.wantOK {
				t.Errorf("IsAntigravityProcessBase(%q) = %v, want %v", gotBase, gotOK, tc.wantOK)
			}
		})
	}
}

func TestScanMultiPIDFallbackAndForceScanBypassBackoff(t *testing.T) {
	tmpDir := t.TempDir()
	daemonDirOverride = tmpDir
	t.Cleanup(func() { daemonDirOverride = "" })

	insp := NewInspector(10 * time.Second)
	insp.forceScanCooldown = 20 * time.Millisecond

	// 1. First Scan fails while Antigravity is closed -> enters backoff + records antigravity_not_running.
	insp.findCandidatesFn = func(context.Context) processScanResult {
		return processScanResult{Err: fmt.Errorf("not found")}
	}
	if got := insp.Scan(); got != nil {
		t.Fatalf("expected nil on initial empty scan, got %+v", got)
	}
	if reason := insp.LastFailureReason(); reason != ReasonAntigravityNotRunning {
		t.Fatalf("LastFailureReason = %q, want %q", reason, ReasonAntigravityNotRunning)
	}

	// Simulate maxed-out 40s backoff (as if mgy waited while Antigravity was closed).
	insp.failedScanMu.Lock()
	insp.failedBackoff = 40 * time.Second
	insp.failedScanAt = time.Now()
	insp.failedScanMu.Unlock()

	// 2. Now user launches Antigravity with TWO candidate language_server PIDs:
	//    PID 9002 (higher priority/PID) has no healthy port; PID 9001 has a healthy port 55123.
	insp.findCandidatesFn = func(context.Context) processScanResult {
		return processScanResult{
			AppRunning: true,
			Candidates: []processCandidate{
				{PID: 9002, CSRFToken: "bad-tok", Priority: 11},
				{PID: 9001, CSRFToken: "good-tok", Priority: 9},
			},
		}
	}
	insp.findPortsFn = func(_ context.Context, pid int) ([]int, error) {
		if pid == 9002 {
			return []int{55120}, nil
		}
		return []int{55122, 55123}, nil
	}
	insp.verifyPortFn = func(port int, csrf string) bool {
		return port == 55123 && csrf == "good-tok"
	}

	// Periodic Scan() is still held back by the 40s backoff...
	if got := insp.Scan(); got != nil {
		t.Fatalf("expected periodic Scan() to respect 40s backoff, got %+v", got)
	}
	// ...whereas ForceScan() immediately bypasses the 40s backoff and falls back from PID 9002 to PID 9001!
	got := insp.ForceScan()
	if got == nil || got.PID != 9001 || got.Port != 55123 || got.CSRFToken != "good-tok" {
		t.Fatalf("ForceScan() = %+v, want PID=9001 Port=55123", got)
	}
	if reason := insp.LastFailureReason(); reason != ReasonNone {
		t.Fatalf("expected LastFailureReason to clear on connect, got %q", reason)
	}

	// 3. When Antigravity closes, unhealthy transition notifies listeners and updates reason.
	var updates []InstanceInfo
	insp.OnUpdate(func(info InstanceInfo) {
		updates = append(updates, info)
	})
	insp.verifyPortFn = func(int, string) bool { return false }
	insp.findCandidatesFn = func(context.Context) processScanResult {
		return processScanResult{AccessDenied: true}
	}
	time.Sleep(25 * time.Millisecond)
	if got := insp.ForceScan(); got != nil {
		t.Fatalf("expected nil after disconnect, got %+v", got)
	}
	if reason := insp.LastFailureReason(); reason != ReasonLSAccessDenied {
		t.Fatalf("LastFailureReason = %q, want %q", reason, ReasonLSAccessDenied)
	}
	if len(updates) != 1 || updates[0].IsHealthy {
		t.Fatalf("expected single unhealthy listener notification, got %+v", updates)
	}
}

func TestResetBackoffClearsBackoffAndForceScanCooldown(t *testing.T) {
	tmpDir := t.TempDir()
	daemonDirOverride = tmpDir
	t.Cleanup(func() { daemonDirOverride = "" })

	insp := NewInspector(10 * time.Second)
	insp.forceScanCooldown = 10 * time.Second

	// 1. Trigger a failed ForceScan during daemon restart window: sets both failedBackoff and lastForceScanAt.
	insp.findCandidatesFn = func(context.Context) processScanResult {
		return processScanResult{Err: fmt.Errorf("restarting")}
	}
	if got := insp.ForceScan(); got != nil {
		t.Fatalf("expected nil during restart window, got %+v", got)
	}

	// 2. Daemon finishes restarting on a new port; before ResetBackoff both Scan and ForceScan are blocked.
	insp.findCandidatesFn = func(context.Context) processScanResult {
		return processScanResult{
			Candidates: []processCandidate{{PID: 7777, CSRFToken: "new-csrf", Priority: 5}},
		}
	}
	insp.findPortsFn = func(_ context.Context, pid int) ([]int, error) {
		if pid == 7777 {
			return []int{44556}, nil
		}
		return nil, nil
	}
	insp.verifyPortFn = func(port int, csrf string) bool {
		return port == 44556 && csrf == "new-csrf"
	}

	if got := insp.Scan(); got != nil {
		t.Fatalf("expected Scan() to be held back by backoff before ResetBackoff, got %+v", got)
	}
	if got := insp.ForceScan(); got != nil {
		t.Fatalf("expected ForceScan() to be held back by cooldown before ResetBackoff, got %+v", got)
	}

	// 3. ResetBackoff clears both backoff and ForceScan cooldown so immediate ForceScan succeeds.
	insp.ResetBackoff()
	got := insp.ForceScan()
	if got == nil || got.PID != 7777 || got.Port != 44556 || got.CSRFToken != "new-csrf" {
		t.Fatalf("ForceScan() after ResetBackoff = %+v, want PID=7777 Port=44556", got)
	}

	// 4. Verify ResetBackoff also unblocks plain Scan() when backed off for 40s.
	insp.mu.Lock()
	insp.current = nil
	insp.mu.Unlock()
	insp.failedScanMu.Lock()
	insp.failedBackoff = 40 * time.Second
	insp.failedScanAt = time.Now()
	insp.failedScanMu.Unlock()

	if got := insp.Scan(); got != nil {
		t.Fatalf("expected Scan() to respect 40s backoff, got %+v", got)
	}
	insp.ResetBackoff()
	got2 := insp.Scan()
	if got2 == nil || got2.PID != 7777 || got2.Port != 44556 {
		t.Fatalf("Scan() after ResetBackoff = %+v, want PID=7777 Port=44556", got2)
	}
}



