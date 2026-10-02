package cockpit

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)


// QuotaMetric represents an individual quota dimension (percentage + reset time).
type QuotaMetric struct {
	RemainingFraction float64 `json:"remaining_fraction"`
	RemainingPercent  float64 `json:"remaining_percent"`
	ResetTime         string  `json:"reset_time"`
	ResetFriendly     string  `json:"reset_friendly"`
}

// AccountQuota represents an Antigravity account and its 4 key quota dimensions.
type AccountQuota struct {
	ID           string       `json:"id"`
	Email        string       `json:"email"`
	Name         string       `json:"name"`
	IsCurrent    bool         `json:"is_current"`
	Claude5h     *QuotaMetric `json:"claude_5h"`
	ClaudeWeekly *QuotaMetric `json:"claude_weekly"`
	Gemini5h     *QuotaMetric `json:"gemini_5h"`
	GeminiWeekly *QuotaMetric `json:"gemini_weekly"`
	UpdatedAt    int64        `json:"updated_at"`
}

// CockpitQuotaResponse is the payload returned to mobile clients.
type CockpitQuotaResponse struct {
	CurrentAccount *AccountQuota  `json:"current_account"`
	Accounts       []AccountQuota `json:"accounts"`
	UpdatedAt      int64          `json:"updated_at"`
}

type accountsIndex struct {
	CurrentAccountID string `json:"current_account_id"`
	Accounts         []struct {
		ID    string `json:"id"`
		Email string `json:"email"`
		Name  string `json:"name"`
	} `json:"accounts"`
}

type cockpitConfig struct {
	WsEnabled          bool   `json:"ws_enabled"`
	WsPort             int    `json:"ws_port"`
	ReportEnabled      bool   `json:"report_enabled"`
	ReportPort         int    `json:"report_port"`
	ReportToken        string `json:"report_token"`
	AutoRefreshMinutes int    `json:"auto_refresh_minutes"`
	GlobalProxyEnabled bool   `json:"global_proxy_enabled"`
	GlobalProxyURL     string `json:"global_proxy_url"`
	GlobalProxyNoProxy string `json:"global_proxy_no_proxy"`
}

func getCockpitConfig() (*cockpitConfig, error) {
	dataDir, err := GetCockpitDataDir()
	if err != nil {
		return nil, err
	}
	configFile := filepath.Join(dataDir, "config.json")
	cfgBytes, err := os.ReadFile(configFile)
	if err != nil {
		return nil, err
	}
	var cfg cockpitConfig
	if err := json.Unmarshal(cfgBytes, &cfg); err != nil {
		return nil, err
	}

	// Environment variable overrides take precedence
	if envToken := strings.TrimSpace(os.Getenv("COCKPIT_REPORT_TOKEN")); envToken != "" {
		cfg.ReportToken = envToken
	}
	if envPort := strings.TrimSpace(os.Getenv("COCKPIT_REPORT_PORT")); envPort != "" {
		if p, convErr := strconv.Atoi(envPort); convErr == nil && p > 0 {
			cfg.ReportPort = p
		}
	}
	return &cfg, nil
}

// SaveCockpitReportSettings updates report_enabled, report_port, and report_token in ~/.antigravity_cockpit/config.json
// preserving all existing fields.
func SaveCockpitReportSettings(enabled bool, port int, token string) error {
	dataDir, err := GetCockpitDataDir()
	if err != nil {
		return fmt.Errorf("failed to get cockpit data dir: %w", err)
	}
	configFile := filepath.Join(dataDir, "config.json")
	var rawMap map[string]any

	if cfgBytes, err := os.ReadFile(configFile); err == nil {
		if err := json.Unmarshal(cfgBytes, &rawMap); err != nil {
			rawMap = make(map[string]any)
		}
	} else {
		rawMap = make(map[string]any)
	}

	rawMap["report_enabled"] = enabled
	if port > 0 {
		rawMap["report_port"] = port
	}
	if token != "" {
		rawMap["report_token"] = token
	}

	updatedBytes, err := json.MarshalIndent(rawMap, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Backup existing config if present
	if _, err := os.Stat(configFile); err == nil {
		_ = os.WriteFile(configFile+".bak", updatedBytes, 0644)
	}

	tmpFile := configFile + ".tmp"
	if err := os.WriteFile(tmpFile, updatedBytes, 0644); err != nil {
		return fmt.Errorf("failed to write tmp config: %w", err)
	}
	if err := os.Rename(tmpFile, configFile); err != nil {
		return fmt.Errorf("failed to rename config file: %w", err)
	}

	InvalidatePortCache()
	return nil
}

// GenerateSecureToken creates a 32-character cryptographically secure token.
func GenerateSecureToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// CockpitConfigStatus summarizes the current readiness of Cockpit Tools HTTP service.
type CockpitConfigStatus struct {
	Configured    bool
	ReportEnabled bool
	ReportPort    int
	ReportToken   string
	IsDefault     bool
	Reason        string
}

// CheckCockpitConfigStatus inspects whether Cockpit Tools HTTP report service is properly configured.
func CheckCockpitConfigStatus() CockpitConfigStatus {
	cfg, err := getCockpitConfig()
	if err != nil {
		return CockpitConfigStatus{
			Configured: false,
			Reason:     "未找到 Cockpit 配置文件 (~/.antigravity_cockpit/config.json)",
		}
	}
	token := strings.TrimSpace(cfg.ReportToken)
	isDefault := token == "change-this-token"
	port := cfg.ReportPort
	if port <= 0 {
		port = 18081
	}

	if !cfg.ReportEnabled {
		return CockpitConfigStatus{
			Configured:    false,
			ReportEnabled: false,
			ReportPort:    port,
			ReportToken:   token,
			IsDefault:     isDefault,
			Reason:        "HTTP 报表服务未开启 (report_enabled: false)",
		}
	}
	if token == "" || isDefault {
		return CockpitConfigStatus{
			Configured:    false,
			ReportEnabled: true,
			ReportPort:    port,
			ReportToken:   token,
			IsDefault:     isDefault,
			Reason:        "访问 Token 尚未配置 (处于默认占位符 'change-this-token')",
		}
	}

	return CockpitConfigStatus{
		Configured:    true,
		ReportEnabled: true,
		ReportPort:    port,
		ReportToken:   token,
		IsDefault:     false,
	}
}


// GetAutoRefreshInterval reads the configured auto_refresh_minutes from ~/.antigravity_cockpit/config.json.
// If not configured or invalid, returns defaultInterval.
func GetAutoRefreshInterval(defaultInterval time.Duration) time.Duration {
	cfg, err := getCockpitConfig()
	if err == nil && cfg.AutoRefreshMinutes > 0 {
		return time.Duration(cfg.AutoRefreshMinutes) * time.Minute
	}
	return defaultInterval
}

type cachePayload struct {
	UpdatedAt int64 `json:"updatedAt"`
	Payload   struct {
		Models map[string]struct {
			DisplayName string `json:"displayName"`
			QuotaInfo   struct {
				RemainingFraction *float64 `json:"remainingFraction"`
				ResetTime         string   `json:"resetTime"`
			} `json:"quotaInfo"`
		} `json:"models"`
		QuotaSummary struct {
			Groups []struct {
				Buckets []struct {
					BucketID          string   `json:"bucketId"`
					DisplayName       string   `json:"displayName"`
					RemainingFraction *float64 `json:"remainingFraction"`
					ResetTime         string   `json:"resetTime"`
				} `json:"buckets"`
			} `json:"groups"`
		} `json:"quota_summary"`
	} `json:"payload"`
}

var (
	refreshMutex       sync.Mutex
	isRefreshing       bool
	lastRefreshAttempt time.Time
)

// GetCockpitDataDir returns the path to ~/.antigravity_cockpit, or COCKPIT_DATA_DIR if set.
func GetCockpitDataDir() (string, error) {
	if override := os.Getenv("COCKPIT_DATA_DIR"); override != "" {
		return override, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".antigravity_cockpit"), nil
}

// formatResetFriendly converts an ISO timestamp to "3d 11h 34m" (if >24h), "7h 3m" (if <24h), "<1m", or "已就绪".
func formatResetFriendly(isoStr string) string {
	if strings.TrimSpace(isoStr) == "" {
		return "未知"
	}
	t, err := time.Parse(time.RFC3339Nano, isoStr)
	if err != nil {
		t, err = time.Parse(time.RFC3339, isoStr)
		if err != nil {
			return isoStr
		}
	}
	diff := time.Until(t)
	if diff <= 0 {
		return "已就绪"
	}

	totalMinutes := int(diff.Minutes())
	days := totalMinutes / (24 * 60)
	hours := (totalMinutes % (24 * 60)) / 60
	minutes := totalMinutes % 60

	var parts []string
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%dd", days))
	}
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	if minutes > 0 {
		parts = append(parts, fmt.Sprintf("%dm", minutes))
	}
	if len(parts) == 0 {
		return "<1m"
	}
	return strings.Join(parts, " ")
}

func makeMetric(fraction *float64, resetTime string) *QuotaMetric {
	if fraction == nil {
		return nil
	}
	val := *fraction
	if val < 0 {
		val = 0
	}
	if val > 1 {
		val = 1
	}
	return &QuotaMetric{
		RemainingFraction: val,
		RemainingPercent:  float64(int(val*1000+0.5)) / 10.0,
		ResetTime:         resetTime,
		ResetFriendly:     formatResetFriendly(resetTime),
	}
}

type quotaCacheEntry struct {
	data      *CockpitQuotaResponse
	expiresAt time.Time
	emailKey  string
}

var (
	quotaCacheMu  sync.RWMutex
	quotaCache    *quotaCacheEntry
	quotaCacheTTL = 3 * time.Second
)

// InvalidateQuotaCache clears the memory cache of quota information.
func InvalidateQuotaCache() {
	quotaCacheMu.Lock()
	quotaCache = nil
	quotaCacheMu.Unlock()
}

// GetQuotas reads Cockpit Tools' local storage and cache to construct the full quota snapshot.
// Uses a short TTL in-memory cache to prevent disk I/O storms from high-frequency UI polling.
// An optional activeEmail (e.g. from live Language Server) can be passed to prioritize the true runtime account.
func GetQuotas(activeEmails ...string) (*CockpitQuotaResponse, error) {
	emailKey := ""
	if len(activeEmails) > 0 {
		emailKey = strings.ToLower(strings.TrimSpace(activeEmails[0]))
	}

	quotaCacheMu.RLock()
	if quotaCache != nil && time.Now().Before(quotaCache.expiresAt) && quotaCache.emailKey == emailKey {
		cached := quotaCache.data
		quotaCacheMu.RUnlock()
		return cached, nil
	}
	quotaCacheMu.RUnlock()

	dataDir, err := GetCockpitDataDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get cockpit data dir: %w", err)
	}

	accountsFile := filepath.Join(dataDir, "accounts.json")
	accBytes, err := os.ReadFile(accountsFile)
	if err != nil {
		if os.IsNotExist(err) {
			emptyResp := &CockpitQuotaResponse{
				Accounts:  []AccountQuota{},
				UpdatedAt: time.Now().Unix(),
			}
			quotaCacheMu.Lock()
			quotaCache = &quotaCacheEntry{
				data:      emptyResp,
				emailKey:  emailKey,
				expiresAt: time.Now().Add(30 * time.Second),
			}
			quotaCacheMu.Unlock()
			return emptyResp, nil
		}
		return nil, fmt.Errorf("failed to read accounts.json: %w", err)
	}

	var idx accountsIndex
	if err := json.Unmarshal(accBytes, &idx); err != nil {
		return nil, fmt.Errorf("failed to parse accounts.json: %w", err)
	}

	resolvedCurrentID := ""

	// 1. Live Language Server email — this is the account Antigravity is actually
	// running as. Cockpit's current_account_id can be updated even when token
	// injection fails to change the runtime identity.
	if len(activeEmails) > 0 && strings.TrimSpace(activeEmails[0]) != "" {
		targetEmail := strings.ToLower(strings.TrimSpace(activeEmails[0]))
		for _, acc := range idx.Accounts {
			if strings.ToLower(strings.TrimSpace(acc.Email)) == targetEmail {
				resolvedCurrentID = acc.ID
				break
			}
		}
	}

	// 2. Cockpit accounts.json current_account_id (intended account, if live
	// identity is unavailable)
	if resolvedCurrentID == "" && strings.TrimSpace(idx.CurrentAccountID) != "" {
		for _, acc := range idx.Accounts {
			if acc.ID == strings.TrimSpace(idx.CurrentAccountID) {
				resolvedCurrentID = acc.ID
				break
			}
		}
	}

	// 3. Cockpit Legacy Desktop bound account (Priority 3 fallback)
	if resolvedCurrentID == "" {
		legacyInstFile := filepath.Join(dataDir, "antigravity_legacy_instances.json")
		if lBytes, err := os.ReadFile(legacyInstFile); err == nil {
			var leg struct {
				DefaultSettings struct {
					BindAccountId string `json:"bindAccountId"`
				} `json:"defaultSettings"`
			}
			if err := json.Unmarshal(lBytes, &leg); err == nil && leg.DefaultSettings.BindAccountId != "" {
				for _, acc := range idx.Accounts {
					if acc.ID == leg.DefaultSettings.BindAccountId {
						resolvedCurrentID = acc.ID
						break
					}
				}
			}
		}
	}

	cacheDir := filepath.Join(dataDir, "cache", "quota_api_v1_desktop", "authorized")
	var accountsList []AccountQuota
	var currentAcc *AccountQuota
	maxUpdatedAt := int64(0)

	for _, acc := range idx.Accounts {
		emailNorm := strings.ToLower(strings.TrimSpace(acc.Email))
		h := sha256.Sum256([]byte(emailNorm))
		hashHex := hex.EncodeToString(h[:])
		cachePath := filepath.Join(cacheDir, hashHex+".json")

		item := AccountQuota{
			ID:        acc.ID,
			Email:     acc.Email,
			Name:      acc.Name,
			IsCurrent: acc.ID == resolvedCurrentID,
		}

		if cBytes, err := os.ReadFile(cachePath); err == nil {
			var cp cachePayload
			if err := json.Unmarshal(cBytes, &cp); err == nil {
				item.UpdatedAt = cp.UpdatedAt
				if cp.UpdatedAt > maxUpdatedAt {
					maxUpdatedAt = cp.UpdatedAt
				}

				// 1. Check QuotaSummary buckets first
				for _, g := range cp.Payload.QuotaSummary.Groups {
					for _, b := range g.Buckets {
						switch b.BucketID {
						case "gemini-5h", "gemini_5h":
							item.Gemini5h = makeMetric(b.RemainingFraction, b.ResetTime)
						case "gemini-weekly", "gemini_weekly":
							item.GeminiWeekly = makeMetric(b.RemainingFraction, b.ResetTime)
						case "3p-5h", "claude-5h", "claude_5h":
							item.Claude5h = makeMetric(b.RemainingFraction, b.ResetTime)
						case "3p-weekly", "claude-weekly", "claude_weekly":
							item.ClaudeWeekly = makeMetric(b.RemainingFraction, b.ResetTime)
						}
					}
				}

				// 2. Fallbacks to model-level quotaInfo if bucket was not populated
				if item.Claude5h == nil {
					if m, ok := cp.Payload.Models["claude-opus-4-6-thinking"]; ok && m.QuotaInfo.RemainingFraction != nil {
						item.Claude5h = makeMetric(m.QuotaInfo.RemainingFraction, m.QuotaInfo.ResetTime)
					} else if m, ok := cp.Payload.Models["claude-sonnet-4-6"]; ok && m.QuotaInfo.RemainingFraction != nil {
						item.Claude5h = makeMetric(m.QuotaInfo.RemainingFraction, m.QuotaInfo.ResetTime)
					}
				}
				if item.Gemini5h == nil {
					if m, ok := cp.Payload.Models["gemini-2.5-pro"]; ok && m.QuotaInfo.RemainingFraction != nil {
						item.Gemini5h = makeMetric(m.QuotaInfo.RemainingFraction, m.QuotaInfo.ResetTime)
					}
				}
			}
		}

		// Ensure fallback defaults so UI doesn't crash if an account is brand new or unprobed
		if item.Gemini5h == nil {
			item.Gemini5h = &QuotaMetric{RemainingPercent: 100, ResetFriendly: "就绪"}
		}
		if item.GeminiWeekly == nil {
			item.GeminiWeekly = &QuotaMetric{RemainingPercent: 100, ResetFriendly: "就绪"}
		}
		if item.Claude5h == nil {
			item.Claude5h = &QuotaMetric{RemainingPercent: 100, ResetFriendly: "就绪"}
		}
		if item.ClaudeWeekly == nil {
			item.ClaudeWeekly = &QuotaMetric{RemainingPercent: 100, ResetFriendly: "就绪"}
		}

		if item.IsCurrent {
			currentAcc = &item
		}
		accountsList = append(accountsList, item)
	}

	// Sort accounts so current active account is ALWAYS at the very top
	var ordered []AccountQuota
	if currentAcc != nil {
		ordered = append(ordered, *currentAcc)
	}
	for _, a := range accountsList {
		if currentAcc != nil && a.ID == currentAcc.ID {
			continue
		}
		ordered = append(ordered, a)
	}

	if maxUpdatedAt == 0 {
		maxUpdatedAt = time.Now().UnixMilli()
	}

	resp := &CockpitQuotaResponse{
		CurrentAccount: currentAcc,
		Accounts:       ordered,
		UpdatedAt:      maxUpdatedAt,
	}

	quotaCacheMu.Lock()
	quotaCache = &quotaCacheEntry{
		data:      resp,
		expiresAt: time.Now().Add(quotaCacheTTL),
		emailKey:  emailKey,
	}
	quotaCacheMu.Unlock()

	return resp, nil
}

// TriggerRefresh triggers a fresh quota fetch across all accounts in Cockpit Tools.
// Optional force parameter bypasses the attempt debounce if set to true.
func TriggerRefresh(force ...bool) error {
	refreshMutex.Lock()
	isForce := len(force) > 0 && force[0]

	if isRefreshing {
		refreshMutex.Unlock()
		return nil
	}
	if !isForce && time.Since(lastRefreshAttempt) < 10*time.Second {
		refreshMutex.Unlock()
		return nil
	}
	lastRefreshAttempt = time.Now()
	isRefreshing = true
	refreshMutex.Unlock()

	cfg, err := getCockpitConfig()
	if err != nil {
		refreshMutex.Lock()
		isRefreshing = false
		refreshMutex.Unlock()
		return err
	}

	reportPort := cfg.ReportPort
	if cfg.ReportToken != "" {
		if resolved, rErr := ResolveActiveReportPort(cfg.ReportToken, cfg.ReportPort); rErr == nil && resolved > 0 {
			reportPort = resolved
		}
	}

	if reportPort > 0 && cfg.ReportToken != "" {
		go func(port int, token string) {
			defer func() {
				refreshMutex.Lock()
				isRefreshing = false
				refreshMutex.Unlock()
			}()

			if err := QueryReport(port, token); err != nil {
				log.Printf("[Cockpit] Report request error: %v", err)
			} else {
				InvalidateQuotaCache()
			}
		}(reportPort, cfg.ReportToken)
		return nil
	}

	refreshMutex.Lock()
	isRefreshing = false
	refreshMutex.Unlock()
	return nil
}

// QueryReport sends an HTTP GET request to the local Cockpit Tools report endpoint to trigger fresh quota collection.
func QueryReport(port int, token string) error {
	url := fmt.Sprintf("http://127.0.0.1:%d/report?token=%s&format=yaml", port, token)
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusUnauthorized {
			return fmt.Errorf("HTTP 401 Unauthorized (token 错误或无效)")
		}
		return fmt.Errorf("HTTP %d %s", resp.StatusCode, resp.Status)
	}

	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

// RefreshQuotas triggers a refresh and polls up to 10 seconds for updated cache data.
func RefreshQuotas(activeEmails ...string) (*CockpitQuotaResponse, error) {
	ResetAutoRefreshCooldown()
	currentQuotas, _ := GetQuotas(activeEmails...)
	var initialUpdatedAt int64
	if currentQuotas != nil {
		initialUpdatedAt = currentQuotas.UpdatedAt
	}

	if err := TriggerRefresh(true); err != nil {
		return currentQuotas, err
	}

	// Poll GetQuotas every 500ms up to 10 seconds (well within client's 15s timeout)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(500 * time.Millisecond)
		latest, err := GetQuotas(activeEmails...)
		if err == nil && latest != nil && latest.UpdatedAt > initialUpdatedAt {
			return latest, nil
		}
	}

	// If timeout reached before updates observed, return the latest available snapshot
	return GetQuotas(activeEmails...)
}

// DetectAntigravityAppPath locates the installed Antigravity application path on the current system.
func DetectAntigravityAppPath() string {
	if runtime.GOOS == "darwin" {
		candidates := []string{
			"/Applications/Antigravity.app",
			"/Applications/Antigravity IDE.app",
		}
		if home, err := os.UserHomeDir(); err == nil {
			candidates = append(candidates,
				filepath.Join(home, "Applications", "Antigravity.app"),
				filepath.Join(home, "Applications", "Antigravity IDE.app"),
			)
		}
		for _, p := range candidates {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
		return ""
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
		for _, p := range candidatePaths {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
		if looked, err := exec.LookPath("Antigravity.exe"); err == nil {
			return looked
		}
		return ""
	}

	// Linux
	for _, bin := range []string{"antigravity", "antigravity-ide"} {
		if looked, err := exec.LookPath(bin); err == nil {
			return looked
		}
	}
	return ""
}

// EnsureCockpitAntigravityConfig checks and aligns ~/.antigravity_cockpit/config.json:
// 1. Sets antigravity_launch_on_switch to false so that Cockpit does not abort switch requests
//    with APP_PATH_NOT_FOUND when Antigravity is not currently running (gateway manages graceful quit & relaunch).
// 2. Sets antigravity_app_path to the detected local Antigravity installation if missing or invalid.
// 3. On macOS, creates an Electron symlink inside Contents/MacOS if needed for legacy compatibility.
func EnsureCockpitAntigravityConfig() error {
	dataDir, err := GetCockpitDataDir()
	if err != nil {
		return err
	}
	configFile := filepath.Join(dataDir, "config.json")
	cfgBytes, err := os.ReadFile(configFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Cockpit not configured yet
		}
		return err
	}

	var rawMap map[string]any
	if err := json.Unmarshal(cfgBytes, &rawMap); err != nil {
		return fmt.Errorf("parse config.json: %w", err)
	}

	changed := false

	// 1. Ensure antigravity_launch_on_switch is false
	if val, ok := rawMap["antigravity_launch_on_switch"].(bool); !ok || val {
		rawMap["antigravity_launch_on_switch"] = false
		changed = true
	}

	// 2. Detect and ensure antigravity_app_path
	detected := DetectAntigravityAppPath()
	if detected != "" {
		currPath, _ := rawMap["antigravity_app_path"].(string)
		currPath = strings.TrimSpace(currPath)
		needUpdatePath := false
		if currPath == "" {
			needUpdatePath = true
		} else if _, statErr := os.Stat(currPath); statErr != nil {
			needUpdatePath = true
		}
		if needUpdatePath {
			rawMap["antigravity_app_path"] = detected
			changed = true
		}

		// 3. On macOS, ensure Electron -> Antigravity symlink exists inside the app bundle
		if runtime.GOOS == "darwin" {
			agBin := filepath.Join(detected, "Contents", "MacOS", "Antigravity")
			elBin := filepath.Join(detected, "Contents", "MacOS", "Electron")
			if _, statErr := os.Stat(agBin); statErr == nil {
				if _, elErr := os.Lstat(elBin); os.IsNotExist(elErr) {
					_ = os.Symlink("Antigravity", elBin)
				}
			}
		}
	}

	// 4. Ensure state.vscdb SQLite databases and Antigravity IDE paths exist
	ensureAntigravityStateDBs()

	if !changed {
		return nil
	}

	updatedBytes, err := json.MarshalIndent(rawMap, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal updated config: %w", err)
	}

	// Backup existing config
	_ = os.WriteFile(configFile+".bak", cfgBytes, 0644)

	tmpFile := configFile + ".tmp"
	if err := os.WriteFile(tmpFile, append(updatedBytes, '\n'), 0644); err != nil {
		return fmt.Errorf("write tmp config: %w", err)
	}
	if err := os.Rename(tmpFile, configFile); err != nil {
		return fmt.Errorf("rename config: %w", err)
	}

	log.Printf("[Cockpit] Auto-aligned config.json: antigravity_launch_on_switch=false, antigravity_app_path=%q", rawMap["antigravity_app_path"])

	// 5. If Cockpit Tools process is running, restart it so the in-memory config is reloaded
	if isCockpitProcessRunning() {
		log.Println("[Cockpit] Restarting Cockpit Tools to reload updated configuration into memory...")
		if rErr := RestartCockpitApp(); rErr != nil {
			log.Printf("[Cockpit] Warning: failed to restart Cockpit Tools: %v", rErr)
		} else {
			time.Sleep(1 * time.Second)
		}
	}

	return nil
}

