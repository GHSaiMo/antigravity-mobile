package cockpit

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const fakeUserStatus = `{"userStatus":{"name":"Seven","email":"a@b.com",
"planStatus":{"planInfo":{"planName":"Pro"}},
"userTier":{"id":"g1-ultra-tier","name":"Google AI Ultra"},
"cascadeModelConfigData":{"clientModelConfigs":[
 {"label":"Gemini 4 Argon (Medium)","modelId":"gemini-4-argon-medium","quotaInfo":{"remainingFraction":0.5,"resetTime":"2030-01-01T00:00:00Z"}},
 {"label":"Claude Opus 5.5 (Low)","quotaInfo":{"remainingFraction":0.9,"resetTime":"2030-01-01T00:00:00Z"}}]}}}`

const fakeSummary = `{"response":{"groups":[
 {"buckets":[{"bucketId":"gemini-weekly","remainingFraction":0.66,"resetTime":"2030-01-02T00:00:00Z"},{"bucketId":"gemini-5h","remainingFraction":0.57,"resetTime":"2030-01-01T00:00:00Z"}]},
 {"buckets":[{"bucketId":"3p-weekly","remainingFraction":0.75,"resetTime":"2030-01-02T00:00:00Z"},{"bucketId":"3p-5h","remainingFraction":0.97,"resetTime":"2030-01-01T00:00:00Z"}]}]}}`

func fakeLS(t *testing.T, summaryStatus int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-codeium-csrf-token") != "tok" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/GetUserStatus"):
			_, _ = w.Write([]byte(fakeUserStatus))
		case strings.HasSuffix(r.URL.Path, "/RetrieveUserQuotaSummary"):
			if summaryStatus != 200 {
				w.WriteHeader(summaryStatus)
				return
			}
			_, _ = w.Write([]byte(fakeSummary))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestFetchLiveQuotaMapsFourBuckets(t *testing.T) {
	srv := fakeLS(t, 200)
	defer srv.Close()
	acc, err := FetchLiveQuota(srv.Client(), srv.URL, "tok")
	if err != nil {
		t.Fatal(err)
	}
	if acc.Email != "a@b.com" || acc.SubscriptionTier != "ULTRA" || !acc.IsCurrent {
		t.Fatalf("bad identity: %+v", acc)
	}
	if acc.Claude5h.RemainingPercent != 97 || acc.ClaudeWeekly.RemainingPercent != 75 ||
		acc.Gemini5h.RemainingPercent != 57 || acc.GeminiWeekly.RemainingPercent != 66 {
		t.Fatalf("bad buckets: %+v %+v %+v %+v", acc.Claude5h, acc.ClaudeWeekly, acc.Gemini5h, acc.GeminiWeekly)
	}
	if acc.Gemini4Argon == nil || acc.Gemini4Argon.RemainingPercent != 50 {
		t.Fatalf("bad argon: %+v", acc.Gemini4Argon)
	}
}

func TestFetchLiveQuotaSummaryFailureFallsBackToModels(t *testing.T) {
	srv := fakeLS(t, 500)
	defer srv.Close()
	acc, err := FetchLiveQuota(srv.Client(), srv.URL, "tok")
	if err != nil {
		t.Fatal(err)
	}
	if acc.Claude5h == nil || acc.Claude5h.RemainingPercent != 90 {
		t.Fatalf("claude 5h fallback missing: %+v", acc.Claude5h)
	}
	if acc.ClaudeWeekly != nil || acc.GeminiWeekly != nil {
		t.Fatal("weekly should be nil without summary")
	}
}

func TestFetchLiveQuotaUnreachableOrUnauthorized(t *testing.T) {
	srv := fakeLS(t, 200)
	defer srv.Close()
	if _, err := FetchLiveQuota(srv.Client(), srv.URL, "wrong"); err == nil {
		t.Fatal("expected error on bad csrf")
	}
	srv.Close()
	if _, err := FetchLiveQuota(http.DefaultClient, srv.URL, "tok"); err == nil {
		t.Fatal("expected error when LS down")
	}
}

func TestBuildQuotaResponseWithoutCockpit(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("COCKPIT_TOOLS_DATA_DIR", t.TempDir())
	live := &AccountQuota{Email: "a@b.com", IsCurrent: true}
	resp, err := BuildQuotaResponse(live, "a@b.com")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Source != "live" || resp.CockpitInstalled || resp.CockpitHint == "" ||
		resp.CurrentAccount == nil || len(resp.Accounts) != 0 {
		t.Fatalf("unexpected: %+v", resp)
	}
}
