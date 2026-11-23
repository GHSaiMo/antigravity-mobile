package cockpit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	CloudCodeDailyBaseURL = "https://daily-cloudcode-pa.googleapis.com"
	CloudCodeUserAgent    = "antigravity/2.25.0 (aidev_client; os_type=windows; arch=amd64; cl=1003475615)"
)

var (
	localProxyCacheMu     sync.Mutex
	localProxyCachedURL   *url.URL
	localProxyCachedUntil time.Time
)

func cockpitTransportProxy(req *http.Request) (*url.URL, error) {
	if req != nil && req.URL != nil {
		h := strings.ToLower(strings.TrimSpace(req.URL.Hostname()))
		if h == "127.0.0.1" || h == "localhost" || h == "::1" {
			return nil, nil
		}
	}
	if envURL, err := http.ProxyFromEnvironment(req); err != nil || envURL != nil {
		return envURL, err
	}
	if cfg, err := getCockpitConfig(); err == nil && cfg != nil && cfg.GlobalProxyEnabled {
		p := strings.TrimSpace(cfg.GlobalProxyURL)
		if isSafeProxyURL(p) {
			if u, uErr := url.Parse(p); uErr == nil && u.Host != "" {
				return u, nil
			}
		}
	}
	now := time.Now()
	localProxyCacheMu.Lock()
	if now.Before(localProxyCachedUntil) {
		u := localProxyCachedURL
		localProxyCacheMu.Unlock()
		return u, nil
	}
	localProxyCacheMu.Unlock()

	var detected *url.URL
	if IsProxyPortListening(7890, 150*time.Millisecond) {
		detected, _ = url.Parse("http://127.0.0.1:7890")
	}

	localProxyCacheMu.Lock()
	localProxyCachedURL = detected
	localProxyCachedUntil = now.Add(30 * time.Second)
	localProxyCacheMu.Unlock()
	return detected, nil
}

var cockpitHTTPClient = &http.Client{
	Timeout: 15 * time.Second,
	Transport: &http.Transport{
		Proxy:               cockpitTransportProxy,
		MaxIdleConns:        32,
		MaxIdleConnsPerHost: 16,
		IdleConnTimeout:     90 * time.Second,
	},
}

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

	client := cockpitHTTPClient

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
		Groups []quotaSummaryGroup `json:"groups"`
	}
	_ = json.Unmarshal(summaryBody, &summaryData)

	// 2. Fetch fetchAvailableModels
	modelsURL := fmt.Sprintf("%s/v1internal:fetchAvailableModels", CloudCodeDailyBaseURL)
	var modelsMap map[string]struct {
		DisplayName string `json:"displayName"`
		QuotaInfo   struct {
			RemainingFraction *float64 `json:"remainingFraction"`
			ResetTime         string   `json:"resetTime"`
		} `json:"quotaInfo"`
	}
	modelsReq, err := http.NewRequest(http.MethodPost, modelsURL, strings.NewReader("{}"))
	if err == nil {
		modelsReq.Header.Set("Authorization", "Bearer "+detail.Token.AccessToken)
		modelsReq.Header.Set("Content-Type", "application/json")
		modelsReq.Header.Set("User-Agent", CloudCodeUserAgent)
		modelsResp, doErr := client.Do(modelsReq)
		if doErr == nil && modelsResp != nil {
			modelsBody, _ := io.ReadAll(io.LimitReader(modelsResp.Body, 2<<20))
			_ = modelsResp.Body.Close()
			if modelsResp.StatusCode == http.StatusOK {
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
	if err := os.MkdirAll(cacheDir, 0700); err != nil {
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

	if err := writeFileAtomic0600(cacheFilePath, append(cpBytes, '\n')); err != nil {
		return fmt.Errorf("write cache file: %w", err)
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

	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for _, acc := range accounts {
		if acc.Disabled || acc.Token.RefreshToken == "" {
			continue
		}
		wg.Add(1)
		go func(a *CockpitAccountDetail) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if err := FetchRemoteQuotaForAccount(a); err != nil {
				slog.Warn(fmt.Sprintf("[Cockpit] Direct remote quota error for %s", a.Email), "err", err)
			}
		}(acc)
	}
	wg.Wait()
	InvalidateQuotaCache()
	return nil
}
