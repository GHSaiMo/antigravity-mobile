package cockpit

import (
	"log/slog"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	CloudCodeDailyBaseURL = "https://daily-cloudcode-pa.googleapis.com"
	CloudCodeUserAgent    = "antigravity/1.20.5 darwin/arm64"
)

// FetchRemoteQuotaForAccount queries Google's Cloud Code APIs directly using the account's access token
// and saves the resulting quota snapshot to Cockpit Tools' cache directory.
func FetchRemoteQuotaForAccount(detail *CockpitAccountDetail) error {
	if detail == nil || strings.TrimSpace(detail.Email) == "" {
		return fmt.Errorf("invalid account detail")
	}

	freshTok, refreshed, err := EnsureFreshToken(&detail.Token)
	if err != nil {
		return fmt.Errorf("ensure fresh token for %s: %w", detail.Email, err)
	}
	if refreshed {
		detail.Token = *freshTok
		_ = SaveAccountDetail(detail)
	}

	client := &http.Client{Timeout: 15 * time.Second}

	// 1. Fetch retrieveUserQuotaSummary
	summaryURL := fmt.Sprintf("%s/v1internal:retrieveUserQuotaSummary", CloudCodeDailyBaseURL)
	summaryReq, err := http.NewRequest(http.MethodPost, summaryURL, strings.NewReader("{}"))
	if err != nil {
		return err
	}
	summaryReq.Header.Set("Authorization", "Bearer "+detail.Token.AccessToken)
	summaryReq.Header.Set("Content-Type", "application/json")
	summaryReq.Header.Set("User-Agent", CloudCodeUserAgent)

	summaryResp, err := client.Do(summaryReq)
	if err != nil {
		return fmt.Errorf("retrieveUserQuotaSummary error: %w", err)
	}
	defer summaryResp.Body.Close()

	summaryBody, _ := io.ReadAll(io.LimitReader(summaryResp.Body, 1<<20))
	if summaryResp.StatusCode != http.StatusOK {
		return fmt.Errorf("retrieveUserQuotaSummary returned HTTP %d: %s", summaryResp.StatusCode, strings.TrimSpace(string(summaryBody)))
	}

	var summaryData struct {
		Groups []struct {
			Buckets []struct {
				BucketID          string   `json:"bucketId"`
				DisplayName       string   `json:"displayName"`
				RemainingFraction *float64 `json:"remainingFraction"`
				ResetTime         string   `json:"resetTime"`
			} `json:"buckets"`
		} `json:"groups"`
	}
	_ = json.Unmarshal(summaryBody, &summaryData)

	// 2. Fetch fetchAvailableModels
	modelsURL := fmt.Sprintf("%s/v1internal:fetchAvailableModels", CloudCodeDailyBaseURL)
	modelsReq, err := http.NewRequest(http.MethodPost, modelsURL, strings.NewReader("{}"))
	if err == nil {
		modelsReq.Header.Set("Authorization", "Bearer "+detail.Token.AccessToken)
		modelsReq.Header.Set("Content-Type", "application/json")
		modelsReq.Header.Set("User-Agent", CloudCodeUserAgent)
	}
	modelsResp, err := client.Do(modelsReq)
	var modelsMap map[string]struct {
		DisplayName string `json:"displayName"`
		QuotaInfo   struct {
			RemainingFraction *float64 `json:"remainingFraction"`
			ResetTime         string   `json:"resetTime"`
		} `json:"quotaInfo"`
	}
	if err == nil && modelsResp.StatusCode == http.StatusOK {
		defer modelsResp.Body.Close()
		modelsBody, _ := io.ReadAll(io.LimitReader(modelsResp.Body, 2<<20))
		var mData struct {
			Models map[string]struct {
				DisplayName string `json:"displayName"`
				QuotaInfo   struct {
					RemainingFraction *float64 `json:"remainingFraction"`
					ResetTime         string   `json:"resetTime"`
				} `json:"quotaInfo"`
			} `json:"models"`
		}
		if json.Unmarshal(modelsBody, &mData) == nil {
			modelsMap = mData.Models
		}
	}

	// 3. Assemble and save cachePayload
	var cp cachePayload
	cp.UpdatedAt = time.Now().UnixMilli()
	cp.Payload.Models = modelsMap
	cp.Payload.QuotaSummary.Groups = summaryData.Groups

	dataDir, err := GetCockpitDataDir()
	if err != nil {
		return err
	}
	cacheDir := filepath.Join(dataDir, "cache", "quota_api_v1_desktop", "authorized")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return fmt.Errorf("create cache dir: %w", err)
	}

	emailNorm := strings.ToLower(strings.TrimSpace(detail.Email))
	h := sha256.Sum256([]byte(emailNorm))
	hashHex := hex.EncodeToString(h[:])
	cacheFilePath := filepath.Join(cacheDir, hashHex+".json")

	cpBytes, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal cachePayload: %w", err)
	}

	tmpFile := cacheFilePath + ".tmp"
	if err := os.WriteFile(tmpFile, append(cpBytes, '\n'), 0644); err != nil {
		return fmt.Errorf("write tmp cache file: %w", err)
	}
	if err := os.Rename(tmpFile, cacheFilePath); err != nil {
		return fmt.Errorf("rename cache file: %w", err)
	}

	slog.Info(fmt.Sprintf("[Cockpit] Direct remote quota updated for %s", detail.Email))
	return nil
}

// FetchAllRemoteQuotas queries Google APIs for all accounts in storage concurrently.
func FetchAllRemoteQuotas() error {
	accounts, err := ListAccountsFromStorage()
	if err != nil || len(accounts) == 0 {
		return fmt.Errorf("no accounts available in storage: %v", err)
	}

	var wg sync.WaitGroup
	for _, acc := range accounts {
		if acc.Disabled || acc.Token.RefreshToken == "" {
			continue
		}
		wg.Add(1)
		go func(a *CockpitAccountDetail) {
			defer wg.Done()
			if err := FetchRemoteQuotaForAccount(a); err != nil {
				slog.Warn(fmt.Sprintf("[Cockpit] Direct remote quota error for %s", a.Email), "err", err)
			}
		}(acc)
	}
	wg.Wait()
	InvalidateQuotaCache()
	return nil
}
