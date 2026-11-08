package cockpit

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// CockpitInstallHint is shown to clients when Cockpit Tools is not configured.
const CockpitInstallHint = "请安装 Cockpit Tools,并在网关里用 mgy cockpit 进行配置,即可享受备用账号切换等功能"

const lsServicePath = "/exa.language_server_pb.LanguageServerService/"

// lsCall POSTs an empty Connect-protocol request to the Antigravity language
// server and returns the raw body. baseURL is like "https://127.0.0.1:1234".
func lsCall(client *http.Client, baseURL, token, method string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodPost, baseURL+lsServicePath+method, strings.NewReader("{}"))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connect-Protocol-Version", "1")
	if token != "" {
		req.Header.Set("x-codeium-csrf-token", token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s status %d", method, resp.StatusCode)
	}
	return body, nil
}

// FetchLiveQuota reads the current account and its 4 quota buckets directly
// from the running Antigravity language server (no Cockpit required).
func FetchLiveQuota(client *http.Client, baseURL, token string) (*AccountQuota, error) {
	statusBody, err := lsCall(client, baseURL, token, "GetUserStatus")
	if err != nil {
		return nil, err
	}
	var us struct {
		UserStatus struct {
			Name     string `json:"name"`
			Email    string `json:"email"`
			UserTier struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"userTier"`
			PlanStatus struct {
				PlanInfo struct {
					PlanName string `json:"planName"`
				} `json:"planInfo"`
			} `json:"planStatus"`
			CascadeModelConfigData struct {
				ClientModelConfigs []struct {
					Label     string `json:"label"`
					ModelID   string `json:"modelId"`
					QuotaInfo struct {
						RemainingFraction *float64 `json:"remainingFraction"`
						ResetTime         string   `json:"resetTime"`
					} `json:"quotaInfo"`
				} `json:"clientModelConfigs"`
			} `json:"cascadeModelConfigData"`
		} `json:"userStatus"`
	}
	if err := json.Unmarshal(statusBody, &us); err != nil {
		return nil, fmt.Errorf("parse GetUserStatus: %w", err)
	}
	email := strings.TrimSpace(us.UserStatus.Email)
	if email == "" {
		return nil, fmt.Errorf("language server has no signed-in user")
	}

	// userTier is authoritative (planName may lag, e.g. Ultra reported as Pro).
	tier := us.UserStatus.UserTier.Name
	if tier == "" {
		tier = us.UserStatus.UserTier.ID
	}
	if tier == "" {
		tier = us.UserStatus.PlanStatus.PlanInfo.PlanName
	}

	acc := &AccountQuota{
		ID:               "live:" + strings.ToLower(email),
		Email:            email,
		Name:             strings.TrimSpace(us.UserStatus.Name),
		SubscriptionTier: normalizeSubscriptionTier(tier),
		IsCurrent:        true,
		UpdatedAt:        time.Now().UnixMilli(),
	}

	// Argon comes from model-level quotaInfo.
	for _, m := range us.UserStatus.CascadeModelConfigData.ClientModelConfigs {
		if (isArgonModelOrBucket(m.ModelID) || isArgonModelOrBucket(m.Label)) && m.QuotaInfo.RemainingFraction != nil {
			acc.Gemini4Argon = makeMetric(m.QuotaInfo.RemainingFraction, m.QuotaInfo.ResetTime)
			break
		}
	}

	// 4 base buckets from RetrieveUserQuotaSummary. Failure here is not fatal:
	// we still return the account, with 5h values derived from model quotas.
	sumBody, sumErr := lsCall(client, baseURL, token, "RetrieveUserQuotaSummary")
	if sumErr == nil {
		var qs struct {
			Response struct {
				Groups []struct {
					Buckets []struct {
						BucketID          string   `json:"bucketId"`
						RemainingFraction *float64 `json:"remainingFraction"`
						ResetTime         string   `json:"resetTime"`
					} `json:"buckets"`
				} `json:"groups"`
			} `json:"response"`
		}
		if json.Unmarshal(sumBody, &qs) == nil {
			for _, g := range qs.Response.Groups {
				for _, b := range g.Buckets {
					switch b.BucketID {
					case "gemini-5h", "gemini_5h":
						acc.Gemini5h = makeMetric(b.RemainingFraction, b.ResetTime)
					case "gemini-weekly", "gemini_weekly":
						acc.GeminiWeekly = makeMetric(b.RemainingFraction, b.ResetTime)
					case "3p-5h", "claude-5h", "claude_5h":
						acc.Claude5h = makeMetric(b.RemainingFraction, b.ResetTime)
					case "3p-weekly", "claude-weekly", "claude_weekly":
						acc.ClaudeWeekly = makeMetric(b.RemainingFraction, b.ResetTime)
					}
				}
			}
		}
	}
	if acc.Gemini5h == nil || acc.Claude5h == nil {
		for _, m := range us.UserStatus.CascadeModelConfigData.ClientModelConfigs {
			if m.QuotaInfo.RemainingFraction == nil {
				continue
			}
			l := strings.ToLower(m.Label)
			switch {
			case acc.Claude5h == nil && (strings.Contains(l, "claude") || strings.Contains(l, "gpt")):
				acc.Claude5h = makeMetric(m.QuotaInfo.RemainingFraction, m.QuotaInfo.ResetTime)
			case acc.Gemini5h == nil && strings.Contains(l, "gemini") && !isArgonModelOrBucket(l):
				acc.Gemini5h = makeMetric(m.QuotaInfo.RemainingFraction, m.QuotaInfo.ResetTime)
			}
		}
	}
	return acc, nil
}

// LiveQuotaClient returns the shared local-TLS HTTP client for language server calls.
func LiveQuotaClient() *http.Client { return liveUserHTTPClient }

// IsCockpitInstalled reports whether Cockpit Tools data exists (accounts.json present).
func IsCockpitInstalled() bool {
	dir, err := GetCockpitDataDir()
	if err != nil {
		return false
	}
	_, statErr := os.Stat(filepath.Join(dir, "accounts.json"))
	return statErr == nil
}

// BuildQuotaResponse assembles the /cockpit/quotas payload. When Cockpit is
// installed, its data is used and the current account is overlaid with live
// language-server values (fresher). Otherwise the live account alone is returned.
// live may be nil if the language server is unreachable.
func BuildQuotaResponse(live *AccountQuota, liveEmail string) (*CockpitQuotaResponse, error) {
	installed := IsCockpitInstalled()
	if installed {
		resp, err := GetQuotas(liveEmail)
		if err != nil {
			return nil, err
		}
		out := *resp
		out.Accounts = append([]AccountQuota(nil), resp.Accounts...) // don't mutate the shared cache
		out.Source = "cockpit"
		out.CockpitInstalled = true
		if live != nil && out.CurrentAccount != nil &&
			strings.EqualFold(out.CurrentAccount.Email, live.Email) {
			merged := *out.CurrentAccount
			if live.Claude5h != nil {
				merged.Claude5h = live.Claude5h
			}
			if live.ClaudeWeekly != nil {
				merged.ClaudeWeekly = live.ClaudeWeekly
			}
			if live.Gemini5h != nil {
				merged.Gemini5h = live.Gemini5h
			}
			if live.GeminiWeekly != nil {
				merged.GeminiWeekly = live.GeminiWeekly
			}
			if live.Gemini4Argon != nil {
				merged.Gemini4Argon = live.Gemini4Argon
			}
			merged.UpdatedAt = live.UpdatedAt
			out.CurrentAccount = &merged
			for i := range out.Accounts {
				if out.Accounts[i].ID == merged.ID {
					out.Accounts[i] = merged
				}
			}
		}
		return &out, nil
	}
	return &CockpitQuotaResponse{
		CurrentAccount:   live,
		Accounts:         []AccountQuota{},
		UpdatedAt:        time.Now().UnixMilli(),
		Source:           "live",
		CockpitInstalled: false,
		CockpitHint:      CockpitInstallHint,
	}, nil
}
