package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestDecodeUpstreamTrajectoryStreaming_ParityWithJSONUnmarshal(t *testing.T) {
	payload := `{
		"unknownTopBefore": {"nested": [1, true, null, {"deep": "value"}]},
		"status": "CASCADE_RUN_STATUS_IDLE",
		"trajectory": {
			"unknownTrajBefore": [10, 20, {"k": "v"}],
			"trajectoryId": "traj-parity-1",
			"cascadeId": "cid-parity-1",
			"trajectoryType": "CASCADE_TRAJECTORY_TYPE_MAIN",
			"workspaceUris": ["file:///tmp/workspace-a", "file:///tmp/workspace-b"],
			"steps": [
				{
					"type": "CORTEX_STEP_TYPE_USER_INPUT",
					"status": "CORTEX_STEP_STATUS_DONE",
					"metadata": {"createdAt": "2026-11-27T14:00:00Z"},
					"userInput": {"userResponse": "Investigate 4301-step trajectory truncation"},
					"hugeUnknownStepBlob": {"lines": ["a", "b", "c"]}
				},
				{
					"type": "CORTEX_STEP_TYPE_PLANNER_RESPONSE",
					"status": "CORTEX_STEP_STATUS_DONE",
					"metadata": {"createdAt": "2026-11-27T14:00:05Z", "completedAt": "2026-11-27T14:00:09Z"},
					"plannerResponse": {"response": "Streaming decoder keeps peak memory low.", "thinking": "Step by step"}
				}
			],
			"annotations": {
				"title": "Parity Session",
				"lastUserViewTime": "2026-11-27T14:01:00Z",
				"planExecute": true,
				"sideQuestionCascadeIds": ["side-1"],
				"sideQuestionTitles": {"side-1": "Quick question"}
			},
			"summary": "Summary fallback",
			"metadata": {
				"parentConversationId": "",
				"projectId": "proj-1",
				"sourceMetadata": {"origin": "desktop"},
				"subagentSpec": {"typeName": "research", "role": "Investigator"}
			},
			"executorMetadatas": [
				{"cascadeConfig": {"model": "MODEL_PLACEHOLDER_M420"}}
			],
			"parentReferences": [
				{"parentCascadeId": "parent-0", "stepIndex": 2}
			],
			"unknownTrajAfter": "ignored trailing string"
		},
		"pendingAgentMessages": [
			{"id": "pm-1", "sender": "user", "content": "queued follow-up"}
		],
		"unknownTopAfter": 999
	}`

	var want upstreamTrajectoryResp
	if err := json.Unmarshal([]byte(payload), &want); err != nil {
		t.Fatalf("json.Unmarshal baseline failed: %v", err)
	}
	compactTrajectoryMedia(&want)

	var got upstreamTrajectoryResp
	if err := decodeUpstreamTrajectoryStreaming(strings.NewReader(payload), &got); err != nil {
		t.Fatalf("decodeUpstreamTrajectoryStreaming failed: %v", err)
	}

	if !reflect.DeepEqual(want, got) {
		wantJSON, _ := json.Marshal(want)
		gotJSON, _ := json.Marshal(got)
		t.Fatalf("streaming decode mismatch:\nwant: %s\ngot:  %s", wantJSON, gotJSON)
	}

	// Verify null trajectory & null steps & case-insensitive keys decode cleanly.
	var nullOut upstreamTrajectoryResp
	if err := decodeUpstreamTrajectoryStreaming(strings.NewReader(`{"Status":"CASCADE_RUN_STATUS_IDLE","Trajectory":null}`), &nullOut); err != nil {
		t.Fatalf("null trajectory failed: %v", err)
	}
	if nullOut.Status != "CASCADE_RUN_STATUS_IDLE" {
		t.Fatalf("expected status CASCADE_RUN_STATUS_IDLE, got %q", nullOut.Status)
	}

	var nullSteps upstreamTrajectoryResp
	if err := decodeUpstreamTrajectoryStreaming(strings.NewReader(`{"trajectory":{"CascadeId":"c-null-steps","Steps":null}}`), &nullSteps); err != nil {
		t.Fatalf("null steps failed: %v", err)
	}
	if nullSteps.Trajectory.CascadeID != "c-null-steps" || len(nullSteps.Trajectory.Steps) != 0 {
		t.Fatalf("unexpected null steps decode: %+v", nullSteps)
	}
}

func TestFetchLargeTrajectory_Exceeds64MiB_MessagesAndTitleRecovery(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const (
		cascadeID  = "cid-ultra-long-4301"
		totalSteps = 4301
	)
	ClearTrajectoryCache(cascadeID)
	t.Cleanup(func() { ClearTrajectoryCache(cascadeID) })

	// Each step embeds a ~16 KiB unknown upstream field so the total JSON payload is ~68.8 MiB
	// (> 64 MiB), reproducing the exact 4,301-step / ~65.2 MiB failure from v1.2.8-beta.3.
	padBlob := strings.Repeat("x", 16*1024)
	var rawTrajBytesWritten atomic.Int64
	var getTrajCalls atomic.Int32

	p, _ := newMockTLSProxy(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/GetAllCascadeTrajectories"):
			// Reproduce upstream omitting summary & annotations.title for this ultra-long session.
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"trajectorySummaries": {
					"` + cascadeID + `": {
						"status": "CASCADE_RUN_STATUS_IDLE",
						"stepCount": 4301,
						"lastModifiedTime": "2026-11-27T14:55:00Z"
					}
				}
			}`))
		case strings.HasSuffix(r.URL.Path, "/GetCascadeTrajectory"):
			getTrajCalls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			var written int64
			writeChunk := func(s string) {
				n, _ := w.Write([]byte(s))
				written += int64(n)
			}
			writeChunk(`{"status":"CASCADE_RUN_STATUS_IDLE","futureDesktopField":{"ok":true},"trajectory":{"trajectoryId":"traj-4301","cascadeId":"` + cascadeID + `","steps":[`)
			for i := 0; i < totalSteps; i++ {
				if i > 0 {
					writeChunk(",")
				}
				switch {
				case i == 0:
					writeChunk(`{"type":"CORTEX_STEP_TYPE_USER_INPUT","status":"CORTEX_STEP_STATUS_DONE","metadata":{"createdAt":"2026-11-27T10:00:00Z"},"userInput":{"userResponse":"超长 4301 步会话排查"},"unknownPadding":"` + padBlob + `"}`)
				case i%200 == 0:
					writeChunk(fmt.Sprintf(`{"type":"CORTEX_STEP_TYPE_USER_INPUT","status":"CORTEX_STEP_STATUS_DONE","metadata":{"createdAt":"2026-11-27T11:00:00Z"},"userInput":{"userResponse":"第 %d 步追问"},"unknownPadding":"%s"}`, i, padBlob))
				case i == totalSteps-1:
					writeChunk(`{"type":"CORTEX_STEP_TYPE_PLANNER_RESPONSE","status":"CORTEX_STEP_STATUS_DONE","metadata":{"createdAt":"2026-11-27T14:55:00Z"},"plannerResponse":{"response":"最终总结：全部 4301 步已成功加载。"},"unknownPadding":"` + padBlob + `"}`)
				default:
					writeChunk(fmt.Sprintf(`{"type":"CORTEX_STEP_TYPE_VIEW_FILE","status":"CORTEX_STEP_STATUS_DONE","viewFile":{"absolutePathUri":"file:///tmp/f%d.go","startLine":1,"endLine":50},"unknownPadding":"%s"}`, i, padBlob))
				}
			}
			writeChunk(`]}}`)
			rawTrajBytesWritten.Store(written)
		case strings.HasSuffix(r.URL.Path, "/GetCascadeTrajectoryGeneratorMetadata"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"generatorMetadata":[]}`))
		default:
			http.NotFound(w, r)
		}
	})

	// 1. /gateway/cascade/messages must return HTTP 200 (never 502 unexpected end of JSON input).
	msgRec := httptest.NewRecorder()
	msgReq := httptest.NewRequest(http.MethodGet, "/gateway/cascade/messages?cascadeId="+cascadeID+"&limit=15", nil)
	p.handleCascadeMessages(msgRec, msgReq)
	if msgRec.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200 from /gateway/cascade/messages on >64 MiB trajectory, got %d: %s", msgRec.Code, msgRec.Body.String())
	}
	if rawTrajBytesWritten.Load() <= 64*1024*1024 {
		t.Fatalf("test setup error: expected upstream payload > 64 MiB, got %d bytes", rawTrajBytesWritten.Load())
	}

	var msgResp CascadeMessagesResponse
	if err := json.Unmarshal(msgRec.Body.Bytes(), &msgResp); err != nil {
		t.Fatalf("failed to decode /gateway/cascade/messages response: %v", err)
	}
	if msgResp.TotalSteps != totalSteps {
		t.Fatalf("expected totalSteps=%d, got %d", totalSteps, msgResp.TotalSteps)
	}
	if msgResp.Title != "超长 4301 步会话排查" {
		t.Fatalf("expected fallback title from step 0 USER_INPUT, got %q", msgResp.Title)
	}
	if len(msgResp.Messages) != 15 || !msgResp.HasMore {
		t.Fatalf("expected 15 paginated messages with hasMore=true, got len=%d hasMore=%v", len(msgResp.Messages), msgResp.HasMore)
	}
	lastMsg := msgResp.Messages[len(msgResp.Messages)-1]
	if !strings.Contains(lastMsg.Text, "全部 4301 步已成功加载") {
		t.Fatalf("expected final message text to survive >64 MiB streaming decode, got %q", lastMsg.Text)
	}

	// 2. FetchTrajectoryDetails (Watcher path) must also succeed from cache without error.
	details, err := p.FetchTrajectoryDetails(cascadeID, 60*time.Second)
	if err != nil {
		t.Fatalf("FetchTrajectoryDetails failed on >64 MiB trajectory: %v", err)
	}
	if details.TotalSteps != totalSteps || details.Title != "超长 4301 步会话排查" {
		t.Fatalf("unexpected FetchTrajectoryDetails result: steps=%d title=%q", details.TotalSteps, details.Title)
	}

	// 3. GetAllCascadeTrajectories must backfill the recovered fallback title instead of blank/"当前对话".
	port, token := p.ActiveUpstream()
	listRec := httptest.NewRecorder()
	listReq := httptest.NewRequest(http.MethodPost, "/api/exa.language_server_pb.LanguageServerService/GetAllCascadeTrajectories", strings.NewReader(`{}`))
	p.handleGetAllCascadeTrajectories(listRec, listReq, port, token)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200 from GetAllCascadeTrajectories, got %d", listRec.Code)
	}
	var listResp struct {
		TrajectorySummaries map[string]struct {
			Summary     string `json:"summary"`
			Annotations *struct {
				Title string `json:"title"`
			} `json:"annotations"`
		} `json:"trajectorySummaries"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("failed to decode list response: %v", err)
	}
	entry := listResp.TrajectorySummaries[cascadeID]
	if entry.Summary != "超长 4301 步会话排查" || entry.Annotations == nil || entry.Annotations.Title != "超长 4301 步会话排查" {
		t.Fatalf("expected list entry to backfill fallback title, got summary=%q ann=%+v", entry.Summary, entry.Annotations)
	}

	// 4. Desktop/raw GetCascadeTrajectory RPC on a >8 MiB trajectory must stream raw from upstream
	// so unknown proto fields (futureDesktopField) are preserved rather than stripped by json.Marshal.
	callsBefore := getTrajCalls.Load()
	rpcRec := httptest.NewRecorder()
	rpcReq := httptest.NewRequest(http.MethodPost, "/exa.language_server_pb.LanguageServerService/GetCascadeTrajectory", bytes.NewBufferString(`{"cascadeId":"`+cascadeID+`"}`))
	p.handleGetCascadeTrajectoryRPC(rpcRec, rpcReq, port, token)
	if rpcRec.Code != http.StatusOK {
		t.Fatalf("expected 200 from raw GetCascadeTrajectory passthrough, got %d", rpcRec.Code)
	}
	if getTrajCalls.Load() <= callsBefore {
		t.Fatalf("expected >8 MiB GetCascadeTrajectory RPC to stream raw from upstream")
	}
	if !strings.Contains(rpcRec.Body.String()[:256], `"futureDesktopField":{"ok":true}`) {
		t.Fatalf("expected raw passthrough to preserve unknown desktop proto fields, got prefix: %s", rpcRec.Body.String()[:256])
	}
}

func TestFetchUpstreamTrajectory_ExceedsSafetyLimit_ReturnsClearErrorNoLoadRetry(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	origMax := maxUpstreamTrajectoryBytes
	origRaw := maxRawTrajectoryCacheBytes
	t.Cleanup(func() {
		maxUpstreamTrajectoryBytes = origMax
		maxRawTrajectoryCacheBytes = origRaw
	})

	// Case A: exceeds maxUpstreamTrajectoryBytes on the streaming path (rawCache < max < payload).
	maxRawTrajectoryCacheBytes = 8 * 1024
	maxUpstreamTrajectoryBytes = 32 * 1024

	const cid = "cid-over-cap"
	ClearTrajectoryCache(cid)
	t.Cleanup(func() { ClearTrajectoryCache(cid) })

	var loadCalls atomic.Int32
	p, _ := newMockTLSProxy(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/GetCascadeTrajectory"):
			w.Header().Set("Content-Type", "application/json")
			pad := strings.Repeat("a", 48*1024)
			_, _ = w.Write([]byte(`{"status":"CASCADE_RUN_STATUS_IDLE","trajectory":{"cascadeId":"` + cid + `","steps":[{"type":"CORTEX_STEP_TYPE_USER_INPUT","userInput":{"userResponse":"hi"},"pad":"` + pad + `"}]}}`))
		case strings.HasSuffix(r.URL.Path, "/LoadTrajectory"):
			var req struct {
				CascadeID string `json:"cascadeId"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			if req.CascadeID == cid {
				loadCalls.Add(1)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		default:
			http.NotFound(w, r)
		}
	})

	port, token := p.ActiveUpstream()
	_, err := p.fetchUpstreamTrajectory(cid, port, token)
	if err == nil {
		t.Fatal("expected error when upstream trajectory exceeds safety limit")
	}
	if !errors.Is(err, errUpstreamTrajectoryTooLarge) {
		t.Fatalf("expected errUpstreamTrajectoryTooLarge, got: %v", err)
	}
	if loadCalls.Load() != 0 {
		t.Fatalf("expected 0 LoadTrajectory retries on size-limit overflow, got %d", loadCalls.Load())
	}

	// Case B: maxRawTrajectoryCacheBytes > maxUpstreamTrajectoryBytes (peek limit clamped to max).
	ClearTrajectoryCache(cid)
	maxRawTrajectoryCacheBytes = 64 * 1024
	maxUpstreamTrajectoryBytes = 16 * 1024
	_, err = p.fetchUpstreamTrajectory(cid, port, token)
	if !errors.Is(err, errUpstreamTrajectoryTooLarge) {
		t.Fatalf("expected clamped peek to return errUpstreamTrajectoryTooLarge, got: %v", err)
	}
}

func TestGetAllCascadeTrajectories_ProbesOlderUntitledSessionAndBackfillsTitle(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const untitledID = "cid-untitled-older-5th"
	ClearTrajectoryCache(untitledID)
	t.Cleanup(func() { ClearTrajectoryCache(untitledID) })

	p, _ := newMockTLSProxy(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/GetAllCascadeTrajectories"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"trajectorySummaries": {
					"cid-1": {"status":"CASCADE_RUN_STATUS_IDLE","stepCount":5,"summary":"T1","lastModifiedTime":"2026-11-27T14:50:00Z"},
					"cid-2": {"status":"CASCADE_RUN_STATUS_IDLE","stepCount":5,"summary":"T2","lastModifiedTime":"2026-11-27T14:40:00Z"},
					"cid-3": {"status":"CASCADE_RUN_STATUS_IDLE","stepCount":5,"summary":"T3","lastModifiedTime":"2026-11-27T14:30:00Z"},
					"cid-4": {"status":"CASCADE_RUN_STATUS_IDLE","stepCount":5,"summary":"T4","lastModifiedTime":"2026-11-27T14:20:00Z"},
					"` + untitledID + `": {"status":"CASCADE_RUN_STATUS_IDLE","stepCount":42,"lastModifiedTime":"2026-11-27T14:10:00Z"}
				}
			}`))
		case strings.HasSuffix(r.URL.Path, "/GetCascadeTrajectory"):
			var req struct {
				CascadeID string `json:"cascadeId"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			w.Header().Set("Content-Type", "application/json")
			if req.CascadeID == untitledID {
				_, _ = w.Write([]byte(`{
					"status":"CASCADE_RUN_STATUS_IDLE",
					"trajectory":{
						"cascadeId":"` + untitledID + `",
						"steps":[{"type":"CORTEX_STEP_TYPE_USER_INPUT","status":"CORTEX_STEP_STATUS_DONE","userInput":{"userResponse":"帮我排查无 summary 历史长会话标题"}}]
					}
				}`))
				return
			}
			_, _ = w.Write([]byte(`{"status":"CASCADE_RUN_STATUS_IDLE","trajectory":{"cascadeId":"` + req.CascadeID + `","steps":[]}}`))
		default:
			http.NotFound(w, r)
		}
	})

	port, token := p.ActiveUpstream()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/exa.language_server_pb.LanguageServerService/GetAllCascadeTrajectories", strings.NewReader(`{}`))
	p.handleGetAllCascadeTrajectories(rec, req, port, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		TrajectorySummaries map[string]struct {
			Summary     string `json:"summary"`
			Annotations *struct {
				Title string `json:"title"`
			} `json:"annotations"`
		} `json:"trajectorySummaries"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode list response: %v", err)
	}
	got := resp.TrajectorySummaries[untitledID]
	if got.Summary != "帮我排查无 summary 历史长会话标题" || got.Annotations == nil || got.Annotations.Title != "帮我排查无 summary 历史长会话标题" {
		t.Fatalf("expected untitled older session to be probed & backfilled, got summary=%q ann=%+v", got.Summary, got.Annotations)
	}
}

func TestFetchAllTrajectoriesUpstream_ExceedsCapReturnsExplicitError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	orig := maxAllTrajectoriesBody
	t.Cleanup(func() { maxAllTrajectoriesBody = orig })
	maxAllTrajectoriesBody = 1024

	p, _ := newMockTLSProxy(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"trajectorySummaries":{"big":{"summary":"` + strings.Repeat("z", 2048) + `"}}}`))
	})
	port, token := p.ActiveUpstream()
	_, err := p.fetchAllTrajectoriesUpstream(context.Background(), port, token)
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected explicit safety limit error, got: %v", err)
	}
}
