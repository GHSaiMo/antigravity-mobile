package cockpit

import (
	"context"
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
	ReportPort  int    `json:"report_port"`
	ReportToken string `json:"report_token"`
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
	refreshMutex sync.Mutex
	lastRefresh  time.Time
)

// GetCockpitDataDir returns the path to ~/.antigravity_cockpit.
func GetCockpitDataDir() (string, error) {
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

// GetQuotas reads Cockpit Tools' local storage and cache to construct the full quota snapshot.
// An optional activeEmail (e.g. from live Language Server) can be passed to prioritize the true runtime account.
func GetQuotas(activeEmails ...string) (*CockpitQuotaResponse, error) {
	dataDir, err := GetCockpitDataDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get cockpit data dir: %w", err)
	}

	accountsFile := filepath.Join(dataDir, "accounts.json")
	accBytes, err := os.ReadFile(accountsFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read accounts.json: %w", err)
	}

	var idx accountsIndex
	if err := json.Unmarshal(accBytes, &idx); err != nil {
		return nil, fmt.Errorf("failed to parse accounts.json: %w", err)
	}

	resolvedCurrentID := ""

	// 1. Cockpit accounts.json current_account_id (Priority 1: User's explicitly chosen Cockpit active account)
	if strings.TrimSpace(idx.CurrentAccountID) != "" {
		for _, acc := range idx.Accounts {
			if acc.ID == strings.TrimSpace(idx.CurrentAccountID) {
				resolvedCurrentID = acc.ID
				break
			}
		}
	}

	// 2. Live Language Server active email (Priority 2 fallback)
	if resolvedCurrentID == "" && len(activeEmails) > 0 && strings.TrimSpace(activeEmails[0]) != "" {
		targetEmail := strings.ToLower(strings.TrimSpace(activeEmails[0]))
		for _, acc := range idx.Accounts {
			if strings.ToLower(strings.TrimSpace(acc.Email)) == targetEmail {
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
						case "gemini-5h":
							item.Gemini5h = makeMetric(b.RemainingFraction, b.ResetTime)
						case "gemini-weekly":
							item.GeminiWeekly = makeMetric(b.RemainingFraction, b.ResetTime)
						case "3p-5h":
							item.Claude5h = makeMetric(b.RemainingFraction, b.ResetTime)
						case "3p-weekly":
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

	return &CockpitQuotaResponse{
		CurrentAccount: currentAcc,
		Accounts:       ordered,
		UpdatedAt:      maxUpdatedAt,
	}, nil
}

// TriggerRefresh triggers a fresh quota fetch across all accounts in Cockpit Tools.
func TriggerRefresh() error {
	refreshMutex.Lock()
	defer refreshMutex.Unlock()

	// Debounce if called within 3 seconds
	if time.Since(lastRefresh) < 3*time.Second {
		return nil
	}
	lastRefresh = time.Now()

	dataDir, err := GetCockpitDataDir()
	if err != nil {
		return err
	}

	configFile := filepath.Join(dataDir, "config.json")
	cfgBytes, err := os.ReadFile(configFile)
	if err == nil {
		var cfg cockpitConfig
		if err := json.Unmarshal(cfgBytes, &cfg); err == nil && cfg.ReportPort > 0 && cfg.ReportToken != "" {
			go func(port int, token string) {
				url := fmt.Sprintf("http://127.0.0.1:%d/report?token=%s&format=yaml", port, token)
				client := &http.Client{Timeout: 60 * time.Second}
				resp, err := client.Get(url)
				if err == nil && resp != nil && resp.Body != nil {
					_, _ = io.Copy(io.Discard, resp.Body)
					_ = resp.Body.Close()
				}
			}(cfg.ReportPort, cfg.ReportToken)
			return nil
		}
	}

	// Fallback to macOS AppleScript click tray refresh
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		script := `tell application "System Events" to tell process "Cockpit Tools" to click menu item "🔄 刷新配额" of menu 1 of menu bar item 1 of menu bar 2`
		if err := exec.CommandContext(ctx, "osascript", "-e", script).Run(); err != nil {
			log.Printf("[Cockpit] AppleScript refresh fallback failed: %v", err)
		}
	}()

	return nil
}

// RefreshQuotas triggers a refresh and polls up to 10 seconds for updated cache data.
func RefreshQuotas(activeEmails ...string) (*CockpitQuotaResponse, error) {
	currentQuotas, _ := GetQuotas(activeEmails...)
	var initialUpdatedAt int64
	if currentQuotas != nil {
		initialUpdatedAt = currentQuotas.UpdatedAt
	}

	if err := TriggerRefresh(); err != nil {
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
