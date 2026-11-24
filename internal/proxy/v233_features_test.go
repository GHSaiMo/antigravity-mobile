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
	var startBase map[string]interface{}
	var replyCount atomic.Int32
	var canceledID string
	p := newFakeLSProxy(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/StartCascade"):
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			startType, _ = body["trajectoryType"].(string)
			startBase, _ = body["baseTrajectoryIdentifier"].(map[string]interface{})
			_, _ = w.Write([]byte(`{"cascadeId":"side-q-1"}`))
		case strings.HasSuffix(r.URL.Path, "/SendUserCascadeMessage"):
			var body struct {
				CascadeID string `json:"cascadeId"`
				Items     []struct {
					Text string `json:"text"`
				} `json:"items"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if len(body.Items) > 0 && sentParentMsg == "" {
				sentParentMsg = body.Items[0].Text
			}
			replyCount.Add(1)
			_, _ = w.Write([]byte(`{}`))
		case strings.HasSuffix(r.URL.Path, "/CancelCascadeInvocation"):
			var body struct {
				CascadeID string `json:"cascadeId"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			canceledID = body.CascadeID
			_, _ = w.Write([]byte(`{}`))
		case strings.HasSuffix(r.URL.Path, "/GetAllCascadeTrajectories"):
			_, _ = w.Write([]byte(`{
				"trajectorySummaries": {
					"main-conv": {
						"status":"CASCADE_RUN_STATUS_IDLE",
						"stepCount":4,
						"summary":"Main Conv",
						"annotations":{
							"title":"Main Conv",
							"sideQuestionCascadeIds":["side-q-1"],
							"sideQuestionTitles":{"side-q-1":"Nord Palette Meaning"}
						}
					},
					"side-q-1": {
						"status":"CASCADE_RUN_STATUS_IDLE",
						"stepCount":7,
						"summary":"Quick Side Q",
						"trajectoryType":"CORTEX_TRAJECTORY_TYPE_SIDE_QUESTION"
					}
				}
			}`))
		case strings.HasSuffix(r.URL.Path, "/GetCascadeTrajectory"):
			var body struct {
				CascadeID string `json:"cascadeId"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.CascadeID == "main-conv" {
				_, _ = w.Write([]byte(`{
					"trajectory": {
						"cascadeId": "main-conv",
						"trajectoryType": "CORTEX_TRAJECTORY_TYPE_CASCADE",
						"steps": [
							{"type":"CORTEX_STEP_TYPE_USER_INPUT","userInput":{"items":[{"text":"Parent Turn 1"}]}},
							{"type":"CORTEX_STEP_TYPE_PLANNER_RESPONSE","plannerResponse":{"response":"PARENT_SECRET_HISTORY_1"}},
							{"type":"CORTEX_STEP_TYPE_USER_INPUT","userInput":{"items":[{"text":"Parent Turn 2"}]}},
							{"type":"CORTEX_STEP_TYPE_PLANNER_RESPONSE","plannerResponse":{"response":"PARENT_SECRET_HISTORY_2"}}
						]
					},
					"status": "CASCADE_RUN_STATUS_IDLE"
				}`))
				return
			}
			_, _ = w.Write([]byte(`{
				"trajectory": {
					"cascadeId": "side-q-1",
					"trajectoryType": "CORTEX_TRAJECTORY_TYPE_SIDE_QUESTION",
					"parentReferences": [{
						"conversationId": "main-conv",
						"stepIndex": 1,
						"referenceType": "CORTEX_TRAJECTORY_REFERENCE_TYPE_FORK",
						"trajectoryType": "CORTEX_TRAJECTORY_TYPE_CASCADE"
					}],
					"steps": [
						{"type":"CORTEX_STEP_TYPE_USER_INPUT","userInput":{"items":[{"text":"Parent Turn 1"}]}},
						{"type":"CORTEX_STEP_TYPE_PLANNER_RESPONSE","plannerResponse":{"response":"PARENT_SECRET_HISTORY_1"}},
						{"type":"CORTEX_STEP_TYPE_SYSTEM_MESSAGE","systemMessage":{"message":"Above is the conversation history from the parent conversation; what follows is a side question (/btw) branched from that conversation."}},
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
	defer defaultBtwTracker.remove("side-q-1")

	// Start side question anchored at stepIndex=1 via forkFrom.
	startRec := httptest.NewRecorder()
	p.ServeHTTP(startRec, httptest.NewRequest(http.MethodPost, "/gateway/btw/start", strings.NewReader(`{
		"parentCascadeId": "main-conv",
		"stepIndex": 1,
		"question": "What is Nord?",
		"quote": "#2E3440"
	}`)))
	if startRec.Code != http.StatusOK || startType != "CORTEX_TRAJECTORY_TYPE_SIDE_QUESTION" {
		t.Fatalf("btw/start failed: code=%d startType=%q body=%s", startRec.Code, startType, startRec.Body.String())
	}
	forkFrom, _ := startBase["forkFrom"].(map[string]interface{})
	if forkFrom == nil || forkFrom["cascadeId"] != "main-conv" || forkFrom["stepIndex"] != float64(1) || forkFrom["referenceType"] != "CORTEX_TRAJECTORY_REFERENCE_TYPE_FORK" {
		t.Fatalf("expected baseTrajectoryIdentifier.forkFrom(main-conv, 1, FORK), got: %+v", startBase)
	}
	if !strings.Contains(sentParentMsg, "> #2E3440") || !strings.Contains(sentParentMsg, "What is Nord?") {
		t.Fatalf("unexpected formatted btw prompt: %q", sentParentMsg)
	}

	// Verify status endpoint returns ONLY branch messages after forkStepIndex=1 (never parent history!).
	statusRec := httptest.NewRecorder()
	p.ServeHTTP(statusRec, httptest.NewRequest(http.MethodGet, "/gateway/btw/status?cascadeId=side-q-1", nil))
	statusBody := statusRec.Body.String()
	if statusRec.Code != http.StatusOK || !strings.Contains(statusBody, "Nord is an arctic dark palette.") || !strings.Contains(statusBody, "insertText") {
		t.Fatalf("btw/status unexpected: code=%d body=%s", statusRec.Code, statusBody)
	}
	if strings.Contains(statusBody, "PARENT_SECRET_HISTORY_1") || strings.Contains(statusBody, "Parent Turn 1") {
		t.Fatalf("btw/status leaked shared parent history: %s", statusBody)
	}

	// Verify /gateway/btw/reply and /gateway/btw/cancel.
	replyRec := httptest.NewRecorder()
	p.ServeHTTP(replyRec, httptest.NewRequest(http.MethodPost, "/gateway/btw/reply", strings.NewReader(`{"cascadeId":"side-q-1","message":"Follow-up?"}`)))
	if replyRec.Code != http.StatusOK || replyCount.Load() < 2 {
		t.Fatalf("btw/reply failed: code=%d count=%d body=%s", replyRec.Code, replyCount.Load(), replyRec.Body.String())
	}
	cancelRec := httptest.NewRecorder()
	p.ServeHTTP(cancelRec, httptest.NewRequest(http.MethodPost, "/gateway/btw/cancel", strings.NewReader(`{"cascadeId":"side-q-1"}`)))
	if cancelRec.Code != http.StatusOK || canceledID != "side-q-1" {
		t.Fatalf("btw/cancel failed: code=%d canceled=%q", cancelRec.Code, canceledID)
	}

	// Verify list endpoint returns tracked side question with forkStepIndex and clean preview.
	listRec := httptest.NewRecorder()
	p.ServeHTTP(listRec, httptest.NewRequest(http.MethodGet, "/gateway/btw/list?cascadeId=main-conv", nil))
	if listRec.Code != http.StatusOK || !strings.Contains(listRec.Body.String(), "side-q-1") {
		t.Fatalf("btw/list unexpected: code=%d body=%s", listRec.Code, listRec.Body.String())
	}

	// Verify parent /gateway/cascade/messages returns anchored inline SideQuestions card.
	parentMsgsRec := httptest.NewRecorder()
	p.ServeHTTP(parentMsgsRec, httptest.NewRequest(http.MethodGet, "/gateway/cascade/messages?cascadeId=main-conv", nil))
	if parentMsgsRec.Code != http.StatusOK {
		t.Fatalf("parent cascade-messages failed: %d %s", parentMsgsRec.Code, parentMsgsRec.Body.String())
	}
	var parentResp CascadeMessagesResponse
	if err := json.Unmarshal(parentMsgsRec.Body.Bytes(), &parentResp); err != nil {
		t.Fatal(err)
	}
	if len(parentResp.SideQuestions) != 1 || parentResp.SideQuestions[0].CascadeID != "side-q-1" || parentResp.SideQuestions[0].ForkStepIndex != 1 {
		t.Fatalf("expected 1 inline side question anchored at step 1, got: %+v", parentResp.SideQuestions)
	}
	if parentResp.SideQuestions[0].AnchorMessageID != "step-1" {
		t.Fatalf("expected anchorMessageId=step-1, got %q", parentResp.SideQuestions[0].AnchorMessageID)
	}

	// Verify side-question /gateway/cascade/messages returns BranchOrigin metadata.
	sideMsgsRec := httptest.NewRecorder()
	p.ServeHTTP(sideMsgsRec, httptest.NewRequest(http.MethodGet, "/gateway/cascade/messages?cascadeId=side-q-1", nil))
	var sideResp CascadeMessagesResponse
	if err := json.Unmarshal(sideMsgsRec.Body.Bytes(), &sideResp); err != nil {
		t.Fatal(err)
	}
	if sideResp.BranchOrigin == nil || !sideResp.BranchOrigin.IsSideQuestion || sideResp.BranchOrigin.ParentCascadeID != "main-conv" || sideResp.BranchOrigin.ForkStepIndex != 1 || sideResp.BranchOrigin.ForkTurnNumber != 1 {
		t.Fatalf("unexpected sideResp.BranchOrigin: %+v", sideResp.BranchOrigin)
	}

	// Verify desktop non-/api/ GetAllCascadeTrajectories still hides side-q-1 as a top-level entry.
	desktopRec := httptest.NewRecorder()
	desktopReq := httptest.NewRequest(http.MethodPost, "/exa.language_server_pb.LanguageServerService/GetAllCascadeTrajectories", strings.NewReader(`{}`))
	p.ServeHTTP(desktopRec, desktopReq)
	var desktopEnv struct {
		TrajectorySummaries map[string]map[string]interface{} `json:"trajectorySummaries"`
	}
	if err := json.Unmarshal(desktopRec.Body.Bytes(), &desktopEnv); err != nil {
		t.Fatal(err)
	}
	if desktopRec.Code != http.StatusOK || desktopEnv.TrajectorySummaries["main-conv"] == nil || desktopEnv.TrajectorySummaries["side-q-1"] != nil {
		t.Fatalf("expected desktop list to contain main-conv and hide side-q-1 entry, got: %s", desktopRec.Body.String())
	}

	// Verify mobile /api/ GetAllCascadeTrajectories KEEPS side-q-1 with branchParentId & isSideQuestion.
	mobileRec := httptest.NewRecorder()
	mobileReq := httptest.NewRequest(http.MethodPost, "/api/exa.language_server_pb.LanguageServerService/GetAllCascadeTrajectories", strings.NewReader(`{}`))
	p.ServeHTTP(mobileRec, mobileReq)
	var mobileEnv struct {
		TrajectorySummaries map[string]map[string]interface{} `json:"trajectorySummaries"`
	}
	if err := json.Unmarshal(mobileRec.Body.Bytes(), &mobileEnv); err != nil {
		t.Fatal(err)
	}
	sideSummary := mobileEnv.TrajectorySummaries["side-q-1"]
	if mobileRec.Code != http.StatusOK || sideSummary == nil || sideSummary["isSideQuestion"] != true || sideSummary["branchParentId"] != "main-conv" || sideSummary["branchParentTitle"] != "Main Conv" {
		t.Fatalf("expected mobile /api/ list to keep enriched side-q-1 branch, got: %s", mobileRec.Body.String())
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

func TestExtractBranchOriginDoesNotMisclassifyCompactedCascade(t *testing.T) {
	raw := &upstreamTrajectoryResp{}
	raw.Trajectory.CascadeID = "normal-compacted-conv"
	raw.Trajectory.TrajectoryType = "CORTEX_TRAJECTORY_TYPE_CASCADE"
	var step1 TrajectoryStep
	if err := json.Unmarshal([]byte(`{"type":"CORTEX_STEP_TYPE_SYSTEM_MESSAGE","systemMessage":{"message":"<CONTEXT_SUMMARY>Discussing side question and parent conversation behavior.</CONTEXT_SUMMARY>"}}`), &step1); err != nil {
		t.Fatal(err)
	}
	steps := []TrajectoryStep{
		{Type: "CORTEX_STEP_TYPE_USER_INPUT"},
		step1,
	}
	if origin := extractBranchOrigin(raw, steps); origin != nil {
		t.Fatalf("normal CASCADE with summary mentioning side question must not be classified as branch, got %+v", origin)
	}
}

func TestBuildTrajectoryMessagesSetsStepIndexOnToolsAndRespectsBranchBoundary(t *testing.T) {
	var rawSteps []TrajectoryStep
	if err := json.Unmarshal([]byte(`[
		{"type":"CORTEX_STEP_TYPE_USER_INPUT","userInput":{"items":[{"text":"Parent Q1"}]}},
		{"type":"CORTEX_STEP_TYPE_RUN_COMMAND","runCommand":{"commandLine":"ls"}},
		{"type":"CORTEX_STEP_TYPE_PLANNER_RESPONSE","plannerResponse":{"response":"Parent A1"}},
		{"type":"CORTEX_STEP_TYPE_RUN_COMMAND","runCommand":{"commandLine":"pwd"}},
		{"type":"CORTEX_STEP_TYPE_SYSTEM_MESSAGE","systemMessage":{"message":"Above is the conversation history from the parent conversation; what follows is a side question (/btw) branched from that conversation."}},
		{"type":"CORTEX_STEP_TYPE_USER_INPUT","userInput":{"items":[{"text":"Side Q?"}]}},
		{"type":"CORTEX_STEP_TYPE_RUN_COMMAND","runCommand":{"commandLine":"date"}},
		{"type":"CORTEX_STEP_TYPE_PLANNER_RESPONSE","plannerResponse":{"response":"Side A"}}
	]`), &rawSteps); err != nil {
		t.Fatal(err)
	}

	msgs, totalTools := buildTrajectoryMessages(rawSteps)
	if totalTools != 3 {
		t.Fatalf("totalTools = %d, want 3", totalTools)
	}
	// Expect: user(0), tools(1), agent(2), tools(3), user(5), tools(6), agent(7)
	if len(msgs) != 7 {
		t.Fatalf("got %d messages, want 7: %+v", len(msgs), msgs)
	}
	wantTypes := []string{"user", "tools", "agent", "tools", "user", "tools", "agent"}
	wantSteps := []int{0, 1, 2, 3, 5, 6, 7}
	for i := range wantTypes {
		if msgs[i].Type != wantTypes[i] {
			t.Fatalf("msg[%d].Type = %q, want %q", i, msgs[i].Type, wantTypes[i])
		}
		gotStep, ok := messageBranchStepIndex(msgs[i])
		if !ok || gotStep != wantSteps[i] {
			t.Fatalf("messageBranchStepIndex(msg[%d]) = (%d, %v), want (%d, true)", i, gotStep, ok, wantSteps[i])
		}
	}

	raw := &upstreamTrajectoryResp{}
	raw.Trajectory.CascadeID = "side-tools-test"
	raw.Trajectory.TrajectoryType = "CORTEX_TRAJECTORY_TYPE_SIDE_QUESTION"
	raw.Trajectory.Steps = rawSteps
	det := (&Proxy{}).ParseTrajectoryDetails(raw)
	if det.BranchOrigin == nil || det.BranchOrigin.ForkStepIndex != 3 {
		t.Fatalf("unexpected BranchOrigin: %+v", det.BranchOrigin)
	}
	_, branchMsgs, q, _, ans, _, turns := extractSideQuestionBranchMessages(raw, det, BtwSideQuestionEntry{})
	if q != "Side Q?" || ans != "Side A" || turns != 1 {
		t.Fatalf("unexpected branch summary: q=%q ans=%q turns=%d", q, ans, turns)
	}
	if len(branchMsgs) != 3 || branchMsgs[0].Type != "user" || branchMsgs[1].Type != "tools" || branchMsgs[2].Type != "agent" {
		t.Fatalf("expected only post-fork messages [user, tools, agent], got %+v", branchMsgs)
	}
}

func TestResolveSideQuestionForkStepIndexSnapsToTurnBoundary(t *testing.T) {
	steps := []TrajectoryStep{
		{Type: "CORTEX_STEP_TYPE_USER_INPUT"},       // 0: Turn 1 user
		{Type: "CORTEX_STEP_TYPE_RUN_COMMAND"},      // 1: Turn 1 tool
		{Type: "CORTEX_STEP_TYPE_PLANNER_RESPONSE"}, // 2: Turn 1 interim agent
		{Type: "CORTEX_STEP_TYPE_RUN_COMMAND"},      // 3: Turn 1 tool
		{Type: "CORTEX_STEP_TYPE_PLANNER_RESPONSE"}, // 4: Turn 1 end (boundary)
		{Type: "CORTEX_STEP_TYPE_USER_INPUT"},       // 5: Turn 2 user
		{Type: "CORTEX_STEP_TYPE_PLANNER_RESPONSE"}, // 6: Turn 2 end (tail)
	}

	cases := []struct {
		name        string
		requested   int
		wantStep    int
		wantForkObj bool
	}{
		{"turn1_user_snaps_to_turn1_end", 0, 4, true},
		{"turn1_mid_agent_snaps_to_turn1_end", 2, 4, true},
		{"turn1_end_stays_at_boundary", 4, 4, true},
		{"latest_turn_user_uses_cascadeId", 5, 6, false},
		{"latest_turn_tail_uses_cascadeId", 6, 6, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotStep, gotFork := resolveSideQuestionForkStepIndex(steps, tc.requested)
			if gotStep != tc.wantStep || gotFork != tc.wantForkObj {
				t.Fatalf("resolveSideQuestionForkStepIndex(%d) = (%d, %v), want (%d, %v)",
					tc.requested, gotStep, gotFork, tc.wantStep, tc.wantForkObj)
			}
		})
	}
}
