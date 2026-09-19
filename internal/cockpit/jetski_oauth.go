package cockpit

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"antigravity-mobile/internal/localtls"
)

var (
	ya29RE     = regexp.MustCompile(`ya29\.[A-Za-z0-9._-]+`)
	refreshRE  = regexp.MustCompile(`1//[A-Za-z0-9_-]+`)
	jwtRE      = regexp.MustCompile(`eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+`)
	lsCSRFRE   = regexp.MustCompile(`--csrf_token\s+([0-9a-fA-F-]+)`)
	lsListenRE = regexp.MustCompile(`127\.0\.0\.1:(\d+)`)
)

type jetskiOAuthFile struct {
	Token struct {
		AccessToken  string `json:"access_token"`
		TokenType    string `json:"token_type"`
		RefreshToken string `json:"refresh_token"`
		Expiry       string `json:"expiry"`
	} `json:"token"`
	AuthMethod string `json:"auth_method"`
	IDToken    string `json:"id_token,omitempty"`
}

type parsedOAuth struct {
	AccessToken  string
	RefreshToken string
	IDToken      string
	Email        string
	Expiry       time.Time
}

// applyLanguageServerOAuth is the post-Cockpit hook. Tests replace it.
var applyLanguageServerOAuth = applyJetskiOAuthAndRelaunch

func jetskiOAuthPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".gemini", "jetski-standalone-oauth-token")
}

func googleAccountsPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".gemini", "google_accounts.json")
}

func applyJetskiOAuthAndRelaunch(accountID string) error {
	tok, err := fetchAccountOAuth(accountID)
	if err != nil {
		log.Printf("[Cockpit] cockpit token fetch failed (%v); falling back to vscdb", err)
		tok, err = loadOAuthFromVscdb()
	}
	if err != nil {
		return fmt.Errorf("no oauth for %s: %w", accountID, err)
	}
	if err := writeJetskiOAuthFile(tok); err != nil {
		log.Printf("[Cockpit] jetski oauth write failed: %v", err)
	}
	if tok.Email != "" {
		syncGeminiGoogleAccounts(tok.Email)
	}
	if err := writeGeminiKeychainOAuth(tok); err != nil {
		return fmt.Errorf("write gemini keychain oauth: %w", err)
	}

	want := resolveAccountEmail(accountID)
	if want == "" {
		want = tok.Email
	}
	log.Printf("[Cockpit] wrote language-server oauth for %s; relaunching Antigravity.app with proxy", want)
	if err := quitRunningAntigravity(); err != nil {
		log.Printf("[Cockpit] quit before proxy relaunch: %v", err)
	}
	time.Sleep(800 * time.Millisecond)
	if err := launchAntigravityWithCockpitProxy(); err != nil {
		return err
	}
	live, err := waitLiveUserEmail(30 * time.Second)
	if err != nil {
		log.Printf("[Cockpit] GetUserStatus after keychain relaunch: %v", err)
		return nil
	}
	if want != "" && !strings.EqualFold(strings.TrimSpace(live), want) {
		return fmt.Errorf("Antigravity still signed in as %s, expected %s", live, want)
	}
	log.Printf("[Cockpit] live Language Server is %s", live)
	return nil
}

func writeGeminiKeychainOAuth(tok *parsedOAuth) error {
	env := map[string]any{
		"token": map[string]any{
			"access_token":  tok.AccessToken,
			"token_type":    "Bearer",
			"refresh_token": tok.RefreshToken,
			"expiry":        tok.Expiry.UTC().Format("2006-01-02T15:04:05.000000Z"),
		},
		"auth_method": "consumer",
	}
	raw, err := json.Marshal(env)
	if err != nil {
		return err
	}
	secret := "go-keyring-base64:" + base64.StdEncoding.EncodeToString(raw)

	if runtime.GOOS == "windows" {
		cmd := exec.Command("cmdkey", "/generic:LegacyGeneric:target=gemini:antigravity", "/user:antigravity", "/pass:"+secret)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("cmdkey write: %w (%s)", err, strings.TrimSpace(string(out)))
		}
		log.Printf("[Cockpit] updated Windows Credential Manager gemini/antigravity for %s", tok.Email)
		return nil
	}

	cmd := exec.Command("security", "add-generic-password", "-U", "-s", "gemini", "-a", "antigravity", "-w", secret)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	log.Printf("[Cockpit] updated keychain gemini/antigravity for %s", tok.Email)
	return nil
}

func isSafeProxyURL(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.ContainsAny(raw, "\r\n\x00") {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return false
	}
	scheme := strings.ToLower(u.Scheme)
	return scheme == "http" || scheme == "https" || scheme == "socks5" || scheme == "socks5h"
}

func launchAntigravityWithCockpitProxy() error {
	var proxy, noProxy string
	hasProxy := false
	if cfg, err := getCockpitConfig(); err == nil && cfg.GlobalProxyEnabled && strings.TrimSpace(cfg.GlobalProxyURL) != "" {
		p := strings.TrimSpace(cfg.GlobalProxyURL)
		if !isSafeProxyURL(p) {
			log.Printf("[Cockpit] ⚠️ rejected unsafe GlobalProxyURL %q; launching without proxy", p)
		} else {
			proxy = p
			noProxy = strings.TrimSpace(cfg.GlobalProxyNoProxy)
			if noProxy == "" || strings.ContainsAny(noProxy, "\r\n\x00") {
				noProxy = "127.0.0.1,localhost,::1"
			}
			hasProxy = true
		}
	}

	if runtime.GOOS == "windows" {
		var candidatePaths []string
		if localApp := os.Getenv("LOCALAPPDATA"); localApp != "" {
			candidatePaths = append(candidatePaths, filepath.Join(localApp, "Programs", "antigravity", "Antigravity.exe"))
		}
		if progFiles := os.Getenv("ProgramFiles"); progFiles != "" {
			candidatePaths = append(candidatePaths, filepath.Join(progFiles, "Antigravity", "Antigravity.exe"))
		}
		if progFilesX86 := os.Getenv("ProgramFiles(x86)"); progFilesX86 != "" {
			candidatePaths = append(candidatePaths, filepath.Join(progFilesX86, "Antigravity", "Antigravity.exe"))
		}
		if home, err := os.UserHomeDir(); err == nil {
			candidatePaths = append(candidatePaths, filepath.Join(home, "AppData", "Local", "Programs", "antigravity", "Antigravity.exe"))
		}

		var exePath string
		for _, p := range candidatePaths {
			if _, err := os.Stat(p); err == nil {
				exePath = p
				break
			}
		}
		if exePath == "" {
			if looked, err := exec.LookPath("Antigravity.exe"); err == nil {
				exePath = looked
			}
		}
		if exePath == "" {
			return fmt.Errorf("Antigravity.exe executable not found on Windows")
		}

		cmd := exec.Command(exePath)
		if hasProxy {
			cmd.Env = os.Environ()
			for _, key := range []string{"http_proxy", "https_proxy", "HTTP_PROXY", "HTTPS_PROXY", "all_proxy", "ALL_PROXY"} {
				cmd.Env = append(cmd.Env, key+"="+proxy)
			}
			cmd.Env = append(cmd.Env, "no_proxy="+noProxy, "NO_PROXY="+noProxy)
			log.Printf("[Cockpit] launching Antigravity.exe with proxy %s", proxy)
		} else {
			log.Printf("[Cockpit] launching Antigravity.exe without global proxy")
		}
		return cmd.Start()
	}

	args := []string{"-a", "Antigravity"}
	if hasProxy {
		for _, key := range []string{"http_proxy", "https_proxy", "HTTP_PROXY", "HTTPS_PROXY", "all_proxy", "ALL_PROXY"} {
			args = append(args, "--env", key+"="+proxy)
		}
		args = append(args, "--env", "no_proxy="+noProxy, "--env", "NO_PROXY="+noProxy)
		log.Printf("[Cockpit] launching Antigravity.app with proxy %s", proxy)
	} else {
		log.Printf("[Cockpit] launching Antigravity.app without global proxy")
	}
	cmd := exec.Command("open", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("open Antigravity.app: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func resolveAccountEmail(accountID string) string {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return ""
	}
	if strings.Contains(accountID, "@") {
		return strings.ToLower(accountID)
	}
	dataDir, err := GetCockpitDataDir()
	if err != nil {
		return ""
	}
	raw, err := os.ReadFile(filepath.Join(dataDir, "accounts.json"))
	if err != nil {
		return ""
	}
	var idx accountsIndex
	if json.Unmarshal(raw, &idx) != nil {
		return ""
	}
	for _, acc := range idx.Accounts {
		if acc.ID == accountID {
			return strings.ToLower(strings.TrimSpace(acc.Email))
		}
	}
	return ""
}

func loadOAuthFromVscdb() (*parsedOAuth, error) {
	var lastErr error
	for _, dbPath := range antigravityStateDBPaths() {
		if _, err := os.Stat(dbPath); err != nil {
			continue
		}
		var out []byte
		var err error
		if _, lookErr := exec.LookPath("sqlite3"); lookErr == nil {
			cmd := exec.Command("sqlite3", "-batch", "-noheader", dbPath, "SELECT value FROM ItemTable WHERE key = "+sqliteQuote("antigravityUnifiedStateSync.oauthToken")+";")
			out, err = cmd.CombinedOutput()
		}
		if len(out) == 0 {
			pyScript := "import sqlite3, sys; conn = sqlite3.connect(sys.argv[1]); cur = conn.cursor(); cur.execute('SELECT value FROM ItemTable WHERE key = ?', (sys.argv[2],)); row = cur.fetchone(); sys.stdout.write(row[0] if row and row[0] else '')"
			for _, pyExe := range []string{"python", "python3"} {
				if _, lookErr := exec.LookPath(pyExe); lookErr == nil {
					cmd := exec.Command(pyExe, "-c", pyScript, dbPath, "antigravityUnifiedStateSync.oauthToken")
					if pyOut, pyErr := cmd.Output(); pyErr == nil && len(pyOut) > 0 {
						out = pyOut
						err = nil
						break
					}
				}
			}
		}
		if err != nil || len(out) == 0 {
			if err != nil {
				lastErr = fmt.Errorf("%s: %w (%s)", dbPath, err, strings.TrimSpace(string(out)))
			}
			continue
		}
		tok, err := parseJetskiOAuth(string(out))
		if err != nil {
			lastErr = err
			continue
		}
		return tok, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("oauth token not found in state.vscdb")
	}
	return nil, lastErr
}

func parseJetskiOAuth(raw string) (*parsedOAuth, error) {
	blob := strings.Join(collectOAuthHaystacks(raw), "\n")
	access := ya29RE.FindString(blob)
	refresh := refreshRE.FindString(blob)
	idToken := jwtRE.FindString(blob)
	if access == "" || idToken == "" {
		return nil, fmt.Errorf("vscdb oauth missing access_token or id_token")
	}
	email, exp := jwtEmailAndExp(idToken)
	tok := &parsedOAuth{
		AccessToken:  access,
		RefreshToken: refresh,
		IDToken:      idToken,
		Email:        email,
		Expiry:       exp,
	}
	if tok.Expiry.IsZero() {
		tok.Expiry = time.Now().Add(50 * time.Minute)
	}
	return tok, nil
}

var innerB64RE = regexp.MustCompile(`[A-Za-z0-9+/]{48,}={0,2}`)

func compactBase64(s string) string {
	return strings.Map(func(r rune) rune {
		if r <= ' ' {
			return -1
		}
		return r
	}, s)
}

func collectOAuthHaystacks(raw string) []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(s string) {
		if s == "" {
			return
		}
		if _, ok := seen[s]; ok {
			return
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	queue := []string{raw, compactBase64(raw)}
	for i := 0; i < len(queue) && i < 12; i++ {
		cur := queue[i]
		add(cur)
		if decoded := tryBase64(compactBase64(cur)); decoded != "" {
			if _, ok := seen[decoded]; !ok {
				queue = append(queue, decoded)
			}
			add(decoded)
			for _, m := range innerB64RE.FindAllString(decoded, 12) {
				if inner := tryBase64(m); inner != "" {
					if _, ok := seen[inner]; !ok {
						queue = append(queue, inner)
					}
					add(inner)
				}
			}
		}
	}
	return out
}

func tryBase64(s string) string {
	s = compactBase64(s)
	if len(s) < 16 {
		return ""
	}
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if b, err := enc.DecodeString(s); err == nil && len(b) > 16 {
			return string(b)
		}
	}
	return ""
}

func jwtEmailAndExp(idToken string) (string, time.Time) {
	parts := strings.Split(idToken, ".")
	if len(parts) < 2 {
		return "", time.Time{}
	}
	payload := parts[1]
	if pad := len(payload) % 4; pad != 0 {
		payload += strings.Repeat("=", 4-pad)
	}
	raw, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		raw, err = base64.RawURLEncoding.DecodeString(parts[1])
		if err != nil {
			return "", time.Time{}
		}
	}
	var claims struct {
		Email string `json:"email"`
		Exp   int64  `json:"exp"`
	}
	if json.Unmarshal(raw, &claims) != nil {
		return "", time.Time{}
	}
	var exp time.Time
	if claims.Exp > 0 {
		exp = time.Unix(claims.Exp, 0)
	}
	return strings.ToLower(strings.TrimSpace(claims.Email)), exp
}

func writeJetskiOAuthFile(tok *parsedOAuth) error {
	path := jetskiOAuthPath()
	if path == "" {
		return fmt.Errorf("cannot resolve home for jetski oauth file")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	doc := jetskiOAuthFile{AuthMethod: "consumer", IDToken: tok.IDToken}
	doc.Token.AccessToken = tok.AccessToken
	doc.Token.TokenType = "Bearer"
	doc.Token.RefreshToken = tok.RefreshToken
	doc.Token.Expiry = tok.Expiry.In(time.Local).Format(time.RFC3339Nano)
	raw, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	log.Printf("[Cockpit] wrote jetski-standalone-oauth-token for %s", tok.Email)
	return nil
}

func syncGeminiGoogleAccounts(email string) {
	path := googleAccountsPath()
	if path == "" || email == "" {
		return
	}
	doc := map[string]any{"active": email, "old": []any{}}
	if raw, err := os.ReadFile(path); err == nil {
		var existing map[string]any
		if json.Unmarshal(raw, &existing) == nil {
			if old, ok := existing["old"]; ok {
				doc["old"] = old
			}
		}
	}
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(path, append(raw, '\n'), 0600)
}

func waitLiveUserEmail(d time.Duration) (string, error) {
	deadline := time.Now().Add(d)
	var lastErr error
	for time.Now().Before(deadline) {
		email, err := liveUserEmail()
		if err == nil && strings.TrimSpace(email) != "" {
			return email, nil
		}
		lastErr = err
		time.Sleep(400 * time.Millisecond)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("timed out waiting for GetUserStatus")
	}
	return "", lastErr
}

func liveUserEmail() (string, error) {
	port, csrf, err := lookupLanguageServer()
	if err != nil {
		return "", err
	}
	url := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/GetUserStatus", port)
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader("{}"))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connect-Protocol-Version", "1")
	if csrf != "" {
		req.Header.Set("x-codeium-csrf-token", csrf)
	}
	client := &http.Client{
		Timeout: 2 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: localtls.ClientConfig(),
			DialTLSContext:  localtls.DialTLSContext,
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GetUserStatus status %d", resp.StatusCode)
	}
	var data struct {
		UserStatus struct {
			Email string `json:"email"`
		} `json:"userStatus"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return "", err
	}
	return strings.TrimSpace(data.UserStatus.Email), nil
}

func lookupLanguageServer() (int, string, error) {
	if runtime.GOOS == "windows" {
		psScript := `Get-CimInstance Win32_Process | Where-Object { $_.Name -like '*language_server*' } | Select-Object ProcessId,CommandLine | ConvertTo-Json -Compress`
		cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", psScript)
		out, err := cmd.Output()
		if err != nil {
			return 0, "", err
		}
		trimmed := strings.TrimSpace(string(out))
		if trimmed == "" || trimmed == "null" {
			return 0, "", fmt.Errorf("language_server not found")
		}
		type winProc struct {
			ProcessID   int    `json:"ProcessId"`
			CommandLine string `json:"CommandLine"`
		}
		var procs []winProc
		if strings.HasPrefix(trimmed, "[") {
			_ = json.Unmarshal([]byte(trimmed), &procs)
		} else if strings.HasPrefix(trimmed, "{") {
			var s winProc
			if json.Unmarshal([]byte(trimmed), &s) == nil {
				procs = append(procs, s)
			}
		}
		var pid int
		var csrf string
		for _, p := range procs {
			if strings.Contains(p.CommandLine, "language_server") && strings.Contains(p.CommandLine, "--csrf_token") {
				if m := lsCSRFRE.FindStringSubmatch(p.CommandLine); len(m) > 1 {
					pid = p.ProcessID
					csrf = m[1]
					break
				}
			}
		}
		if pid == 0 || csrf == "" {
			return 0, "", fmt.Errorf("language_server not found or missing csrf")
		}
		netstatOut, err := exec.Command("netstat", "-ano", "-p", "tcp").Output()
		if err != nil {
			return 0, "", err
		}
		pidStr := strconv.Itoa(pid)
		for _, rawLine := range bytes.Split(netstatOut, []byte("\n")) {
			line := strings.TrimSpace(string(rawLine))
			if !strings.HasPrefix(line, "TCP") {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) >= 5 && strings.EqualFold(fields[3], "LISTENING") && fields[4] == pidStr {
				localAddr := fields[1]
				if idx := strings.LastIndex(localAddr, ":"); idx != -1 {
					if p, convErr := strconv.Atoi(localAddr[idx+1:]); convErr == nil && p > 0 {
						return p, csrf, nil
					}
				}
			}
		}
		return 0, "", fmt.Errorf("language_server has no listen port")
	}

	out, err := exec.Command("ps", "-axo", "pid=,command=").Output()
	if err != nil {
		return 0, "", err
	}
	var pid, csrf string
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.Contains(line, "language_server") || !strings.Contains(line, "--csrf_token") {
			continue
		}
		if strings.Contains(line, "multicall") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		pid = fields[0]
		if m := lsCSRFRE.FindStringSubmatch(line); len(m) > 1 {
			csrf = m[1]
		}
		break
	}
	if pid == "" || csrf == "" {
		return 0, "", fmt.Errorf("language_server not found")
	}
	lsof, err := exec.Command("lsof", "-nP", "-p", pid, "-a", "-iTCP", "-sTCP:LISTEN").Output()
	if err != nil {
		return 0, "", fmt.Errorf("lsof language_server: %w", err)
	}
	var ports []int
	for _, line := range strings.Split(string(lsof), "\n") {
		m := lsListenRE.FindStringSubmatch(line)
		if len(m) < 2 {
			continue
		}
		p, convErr := strconv.Atoi(m[1])
		if convErr == nil {
			ports = append(ports, p)
		}
	}
	if len(ports) == 0 {
		return 0, "", fmt.Errorf("language_server has no listen port")
	}
	return ports[0], csrf, nil
}
