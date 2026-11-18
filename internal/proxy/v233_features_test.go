package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestWorkspaceTrustEndpoints(t *testing.T) {
	var trustedState atomic.Bool
	var lastSetPaths []string

	p := newFakeLSProxy(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/GetProjectTrustStatus"):
			_, _ = fmt.Fprintf(w, `{"status":{"isProjectTrusted":%v,"hasBlockedItems":%v,"folders":[{"folderUri":"file:///tmp/demo","path":"/tmp/demo","trusted":%v}]}}`,
				trustedState.Load(), !trustedState.Load(), trustedState.Load())
		case strings.HasSuffix(r.URL.Path, "/SetWorkspaceTrust"):
			var body struct {
				ProjectID string   `json:"projectId"`
				Paths     []string `json:"paths"`
				Trusted   bool     `json:"trusted"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			lastSetPaths = body.Paths
			trustedState.Store(body.Trusted)
			_, _ = fmt.Fprintf(w, `{"status":{"isProjectTrusted":%v,"hasBlockedItems":false,"folders":[{"folderUri":"file:///tmp/demo","path":"/tmp/demo","trusted":%v}]}}`,
				body.Trusted, body.Trusted)
		default:
			http.NotFound(w, r)
		}
	})

	// Pure chat / outside-of-project returns trusted=true without RPC error.
	recPure := httptest.NewRecorder()
	p.ServeHTTP(recPure, httptest.NewRequest(http.MethodGet, "/gateway/projects/trust?projectId=outside-of-project", nil))
	if recPure.Code != http.StatusOK || !strings.Contains(recPure.Body.String(), `"isProjectTrusted":true`) {
		t.Fatalf("expected outside-of-project trusted, got %d %s", recPure.Code, recPure.Body.String())
	}

	// Untrusted project check.
	recGet := httptest.NewRecorder()
	p.ServeHTTP(recGet, httptest.NewRequest(http.MethodGet, "/gateway/projects/trust?projectId=proj-1", nil))
	if recGet.Code != http.StatusOK || !strings.Contains(recGet.Body.String(), `"isProjectTrusted":false`) {
		t.Fatalf("expected untrusted status, got %d %s", recGet.Code, recGet.Body.String())
	}

	// Grant trust with auto-discovered paths.
	recPost := httptest.NewRecorder()
	reqPost := httptest.NewRequest(http.MethodPost, "/gateway/projects/trust", strings.NewReader(`{"projectId":"proj-1","trusted":true}`))
	p.ServeHTTP(recPost, reqPost)
	if recPost.Code != http.StatusOK || !strings.Contains(recPost.Body.String(), `"isProjectTrusted":true`) {
		t.Fatalf("expected trusted after POST, got %d %s", recPost.Code, recPost.Body.String())
	}
	if len(lastSetPaths) != 1 || lastSetPaths[0] != "/tmp/demo" {
		t.Fatalf("expected auto-discovered path /tmp/demo, got %v", lastSetPaths)
	}
}

func TestGitHubPullRequestsBatchEndpointAndURLParser(t *testing.T) {
	t.Setenv("MGY_DISABLE_GH_CLI", "1")
	ref, ok := ParseGitHubPRURL("https://github.com/octocat/hello-world/pull/42/files#diff-1")
	if !ok || ref.Host != "github.com" || ref.Owner != "octocat" || ref.Repo != "hello-world" || ref.Number != 42 {
		t.Fatalf("unexpected ParseGitHubPRURL result: %+v ok=%v", ref, ok)
	}

	var calls atomic.Int32
	// Clear ref cache before test
	gitPRRefCacheMu.Lock()
	gitPRRefCache = make(map[string]gitPRRefCacheEntry)
	gitPRRefCacheMu.Unlock()

	p := newFakeLSProxy(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/GetGitHubPullRequests") {
			calls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"statuses": [{
					"ref": {"host": "github.com", "owner": "octocat", "repo": "hello-world", "number": 42},
					"url": "https://github.com/octocat/hello-world/pull/42",
					"title": "Add awesome PR pills",
					"state": "GITHUB_PULL_REQUEST_STATE_OPEN",
					"authorLogin": "octocat",
					"headRefName": "feat/pr-pills",
					"baseRefName": "main",
					"body": "Implements PR pill status badges."
				}]
			}`))
			return
		}
		http.NotFound(w, r)
	})
	body := `{"urls":["https://github.com/octocat/hello-world/pull/42","https://github.com/octocat/hello-world/pull/42"]}`
	rec1 := httptest.NewRecorder()
	p.ServeHTTP(rec1, httptest.NewRequest(http.MethodPost, "/gateway/github/pull-requests", strings.NewReader(body)))
	if rec1.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec1.Code, rec1.Body.String())
	}
	var res struct {
		PullRequests []GitPullRequestItem `json:"pullRequests"`
	}
	if err := json.Unmarshal(rec1.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if len(res.PullRequests) != 1 || res.PullRequests[0].State != "OPEN" || res.PullRequests[0].Title != "Add awesome PR pills" {
		t.Fatalf("unexpected pullRequests: %+v", res.PullRequests)
	}

	// Second request hits cache.
	rec2 := httptest.NewRecorder()
	p.ServeHTTP(rec2, httptest.NewRequest(http.MethodPost, "/gateway/github/pull-requests", strings.NewReader(body)))
	if rec2.Code != http.StatusOK || calls.Load() != 1 {
		t.Fatalf("expected 1 upstream RPC call (cached on 2nd), got %d", calls.Load())
	}
}

func TestFastGitBranchAndCachedListPR(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/feat/v233\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "packages", "app")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	root, branch := findGitRepoRootFast(sub)
	if filepath.Clean(root) != filepath.Clean(dir) || branch != "feat/v233" {
		t.Fatalf("got root=%q branch=%q, want %q feat/v233", root, branch, dir)
	}

	gitPRCacheMu.Lock()
	gitPRCache[fmt.Sprintf("9999|%s|feat/v233", filepath.Clean(dir))] = gitPRCacheEntry{
		prs: []GitPullRequestItem{
			{Number: 12, State: "OPEN", HeadRefName: "feat/v233", Title: "V2.33 support"},
		},
		fetchedAt: time.Now(),
	}
	gitPRCacheMu.Unlock()

	gotBranch, pr, _, needsWarmup := cachedWorkspacePullRequest([]string{"file://" + sub}, 9999)
	if needsWarmup || gotBranch != "feat/v233" || pr == nil || pr.Number != 12 {
		t.Fatalf("expected cached PR #12 on feat/v233, got branch=%q pr=%+v needsWarmup=%v", gotBranch, pr, needsWarmup)
	}
}

func TestBtwSideQuestionLifecycleAndHiddenFromMainList(t *testing.T) {
	var startType, sentParentMsg string
	p := newFakeLSProxy(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/StartCascade"):
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			startType, _ = body["trajectoryType"].(string)
			_, _ = w.Write([]byte(`{"cascadeId":"side-q-1"}`))
		case strings.HasSuffix(r.URL.Path, "/SendUserCascadeMessage"):
			var body struct {
				Items []struct {
					Text string `json:"text"`
				} `json:"items"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if len(body.Items) > 0 {
				sentParentMsg = body.Items[0].Text
			}
			_, _ = w.Write([]byte(`{}`))
		case strings.HasSuffix(r.URL.Path, "/GetAllCascadeTrajectories"):
			_, _ = w.Write([]byte(`{
				"trajectorySummaries": {
					"main-conv": {"status":"CASCADE_RUN_STATUS_IDLE","stepCount":2,"summary":"Main Conv"},
					"side-q-1": {"status":"CASCADE_RUN_STATUS_IDLE","stepCount":2,"summary":"Quick Side Q","trajectoryType":"CORTEX_TRAJECTORY_TYPE_SIDE_QUESTION"}
				}
			}`))
		case strings.HasSuffix(r.URL.Path, "/GetCascadeTrajectory"):
			_, _ = w.Write([]byte(`{
				"trajectory": {
					"cascadeId": "side-q-1",
					"steps": [
						{"type":"CORTEX_STEP_TYPE_USER_INPUT","userInput":{"items":[{"text":"What is Nord?"}]}},
						{"type":"CORTEX_STEP_TYPE_PLANNER_RESPONSE","plannerResponse":{"response":"Nord is an arctic dark palette."}}
					]
				},
				"status": "CASCADE_RUN_STATUS_IDLE"
			}`))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	})
	startRec := httptest.NewRecorder()
	p.ServeHTTP(startRec, httptest.NewRequest(http.MethodPost, "/gateway/btw/start", strings.NewReader(`{
		"parentCascadeId": "main-conv",
		"question": "What is Nord?",
		"quote": "#2E3440"
	}`)))
	if startRec.Code != http.StatusOK || startType != "CORTEX_TRAJECTORY_TYPE_SIDE_QUESTION" {
		t.Fatalf("btw/start failed: code=%d startType=%q body=%s", startRec.Code, startType, startRec.Body.String())
	}
	if !strings.Contains(sentParentMsg, "> #2E3440") || !strings.Contains(sentParentMsg, "What is Nord?") {
		t.Fatalf("unexpected formatted btw prompt: %q", sentParentMsg)
	}

	// Verify status endpoint returns answer + formatted insertText.
	statusRec := httptest.NewRecorder()
	p.ServeHTTP(statusRec, httptest.NewRequest(http.MethodGet, "/gateway/btw/status?cascadeId=side-q-1", nil))
	if statusRec.Code != http.StatusOK || !strings.Contains(statusRec.Body.String(), "Nord is an arctic dark palette.") || !strings.Contains(statusRec.Body.String(), "insertText") {
		t.Fatalf("btw/status unexpected: code=%d body=%s", statusRec.Code, statusRec.Body.String())
	}

	// Verify list endpoint returns tracked side question.
	listRec := httptest.NewRecorder()
	p.ServeHTTP(listRec, httptest.NewRequest(http.MethodGet, "/gateway/btw/list?cascadeId=main-conv", nil))
	if listRec.Code != http.StatusOK || !strings.Contains(listRec.Body.String(), "side-q-1") {
		t.Fatalf("btw/list unexpected: code=%d body=%s", listRec.Code, listRec.Body.String())
	}

	// Verify GetAllCascadeTrajectories filters out side-q-1 from main list.
	allRec := httptest.NewRecorder()
	allReq := httptest.NewRequest(http.MethodPost, "/exa.language_server_pb.LanguageServerService/GetAllCascadeTrajectories", strings.NewReader(`{}`))
	p.ServeHTTP(allRec, allReq)
	if allRec.Code != http.StatusOK || !strings.Contains(allRec.Body.String(), "main-conv") || strings.Contains(allRec.Body.String(), "side-q-1") {
		t.Fatalf("expected main-conv only and side-q-1 hidden, got: %s", allRec.Body.String())
	}
}

func TestLargeMedia200MBLimitsAndAMR(t *testing.T) {
	setupInbox(t)
	// Without explicit MGY_MAX_UPLOAD_MB override, audio/video allows 200 MB while docs stay 50 MB.
	t.Setenv("MGY_MAX_UPLOAD_MB", "")
	if got := maxUploadBytesForKind("audio"); got != 200*1024*1024 {
		t.Fatalf("audio maxUploadBytes = %d, want 200MB", got)
	}
	if got := maxUploadBytesForKind("video"); got != 200*1024*1024 {
		t.Fatalf("video maxUploadBytes = %d, want 200MB", got)
	}
	if got := maxUploadBytesForKind("document"); got != 50*1024*1024 {
		t.Fatalf("document maxUploadBytes = %d, want 50MB", got)
	}

	if !supportsNativeAttachment(Attachment{Kind: "audio", Size: 180 * 1024 * 1024}) {
		t.Fatalf("expected 180MB audio to support native media attachment")
	}
	if supportsNativeAttachment(Attachment{Kind: "document", Size: 25 * 1024 * 1024}) {
		t.Fatalf("expected 25MB document to exceed 20MB native non-media cap")
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/attachments", bytes.NewReader([]byte("#!AMR\nhello")))
	req.Header.Set("X-File-Name", url.PathEscape("voice-note.amr"))
	rec := httptest.NewRecorder()
	(&Proxy{}).HandleAttachmentUpload(rec, req)
	att := decodeAttachment(t, rec)
	if att.Kind != "audio" || att.Mime != "audio/amr" {
		t.Fatalf("expected audio/amr for .amr, got kind=%q mime=%q", att.Kind, att.Mime)
	}
}
