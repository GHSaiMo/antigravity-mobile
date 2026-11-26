package cockpit

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)


// QuotaMetric represents an individual quota dimension (percentage + reset time).
type QuotaMetric struct {
	RemainingFraction  float64 `json:"remaining_fraction"`
	RemainingPercent   float64 `json:"remaining_percent"`
	ResetTime          string  `json:"reset_time"`
	ResetFriendly      string  `json:"reset_friendly"`
	EstCycleUsedTokens int64   `json:"est_cycle_used_tokens,omitempty"`
	EstRemainingTokens int64   `json:"est_remaining_tokens,omitempty"`
	EstTotalTokens     int64   `json:"est_total_tokens,omitempty"`
	EstReliable        bool    `json:"est_reliable,omitempty"`
}

// AccountQuota represents an Antigravity account and its quota dimensions.
type AccountQuota struct {
	ID               string       `json:"id"`
	Email            string       `json:"email"`
	Name             string       `json:"name"`
	SubscriptionTier string       `json:"subscription_tier,omitempty"`
	IsCurrent        bool         `json:"is_current"`
	Claude5h         *QuotaMetric `json:"claude_5h"`
	ClaudeWeekly     *QuotaMetric `json:"claude_weekly"`
	Gemini5h         *QuotaMetric `json:"gemini_5h"`
	GeminiWeekly     *QuotaMetric `json:"gemini_weekly"`
	Gemini4Argon     *QuotaMetric `json:"gemini_4_argon,omitempty"`
	UpdatedAt        int64        `json:"updated_at"`
}

// CockpitQuotaResponse is the payload returned to mobile clients.
type CockpitQuotaResponse struct {
	CurrentAccount *AccountQuota  `json:"current_account"`
	Accounts       []AccountQuota `json:"accounts"`
	UpdatedAt      int64          `json:"updated_at"`

	// Source is "cockpit" (data from Cockpit Tools) or "live" (read from the Antigravity language server).
	Source           string `json:"source,omitempty"`
	CockpitInstalled bool   `json:"cockpit_installed"`
	CockpitHint      string `json:"cockpit_hint,omitempty"`
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
	var existingBytes []byte

	if cfgBytes, err := os.ReadFile(configFile); err == nil {
		existingBytes = cfgBytes
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

	// Backup previous config (not updatedBytes) with restricted 0600 permissions
	if len(existingBytes) > 0 {
		_ = writeFileAtomic0600(configFile+".bak", existingBytes)
	}

	if err := writeFileAtomic0600(configFile, updatedBytes); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
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

type quotaSummaryGroup struct {
	DisplayName string `json:"displayName,omitempty"`
	Description string `json:"description,omitempty"`
	Buckets     []struct {
		BucketID          string   `json:"bucketId"`
		DisplayName       string   `json:"displayName"`
		RemainingFraction *float64 `json:"remainingFraction"`
		ResetTime         string   `json:"resetTime"`
	} `json:"buckets"`
}

type cachePayload struct {
	UpdatedAt int64 `json:"updatedAt"`
	Payload   struct {
		SubscriptionTier string `json:"subscription_tier,omitempty"`
		Models           map[string]struct {
			DisplayName string `json:"displayName"`
			QuotaInfo   struct {
				RemainingFraction *float64 `json:"remainingFraction"`
				ResetTime         string   `json:"resetTime"`
			} `json:"quotaInfo"`
		} `json:"models"`
		QuotaSummary struct {
			Groups []quotaSummaryGroup `json:"groups"`
		} `json:"quota_summary"`
	} `json:"payload"`
}

type storedAccountQuota struct {
	SubscriptionTier string `json:"subscription_tier"`
	Models           []struct {
		Name        string `json:"name"`
		DisplayName string `json:"display_name"`
		Percentage  *int   `json:"percentage"`
		ResetTime   string `json:"reset_time"`
	} `json:"models"`
}

func extractStoredAccountQuota(raw any) *storedAccountQuota {
	if raw == nil {
		return nil
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var sq storedAccountQuota
	if err := json.Unmarshal(b, &sq); err != nil {
		return nil
	}
	return &sq
}

// normalizeSubscriptionTier maps Cockpit Tools' raw subscription_tier string
// (e.g. "g1-ultra-tier", "g1-pro-tier", "free-tier") to "ULTRA", "PRO", "FREE", or "".
func normalizeSubscriptionTier(raw string) string {
	t := strings.ToLower(strings.TrimSpace(raw))
	if t == "" || t == "unknown" {
		return ""
	}
	if strings.Contains(t, "ultra") {
		return "ULTRA"
	}
	if strings.Contains(t, "pro") {
		return "PRO"
	}
	return "FREE"
}

// accountTierSortRank orders backup accounts by subscription tier:
// ULTRA first (0), PRO/other paid or unknown tiers in the middle (1), FREE last (2).
// Stable sorting preserves the original Cockpit index order within each tier group.
func accountTierSortRank(a AccountQuota) int {
	tier := strings.ToUpper(strings.TrimSpace(a.SubscriptionTier))
	switch {
	case strings.Contains(tier, "ULTRA"):
		return 0
	case strings.Contains(tier, "PRO"):
		return 1
	case strings.Contains(tier, "FREE"):
		return 2
	case a.Gemini5h == nil && a.Claude5h == nil:
		return 2
	default:
		return 1
	}
}

func isArgonModelOrBucket(id string) bool {
	lower := strings.ToLower(strings.TrimSpace(id))
	if lower == "" || strings.HasSuffix(lower, "-disabled") || strings.HasSuffix(lower, "_disabled") {
		return false
	}
	return strings.Contains(lower, "argon")
}

func groupMentionsArgon(displayName, description string) bool {
	return strings.Contains(strings.ToLower(displayName), "argon") ||
		strings.Contains(strings.ToLower(description), "argon")
}

// isQuotaMetricMirrored reports whether candidate (e.g. model-level Gemini 4 Argon) is
// simply mirroring another bucket (e.g. Gemini 5h / Gemini Weekly) after upstream folded
// the standalone Argon pool into the main Gemini pool. Allows ±0.5% fraction rounding and
// ±60s resetTime jitter between group-level and model-level fields.
func isQuotaMetricMirrored(candidate, ref *QuotaMetric) bool {
	if candidate == nil || ref == nil {
		return false
	}
	if math.Abs(candidate.RemainingFraction-ref.RemainingFraction) > 0.005 {
		return false
	}
	tA, errA := time.Parse(time.RFC3339, strings.TrimSpace(candidate.ResetTime))
	tB, errB := time.Parse(time.RFC3339, strings.TrimSpace(ref.ResetTime))
	if errA != nil || errB != nil {
		return false
	}
	diff := tA.Sub(tB)
	if diff < 0 {
		diff = -diff
	}
	return diff <= 60*time.Second
}

func isArgonMergedIntoGemini(acc *AccountQuota, hasDedicatedArgonBucket, argonMergedInGeminiGroup bool) bool {
	if acc == nil || acc.Gemini4Argon == nil || hasDedicatedArgonBucket {
		return false
	}
	if argonMergedInGeminiGroup {
		return true
	}
	return isQuotaMetricMirrored(acc.Gemini4Argon, acc.Gemini5h) ||
		isQuotaMetricMirrored(acc.Gemini4Argon, acc.GeminiWeekly)
}

var (
	refreshMutex       sync.Mutex
	isRefreshing       bool
	refreshDone        chan struct{}
	lastRefreshAttempt time.Time
)

// GetCockpitDataDir returns the path to Cockpit Tools data directory.
// Resolution order aligns with official Cockpit Tools:
// 1. COCKPIT_TOOLS_DATA_DIR environment variable (official)
// 2. COCKPIT_DATA_DIR environment variable (legacy)
// 3. ~/.cockpit_tools directory (official primary default)
// 4. ~/.antigravity_cockpit directory (legacy default)
// If neither exists, defaults to ~/.cockpit_tools and ensures bidirectional symlink compatibility.
func GetCockpitDataDir() (string, error) {
	if override := strings.TrimSpace(os.Getenv("COCKPIT_TOOLS_DATA_DIR")); override != "" {
		return override, nil
	}
	if override := strings.TrimSpace(os.Getenv("COCKPIT_DATA_DIR")); override != "" {
		return override, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	officialDir := filepath.Join(home, ".cockpit_tools")
	legacyDir := filepath.Join(home, ".antigravity_cockpit")

	// 1. If official directory exists, prefer it
	if fi, sErr := os.Stat(officialDir); sErr == nil && fi.IsDir() {
		// Ensure legacy symlink exists for backward compatibility if missing
		if _, lErr := os.Lstat(legacyDir); os.IsNotExist(lErr) {
			_ = os.Symlink(officialDir, legacyDir)
		}
		return officialDir, nil
	}

	// 2. If legacy directory exists, use it and ensure official symlink exists
	if fi, sErr := os.Stat(legacyDir); sErr == nil && fi.IsDir() {
		if _, oErr := os.Lstat(officialDir); os.IsNotExist(oErr) {
			_ = os.Symlink(legacyDir, officialDir)
		}
		return legacyDir, nil
	}

	// 3. Default to official directory
	return officialDir, nil
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
	resetTime = strings.TrimSpace(resetTime)
	friendly := formatResetFriendly(resetTime)
	// Upstream retrieveUserQuotaSummary omits resetTime for full (100%) rolling
	// windows when no active countdown has started (e.g. ULTRA 3p-5h replenished
	// to 100% while 3p-weekly is partially used). Treat full + empty resetTime
	// as ready rather than unknown.
	if resetTime == "" && val >= 1.0 {
		friendly = "已就绪"
	}
	return &QuotaMetric{
		RemainingFraction: val,
		RemainingPercent:  float64(int(val*1000+0.5)) / 10.0,
		ResetTime:         resetTime,
		ResetFriendly:     friendly,
	}
}

type quotaCacheEntry struct {
	data      *CockpitQuotaResponse
	expiresAt time.Time
}

type decryptedQuotaCacheEntry struct {
	modTime int64
	quota   *storedAccountQuota
}

type rawQuotaFileCacheEntry struct {
	modTime int64
	size    int64
	payload cachePayload
}

var (
	quotaCacheMu        sync.RWMutex
	quotaCacheByEmail   = make(map[string]*quotaCacheEntry)
	decryptedQuotaCache = make(map[string]decryptedQuotaCacheEntry)
	rawQuotaFileCache   = make(map[string]rawQuotaFileCacheEntry)
	quotaCacheTTL       = 3 * time.Second
)

// InvalidateQuotaCache clears the memory cache of quota information.
func InvalidateQuotaCache() {
	quotaCacheMu.Lock()
	quotaCacheByEmail = make(map[string]*quotaCacheEntry)
	decryptedQuotaCache = make(map[string]decryptedQuotaCacheEntry)
	rawQuotaFileCache = make(map[string]rawQuotaFileCacheEntry)
	quotaCacheMu.Unlock()
	invalidateModernAccountIndexCache()
}

func loadCachedQuotaPayloadFile(cachePath string) (cachePayload, bool) {
	fi, err := os.Stat(cachePath)
	if err != nil || fi.IsDir() {
		return cachePayload{}, false
	}
	mtime := fi.ModTime().UnixNano()
	sz := fi.Size()

	quotaCacheMu.RLock()
	if cached, ok := rawQuotaFileCache[cachePath]; ok && cached.modTime == mtime && cached.size == sz {
		cp := cached.payload
		quotaCacheMu.RUnlock()
		return cp, true
	}
	quotaCacheMu.RUnlock()

	cBytes, err := os.ReadFile(cachePath)
	if err != nil {
		return cachePayload{}, false
	}
	var cp cachePayload
	if err := json.Unmarshal(cBytes, &cp); err != nil {
		return cachePayload{}, false
	}

	quotaCacheMu.Lock()
	if len(rawQuotaFileCache) > 64 {
		rawQuotaFileCache = make(map[string]rawQuotaFileCacheEntry)
	}
	rawQuotaFileCache[cachePath] = rawQuotaFileCacheEntry{
		modTime: mtime,
		size:    sz,
		payload: cp,
	}
	quotaCacheMu.Unlock()
	return cp, true
}

func getCachedStoredAccountQuota(dataDir, accountID string) *storedAccountQuota {
	accountFilePath := filepath.Join(dataDir, "accounts", accountID+".json")
	fi, err := os.Stat(accountFilePath)
	if err != nil || fi.IsDir() {
		return nil
	}
	mtime := fi.ModTime().UnixNano()

	quotaCacheMu.RLock()
	if cached, ok := decryptedQuotaCache[accountID]; ok && cached.modTime == mtime {
		quotaCacheMu.RUnlock()
		return cached.quota
	}
	quotaCacheMu.RUnlock()

	detail, dErr := DecryptAccountByExactID(accountID)
	if dErr != nil || detail == nil {
		return nil
	}
	sq := extractStoredAccountQuota(detail.Quota)

	quotaCacheMu.Lock()
	if len(decryptedQuotaCache) > 64 {
		decryptedQuotaCache = make(map[string]decryptedQuotaCacheEntry)
	}
	decryptedQuotaCache[accountID] = decryptedQuotaCacheEntry{
		modTime: mtime,
		quota:   sq,
	}
	quotaCacheMu.Unlock()
	return sq
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
	if entry := quotaCacheByEmail[emailKey]; entry != nil && time.Now().Before(entry.expiresAt) {
		cached := entry.data
		quotaCacheMu.RUnlock()
		return cached, nil
	}
	quotaCacheMu.RUnlock()

	dataDir, err := GetCockpitDataDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get cockpit data dir: %w", err)
	}

	// The index lives in accounts.json on Cockpit Tools <= v1.3 and in
	// antigravity_accounts.sqlite on v1.4+. LoadAccountIndex hides the difference;
	// an absent index yields an empty account list rather than an error.
	idx, err := LoadAccountIndex(dataDir)
	if err != nil {
		return nil, err
	}
	if len(idx.Accounts) == 0 {
		emptyResp := &CockpitQuotaResponse{
			Accounts:  []AccountQuota{},
			UpdatedAt: time.Now().Unix(),
		}
		quotaCacheMu.Lock()
		if len(quotaCacheByEmail) > 16 {
			quotaCacheByEmail = make(map[string]*quotaCacheEntry)
		}
		quotaCacheByEmail[emailKey] = &quotaCacheEntry{
			data:      emptyResp,
			expiresAt: time.Now().Add(30 * time.Second),
		}
		quotaCacheMu.Unlock()
		return emptyResp, nil
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

		// 1. Read account detail from ~/.cockpit_tools/accounts/<id>.json first
		// to determine subscription_tier and have model percentage fallback ready (cached by mtime).
		sq := getCachedStoredAccountQuota(dataDir, acc.ID)
		if sq != nil && sq.SubscriptionTier != "" {
			item.SubscriptionTier = normalizeSubscriptionTier(sq.SubscriptionTier)
		}

		argonMergedInGeminiGroup := false
		hasCacheModels := false
		if cp, ok := loadCachedQuotaPayloadFile(cachePath); ok {
			{
				item.UpdatedAt = cp.UpdatedAt
				if cp.UpdatedAt > maxUpdatedAt {
					maxUpdatedAt = cp.UpdatedAt
				}
				if item.SubscriptionTier == "" && cp.Payload.SubscriptionTier != "" {
					item.SubscriptionTier = normalizeSubscriptionTier(cp.Payload.SubscriptionTier)
				}

				isFree := item.SubscriptionTier == "FREE"
				hasDedicatedArgonBucket := false
				hasCacheModels = len(cp.Payload.Models) > 0

				// 2. Check QuotaSummary buckets first
				for _, g := range cp.Payload.QuotaSummary.Groups {
					groupHasGemini := false
					for _, b := range g.Buckets {
						switch b.BucketID {
						case "gemini-5h", "gemini_5h":
							groupHasGemini = true
							if !isFree {
								item.Gemini5h = makeMetric(b.RemainingFraction, b.ResetTime)
							}
						case "gemini-weekly", "gemini_weekly":
							groupHasGemini = true
							item.GeminiWeekly = makeMetric(b.RemainingFraction, b.ResetTime)
						case "3p-5h", "claude-5h", "claude_5h":
							if !isFree {
								item.Claude5h = makeMetric(b.RemainingFraction, b.ResetTime)
							}
						case "3p-weekly", "claude-weekly", "claude_weekly":
							item.ClaudeWeekly = makeMetric(b.RemainingFraction, b.ResetTime)
						default:
							if item.Gemini4Argon == nil && isArgonModelOrBucket(b.BucketID) && b.RemainingFraction != nil {
								item.Gemini4Argon = makeMetric(b.RemainingFraction, b.ResetTime)
								hasDedicatedArgonBucket = true
							}
						}
					}
					if groupHasGemini && groupMentionsArgon(g.DisplayName, g.Description) {
						argonMergedInGeminiGroup = true
					}
				}

				// 3. Fallbacks to model-level quotaInfo if bucket was not populated
				if isFree {
					// Free accounts strictly only have Weekly quotas, no 5h rolling window
					item.Claude5h = nil
					item.Gemini5h = nil
					if item.ClaudeWeekly == nil {
						for _, k := range []string{"claude-opus-5-5-high", "claude-sonnet-5-5-high", "claude-opus-4-6-thinking", "claude-sonnet-4-6"} {
							if m, ok := cp.Payload.Models[k]; ok && m.QuotaInfo.RemainingFraction != nil {
								item.ClaudeWeekly = makeMetric(m.QuotaInfo.RemainingFraction, m.QuotaInfo.ResetTime)
								break
							}
						}
						if item.ClaudeWeekly == nil {
							for k, m := range cp.Payload.Models {
								lk := strings.ToLower(k)
								if (strings.Contains(lk, "claude") || strings.Contains(lk, "gpt")) && m.QuotaInfo.RemainingFraction != nil {
									item.ClaudeWeekly = makeMetric(m.QuotaInfo.RemainingFraction, m.QuotaInfo.ResetTime)
									break
								}
							}
						}
					}
					if item.GeminiWeekly == nil {
						for _, k := range []string{"gemini-4-hydrogen-high", "gemini-4-hydrogen-medium", "gemini-4-carbon-high", "gemini-3.8-flash-high", "gemini-3.8-flash-medium", "gemini-3-flash", "gemini-2.5-pro"} {
							if m, ok := cp.Payload.Models[k]; ok && m.QuotaInfo.RemainingFraction != nil {
								item.GeminiWeekly = makeMetric(m.QuotaInfo.RemainingFraction, m.QuotaInfo.ResetTime)
								break
							}
						}
						if item.GeminiWeekly == nil {
							for k, m := range cp.Payload.Models {
								lk := strings.ToLower(k)
								if strings.Contains(lk, "gemini") && !isArgonModelOrBucket(lk) && m.QuotaInfo.RemainingFraction != nil {
									item.GeminiWeekly = makeMetric(m.QuotaInfo.RemainingFraction, m.QuotaInfo.ResetTime)
									break
								}
							}
						}
					}
				} else {
					if item.Claude5h == nil {
						for _, k := range []string{"claude-opus-5-5-high", "claude-sonnet-5-5-high", "claude-opus-4-6-thinking", "claude-sonnet-4-6"} {
							if m, ok := cp.Payload.Models[k]; ok && m.QuotaInfo.RemainingFraction != nil {
								item.Claude5h = makeMetric(m.QuotaInfo.RemainingFraction, m.QuotaInfo.ResetTime)
								break
							}
						}
					}
					if item.Gemini5h == nil {
						for _, k := range []string{"gemini-4-neon-high", "gemini-4-carbon-high", "gemini-4-hydrogen-high", "gemini-3.8-flash-high", "gemini-2.5-pro"} {
							if m, ok := cp.Payload.Models[k]; ok && m.QuotaInfo.RemainingFraction != nil {
								item.Gemini5h = makeMetric(m.QuotaInfo.RemainingFraction, m.QuotaInfo.ResetTime)
								break
							}
						}
						if item.Gemini5h == nil {
							for k, m := range cp.Payload.Models {
								lk := strings.ToLower(k)
								if strings.Contains(lk, "gemini") && !isArgonModelOrBucket(lk) && m.QuotaInfo.RemainingFraction != nil {
									item.Gemini5h = makeMetric(m.QuotaInfo.RemainingFraction, m.QuotaInfo.ResetTime)
									break
								}
							}
						}
					}
				}

				if !argonMergedInGeminiGroup && item.Gemini4Argon == nil && len(cp.Payload.Models) > 0 {
					for _, key := range []string{"gemini-4-argon-high", "gemini-4-argon-medium", "gemini-4-argon-low", "gemini-4-argon"} {
						if m, ok := cp.Payload.Models[key]; ok && m.QuotaInfo.RemainingFraction != nil {
							item.Gemini4Argon = makeMetric(m.QuotaInfo.RemainingFraction, m.QuotaInfo.ResetTime)
							break
						}
					}
					if item.Gemini4Argon == nil {
						for k, m := range cp.Payload.Models {
							if isArgonModelOrBucket(k) && m.QuotaInfo.RemainingFraction != nil {
								item.Gemini4Argon = makeMetric(m.QuotaInfo.RemainingFraction, m.QuotaInfo.ResetTime)
								break
							}
						}
					}
				}

				if isArgonMergedIntoGemini(&item, hasDedicatedArgonBucket, argonMergedInGeminiGroup) {
					item.Gemini4Argon = nil
				}
			}
		}

		// Fallback argon from sq only when desktop cache didn't already supply model quotas or confirm merge.
		if !argonMergedInGeminiGroup && !hasCacheModels && sq != nil && item.Gemini4Argon == nil {
			for _, m := range sq.Models {
				if isArgonModelOrBucket(m.Name) && m.Percentage != nil {
					frac := float64(*m.Percentage) / 100.0
					cand := makeMetric(&frac, m.ResetTime)
					if !isQuotaMetricMirrored(cand, item.Gemini5h) && !isQuotaMetricMirrored(cand, item.GeminiWeekly) {
						item.Gemini4Argon = cand
					}
					break
				}
			}
		}

		// Ensure fallback defaults for base buckets so UI doesn't crash if an account is brand new or unprobed.
		// Free accounts strictly only have 2 weekly quotas.
		if item.SubscriptionTier == "FREE" {
			item.Claude5h = nil
			item.Gemini5h = nil
		} else {
			if item.Gemini5h == nil {
				item.Gemini5h = &QuotaMetric{RemainingPercent: 100, ResetFriendly: "就绪"}
			}
			if item.Claude5h == nil {
				item.Claude5h = &QuotaMetric{RemainingPercent: 100, ResetFriendly: "就绪"}
			}
		}
		if item.GeminiWeekly == nil {
			item.GeminiWeekly = &QuotaMetric{RemainingPercent: 100, ResetFriendly: "就绪"}
		}
		if item.ClaudeWeekly == nil {
			item.ClaudeWeekly = &QuotaMetric{RemainingPercent: 100, ResetFriendly: "就绪"}
		}

		if item.IsCurrent {
			currentAcc = &item
		}
		accountsList = append(accountsList, item)
	}

	// Sort accounts so current active account is ALWAYS at the very top,
	// and backup accounts prioritize ULTRA first and FREE last while
	// preserving their original relative order within each tier group.
	ordered := make([]AccountQuota, 0, len(accountsList))
	if currentAcc != nil {
		ordered = append(ordered, *currentAcc)
	}
	backups := make([]AccountQuota, 0, len(accountsList))
	for _, a := range accountsList {
		if currentAcc != nil && a.ID == currentAcc.ID {
			continue
		}
		backups = append(backups, a)
	}
	sort.SliceStable(backups, func(i, j int) bool {
		return accountTierSortRank(backups[i]) < accountTierSortRank(backups[j])
	})
	ordered = append(ordered, backups...)

	if maxUpdatedAt == 0 {
		maxUpdatedAt = time.Now().UnixMilli()
	}

	resp := &CockpitQuotaResponse{
		CurrentAccount: currentAcc,
		Accounts:       ordered,
		UpdatedAt:      maxUpdatedAt,
	}

	quotaCacheMu.Lock()
	if len(quotaCacheByEmail) > 16 {
		quotaCacheByEmail = make(map[string]*quotaCacheEntry)
	}
	quotaCacheByEmail[emailKey] = &quotaCacheEntry{
		data:      resp,
		expiresAt: time.Now().Add(quotaCacheTTL),
	}
	quotaCacheMu.Unlock()

	return resp, nil
}

func triggerRefreshWithWaitChannel(isForce bool) (<-chan struct{}, error) {
	refreshMutex.Lock()
	if isRefreshing {
		ch := refreshDone
		refreshMutex.Unlock()
		return ch, nil
	}
	if !isForce && time.Since(lastRefreshAttempt) < 10*time.Second {
		refreshMutex.Unlock()
		return nil, nil
	}
	lastRefreshAttempt = time.Now()
	isRefreshing = true
	done := make(chan struct{})
	refreshDone = done
	refreshMutex.Unlock()

	finishRefresh := func() {
		refreshMutex.Lock()
		isRefreshing = false
		if refreshDone == done {
			refreshDone = nil
		}
		close(done)
		refreshMutex.Unlock()
	}

	cfg, err := getCockpitConfig()
	if err != nil {
		// Cockpit Tools config.json not found (e.g. headless Linux/NAS without Cockpit desktop app).
		// Fall directly to native Google Cloud Code API quota fetch.
		go func() {
			defer finishRefresh()
			slog.Info("[Cockpit] Cockpit config not available; triggering direct native quota fetch...")
			if fErr := FetchAllRemoteQuotas(); fErr != nil {
				slog.Warn("[Cockpit] Direct native quota fetch failed", "err", fErr)
			} else {
				InvalidateQuotaCache()
			}
		}()
		return done, nil
	}

	reportPort := cfg.ReportPort
	if cfg.ReportToken != "" {
		if resolved, rErr := ResolveActiveReportPort(cfg.ReportToken, cfg.ReportPort); rErr == nil && resolved > 0 {
			reportPort = resolved
		}
	}

	if reportPort > 0 && cfg.ReportToken != "" && IsCockpitListening(reportPort, 300*time.Millisecond) {
		go func(port int, token string) {
			defer finishRefresh()

			if err := QueryReport(port, token); err != nil {
				slog.Warn(fmt.Sprintf("[Cockpit] Report request error: %v; falling back to direct quota fetch", err))
				if fErr := FetchAllRemoteQuotas(); fErr == nil {
					InvalidateQuotaCache()
				}
			} else {
				InvalidateQuotaCache()
			}
		}(reportPort, cfg.ReportToken)
		return done, nil
	}

	// Native path: Cockpit Tools desktop process not running, query directly via Google Cloud Code API
	go func() {
		defer finishRefresh()
		slog.Info("[Cockpit] Cockpit Tools desktop process not listening; triggering direct native quota fetch...")
		if err := FetchAllRemoteQuotas(); err != nil {
			slog.Warn("[Cockpit] Direct native quota fetch failed", "err", err)
		} else {
			InvalidateQuotaCache()
		}
	}()
	return done, nil
}

// TriggerRefresh triggers a fresh quota fetch across all accounts in Cockpit Tools.
// Optional force parameter bypasses the attempt debounce if set to true.
func TriggerRefresh(force ...bool) error {
	isForce := len(force) > 0 && force[0]
	_, err := triggerRefreshWithWaitChannel(isForce)
	return err
}

func buildReportURL(port int, token string) string {
	q := url.Values{}
	if token != "" {
		q.Set("token", token)
	}
	q.Set("format", "yaml")
	return fmt.Sprintf("http://127.0.0.1:%d/report?%s", port, q.Encode())
}

func sanitizeReportError(err error, port int) error {
	if err == nil {
		return nil
	}
	var uErr *url.Error
	if errors.As(err, &uErr) {
		return fmt.Errorf("%s http://127.0.0.1:%d/report: %w", uErr.Op, port, uErr.Err)
	}
	return err
}

// QueryReport sends an HTTP GET request to the local Cockpit Tools report endpoint to trigger fresh quota collection.
func QueryReport(port int, token string) error {
	reqURL := buildReportURL(port, token)
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(reqURL)
	if err != nil {
		return sanitizeReportError(err, port)
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

// RefreshQuotas triggers a refresh and awaits completion via event channel (bounded at 10 seconds).
func RefreshQuotas(activeEmails ...string) (*CockpitQuotaResponse, error) {
	ResetAutoRefreshCooldown()
	doneCh, err := triggerRefreshWithWaitChannel(true)
	if err != nil {
		currentQuotas, _ := GetQuotas(activeEmails...)
		return currentQuotas, err
	}

	if doneCh != nil {
		timer := time.NewTimer(10 * time.Second)
		select {
		case <-doneCh:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		case <-timer.C:
		}
	}

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

	// Backup existing config with restricted 0600 permissions
	_ = writeFileAtomic0600(configFile+".bak", cfgBytes)

	if err := writeFileAtomic0600(configFile, append(updatedBytes, '\n')); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	slog.Info("[Cockpit] Auto-aligned config.json: antigravity_launch_on_switch=false", "antigravity_app_path", rawMap["antigravity_app_path"])

	// 5. If Cockpit Tools process is running, restart it so the in-memory config is reloaded
	if cockpitProcessChecker() {
		slog.Info("[Cockpit] Restarting Cockpit Tools to reload updated configuration into memory...")
		if rErr := RestartCockpitApp(); rErr != nil {
			slog.Warn("[Cockpit] Warning: failed to restart Cockpit Tools", "err", rErr)
		} else {
			time.Sleep(1 * time.Second)
		}
	}

	return nil
}

