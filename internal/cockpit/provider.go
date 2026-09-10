package cockpit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
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

// formatResetFriendly converts an ISO timestamp to "HH:MM (约XhYm后)" or "HH:MM (已就绪)".
func formatResetFriendly(isoStr string) string {
	if strings.TrimSpace(isoStr) == "" {
		return "未知"
	}
	t, err := time.Parse(time.RFC3339, isoStr)
	if err != nil {
		return isoStr
	}
	local := t.Local()
	diff := time.Until(local)
	if diff > 0 {
		h := int(diff.Hours())
		m := int(diff.Minutes()) % 60
		if h > 0 {
			return fmt.Sprintf("%02d:%02d (约%dh%02dm后)", local.Hour(), local.Minute(), h, m)
		}
		return fmt.Sprintf("%02d:%02d (约%dm后)", local.Hour(), local.Minute(), m)
	}
	return fmt.Sprintf("%02d:%02d (已就绪)", local.Hour(), local.Minute())
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
func GetQuotas() (*CockpitQuotaResponse, error) {
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
			IsCurrent: acc.ID == idx.CurrentAccountID,
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

	// Debounce if called within 5 seconds
	if time.Since(lastRefresh) < 5*time.Second {
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
				_, _ = client.Get(url)
			}(cfg.ReportPort, cfg.ReportToken)
			return nil
		}
	}

	// Fallback to macOS AppleScript click tray refresh
	go func() {
		script := `tell application "System Events" to tell process "Cockpit Tools" to click menu item "🔄 刷新配额" of menu 1 of menu bar item 1 of menu bar 2`
		_ = exec.Command("osascript", "-e", script).Run()
	}()

	return nil
}
