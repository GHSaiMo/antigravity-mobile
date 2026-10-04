package proxy

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Characterization ("golden") tests for ParseTrajectoryDetails.
//
// Each scenario feeds a raw upstream trajectory through the parser and compares the
// full result (everything except the echoed input steps) with testdata/parse_golden/<name>.json.
// Regenerate after an intentional behavior change with:
//
//	go test ./internal/proxy -run TestParseTrajectoryDetails_Golden -update-golden
var updateGolden = flag.Bool("update-golden", false, "rewrite ParseTrajectoryDetails golden files")

type goldenScenario struct {
	name  string
	json  string                          // supports {{HOME}} placeholder
	setup func(t *testing.T, home string) // optional: seed caches / files
}

// --- fixture builders -------------------------------------------------------------

func gTraj(cid, status string, extra string, steps ...string) string {
	if extra != "" {
		extra = "," + extra
	}
	return fmt.Sprintf(`{"status":%q,"trajectory":{"cascadeId":%q,"trajectoryId":"traj-%s","steps":[%s]%s}}`,
		status, cid, cid, strings.Join(steps, ","), extra)
}

func gUser(text string) string {
	return fmt.Sprintf(`{"type":"CORTEX_STEP_TYPE_USER_INPUT","status":"CORTEX_STEP_STATUS_DONE","userInput":{"userResponse":%q}}`, text)
}

func gPlanner(text string) string {
	return fmt.Sprintf(`{"type":"CORTEX_STEP_TYPE_PLANNER_RESPONSE","status":"CORTEX_STEP_STATUS_DONE","plannerResponse":{"response":%q}}`, text)
}

func gTool(stepType, metadata string) string {
	if metadata == "" {
		metadata = "{}"
	}
	return fmt.Sprintf(`{"type":%q,"status":"CORTEX_STEP_STATUS_DONE","metadata":%s}`, stepType, metadata)
}

func gArtifact(uri string, requestFeedback, createFile bool) string {
	return fmt.Sprintf(`{"type":"CORTEX_STEP_TYPE_CODE_ACTION","status":"CORTEX_STEP_STATUS_DONE","codeAction":{"isArtifactFile":true,"artifactMetadata":{"requestFeedback":%t},"actionResult":{"edit":{"absoluteUri":%q,"createFile":%t}}}}`,
		requestFeedback, uri, createFile)
}

func gCodeEdit(uri string) string {
	return fmt.Sprintf(`{"type":"CORTEX_STEP_TYPE_CODE_ACTION","status":"CORTEX_STEP_STATUS_DONE","codeAction":{"actionResult":{"edit":{"absoluteUri":%q}}}}`, uri)
}

func gWaiting(interaction string) string {
	return fmt.Sprintf(`{"type":"CORTEX_STEP_TYPE_RUN_COMMAND","status":"CORTEX_STEP_STATUS_WAITING","requestedInteraction":%s}`, interaction)
}

func gErrorStep(shortErr string, showUser bool) string {
	return fmt.Sprintf(`{"type":"CORTEX_STEP_TYPE_ERROR_MESSAGE","status":"CORTEX_STEP_STATUS_DONE","errorMessage":{"error":{"shortError":%q},"shouldShowUser":%t}}`, shortErr, showUser)
}

// gHiddenError is an internal retry error the IDE hides from the user (shouldShowModel).
func gHiddenError(shortErr string) string {
	return fmt.Sprintf(`{"type":"CORTEX_STEP_TYPE_ERROR_MESSAGE","status":"CORTEX_STEP_STATUS_DONE","errorMessage":{"error":{"shortError":%q},"shouldShowModel":true}}`, shortErr)
}

func gRunning(body string) string {
	return fmt.Sprintf(`{"type":"CORTEX_STEP_TYPE_RUN_COMMAND","status":"CORTEX_STEP_STATUS_RUNNING",%s}`, body)
}

func gExecCfg(cfgs ...string) string {
	parts := make([]string, len(cfgs))
	for i, c := range cfgs {
		parts[i] = `{"cascadeConfig":` + c + `}`
	}
	return `"executorMetadatas":[` + strings.Join(parts, ",") + `]`
}

const (
	statusIdle    = "CASCADE_RUN_STATUS_IDLE"
	statusRunning = "CASCADE_RUN_STATUS_RUNNING"
)

func goldenScenarios() []goldenScenario {
	home := "{{HOME}}"
	return []goldenScenario{
		// ---- message building ----
		{name: "messages_basic", json: gTraj("g-messages-basic", statusIdle, "",
			gUser("hello"),
			gTool("CORTEX_STEP_TYPE_VIEW_FILE", `{"toolCall":{"name":"view_file"}}`),
			gTool("CORTEX_STEP_TYPE_RUN_COMMAND", `{"toolAction":"Ran command"}`),
			gTool("CORTEX_STEP_TYPE_GREP_SEARCH", ""),
			`{"type":"CORTEX_STEP_TYPE_SYSTEM_MESSAGE","status":"CORTEX_STEP_STATUS_DONE"}`,
			gPlanner("   "),
			gPlanner("done, see ![shot](https://example.com/a.png)"),
			gTool("CORTEX_STEP_TYPE_LIST_DIR", `{"toolCall":{"name":"list_dir"}}`),
			gUser("thanks"),
			gTool("CORTEX_STEP_TYPE_FIND", ""),
		)},
		{name: "messages_errors", json: gTraj("g-messages-errors", statusIdle, "",
			gUser("go"),
			gHiddenError("transient"),
			gErrorStep("fatal boom", true),
			`{"type":"CORTEX_STEP_TYPE_ERROR_MESSAGE","status":"CORTEX_STEP_STATUS_DONE","errorMessage":{"error":{"shortError":"stream was interrupted"}}}`,
			`{"type":"CORTEX_STEP_TYPE_ERROR_MESSAGE","status":"CORTEX_STEP_STATUS_DONE","error":{"shortError":"legacy error"}}`,
		)},
		{name: "messages_user_media", json: gTraj("g-messages-media", statusIdle, "",
			`{"type":"CORTEX_STEP_TYPE_USER_INPUT","userInput":{"userResponse":"look","media":[{"thumbnail":"thumb1","uri":"/x/orig1.png"},{"inlineData":"inline2","uri":"/x/orig1.png"},{"thumbnail":" thumb1 "}],"images":[{"base64Data":"b64a"},{"base64Data":"thumb1"}]}}`,
			`{"type":"CORTEX_STEP_TYPE_USER_INPUT","userInput":{"items":[{"text":"from items"}]}}`,
			`{"type":"CORTEX_STEP_TYPE_USER_INPUT","userInput":{"userResponse":"see MEDIA: /tmp/pic.png and ![i](https://e.com/i.jpg)"}}`,
			`{"type":"CORTEX_STEP_TYPE_USER_INPUT","userInput":{"userResponse":"Comments on artifact URI: file:///a/plan.md\nThe user has approved this document"}}`,
			`{"type":"CORTEX_STEP_TYPE_USER_INPUT","userInput":{"userResponse":"Comments on artifact URI: file:///a/plan.md\nThe user has approved this document","media":[{"thumbnail":"keepme"}]}}`,
			`{"type":"CORTEX_STEP_TYPE_USER_INPUT","userInput":{"userResponse":"   "}}`,
			`{"type":"CORTEX_STEP_TYPE_USER_INPUT"}`,
		)},

		// ---- duration ----
		{name: "duration_rfc3339nano", json: gTraj("g-duration-nano", statusIdle, "",
			`{"type":"CORTEX_STEP_TYPE_USER_INPUT","metadata":{"createdAt":"2026-01-01T10:00:00.000000000Z"},"userInput":{"userResponse":"a"}}`,
			`{"type":"CORTEX_STEP_TYPE_PLANNER_RESPONSE","metadata":{"createdAt":"2026-01-01T11:05:07.500000000Z"},"plannerResponse":{"response":"b"}}`,
		)},
		{name: "duration_rfc3339_skips_invalid", json: gTraj("g-duration-mixed", statusIdle, "",
			`{"type":"CORTEX_STEP_TYPE_USER_INPUT","metadata":{"createdAt":"not-a-time"},"userInput":{"userResponse":"a"}}`,
			`{"type":"CORTEX_STEP_TYPE_PLANNER_RESPONSE","metadata":{"createdAt":"2026-01-01T10:00:00Z"},"plannerResponse":{"response":"b"}}`,
			`{"type":"CORTEX_STEP_TYPE_PLANNER_RESPONSE","metadata":{"createdAt":"2026-01-01T10:00:45Z"},"plannerResponse":{"response":"c"}}`,
			`{"type":"CORTEX_STEP_TYPE_PLANNER_RESPONSE","metadata":{"createdAt":"garbage"},"plannerResponse":{"response":"d"}}`,
		)},
		{name: "duration_no_timestamps", json: gTraj("g-duration-none", statusIdle, "", gUser("a"), gPlanner("b"))},
		{name: "empty_trajectory", json: gTraj("g-empty", statusIdle, "")},

		// ---- model / config resolution ----
		{name: "model_claude_modelname", json: gTraj("g-model-claude", statusIdle,
			gExecCfg(`{"plannerConfig":{"modelName":"claude-sonnet-4-6"}}`), gUser("x"))},
		{name: "model_gemini_planmodel", json: gTraj("g-model-gemini", statusIdle,
			gExecCfg(`{"plannerConfig":{"planModel":"gemini-3.1-pro"}}`), gUser("x"))},
		{name: "model_gpt", json: gTraj("g-model-gpt", statusIdle,
			gExecCfg(`{"plannerConfig":{"modelName":"gpt-oss-120b"}}`), gUser("x"))},
		{name: "model_other_name_kept", json: gTraj("g-model-other", statusIdle,
			gExecCfg(`{"plannerConfig":{"modelName":"mystery-model"}}`), gUser("x"))},
		{name: "model_ignores_null_and_empty_cfg_uses_latest", json: gTraj("g-model-latest", statusIdle,
			gExecCfg(`{"plannerConfig":{"modelName":"gemini-old"}}`, `{"plannerConfig":{"modelName":"claude-new"}}`, `null`, `{}`), gUser("x"))},
		{name: "model_recorded_wins", json: gTraj("g-model-recorded", statusIdle,
			gExecCfg(`{"plannerConfig":{"modelName":"gemini-from-upstream"}}`), gUser("x")),
			setup: func(t *testing.T, _ string) {
				SetCascadeModel("g-model-recorded", "claude-opus-4", json.RawMessage(`{"plannerConfig":{"modelName":"claude-opus-4"}}`))
			}},
		{name: "model_recorded_name_only_cfg_from_upstream", json: gTraj("g-model-recorded-name", statusIdle,
			gExecCfg(`{"plannerConfig":{"modelName":"gemini-from-upstream"}}`), gUser("x")),
			setup: func(t *testing.T, _ string) { SetCascadeModel("g-model-recorded-name", "gpt-recorded", nil) }},

		// ---- workspace / title ----
		{name: "title_annotations", json: gTraj("g-title-ann", statusIdle,
			`"annotations":{"title":"From Annotation"},"summary":"ignored summary","workspaceUris":["file:///ws/one","file:///ws/two"]`, gUser("prompt"))},
		{name: "title_summary", json: gTraj("g-title-sum", statusIdle, `"summary":"Summary Title"`, gUser("prompt"))},
		{name: "title_unnamed_uses_annotation_file", json: gTraj("g-title-file", statusIdle, `"summary":"未命名会话"`, gUser("prompt")),
			setup: func(t *testing.T, _ string) { writeAnnotationTitle("g-title-file", "Title From Disk") }},
		{name: "title_from_user_input", json: gTraj("g-title-user", statusIdle, "", gUser("Please refactor the payment module thoroughly now"))},
		{name: "title_all_empty", json: gTraj("g-title-none", statusIdle, "", gPlanner("only agent"))},

		// ---- proceed detection ----
		{name: "proceed_plan_requests_feedback", json: gTraj("g-proceed-plan", statusIdle, "",
			gUser("plan it"), gPlanner("plan"), gArtifact("file:///p/.gemini/antigravity/brain/g-proceed-plan/implementation_plan.md", true, true))},
		{name: "proceed_reset_by_code_edit_after_plan", json: gTraj("g-proceed-edit", statusIdle, "",
			gUser("plan it"), gArtifact("file:///p/implementation_plan.md", true, true), gCodeEdit("file:///src/main.go"))},
		{name: "proceed_code_edit_before_plan_ok", json: gTraj("g-proceed-edit-before", statusIdle, "",
			gUser("plan it"), gCodeEdit("file:///src/main.go"), gArtifact("file:///p/implementation_plan.md", true, true))},
		{name: "proceed_suppressed_while_running", json: gTraj("g-proceed-running", statusRunning, "",
			gUser("plan it"), gArtifact("file:///p/implementation_plan.md", true, true))},
		{name: "proceed_walkthrough_never", json: gTraj("g-proceed-walk", statusIdle, "",
			gUser("go"), gArtifact("file:///p/brain/x/walkthrough.md", true, true))},
		{name: "proceed_scratch_not_artifact", json: gTraj("g-proceed-scratch", statusIdle, "",
			gUser("go"), gArtifact("file:///p/brain/x/scratch/notes.md", true, true))},
		{name: "proceed_previous_turn_ignored", json: gTraj("g-proceed-prev", statusIdle, "",
			gUser("one"), gArtifact("file:///p/implementation_plan.md", true, true), gUser("two"), gPlanner("ok"))},
		{name: "proceed_approved_but_rerequested_feedback", json: gTraj("g-proceed-approved-ac", statusIdle, "",
			`{"type":"CORTEX_STEP_TYPE_USER_INPUT","userInput":{"userResponse":"go","artifactComments":[{"artifactUri":"file:///p/implementation_plan.md","approvalStatus":"ARTIFACT_APPROVAL_STATUS_APPROVED"}]}}`,
			gArtifact("file:///p/implementation_plan.md", true, true))},
		{name: "proceed_approved_via_artifact_comment_suppresses_disk_feedback", json: gTraj("g-proceed-approved-disk", statusIdle, "",
			`{"type":"CORTEX_STEP_TYPE_USER_INPUT","userInput":{"userResponse":"go","artifactComments":[{"artifactUri":"file://`+home+`/brain/approved.md","approvalStatus":1}]}}`,
			gArtifact("file://"+home+"/brain/approved.md", false, true)),
			setup: func(t *testing.T, h string) {
				mustWrite(t, filepath.Join(h, "brain", "approved.md.metadata.json"), `{"requestFeedback":true}`)
			}},
		{name: "proceed_approved_via_text_suppresses_disk_feedback", json: gTraj("g-proceed-approved-text", statusIdle, "",
			gUser("Comments on artifact URI: file://"+home+"/brain/approved2.md\nThe user has approved this document"),
			gArtifact("file://"+home+"/brain/approved2.md", false, true)),
			setup: func(t *testing.T, h string) {
				mustWrite(t, filepath.Join(h, "brain", "approved2.md.metadata.json"), `{"requestFeedback":true}`)
			}},
		{name: "proceed_via_disk_metadata", json: gTraj("g-proceed-disk", statusIdle, "",
			gUser("go"),
			`{"type":"CORTEX_STEP_TYPE_CODE_ACTION","codeAction":{"isArtifactFile":true,"actionResult":{"edit":{"absoluteUri":"file://`+home+`/brain/doc.md","createFile":true}}}}`),
			setup: func(t *testing.T, h string) {
				mustWrite(t, filepath.Join(h, "brain", "doc.md.metadata.json"), `{"requestFeedback":true}`)
			}},
		{name: "proceed_disk_metadata_false", json: gTraj("g-proceed-disk-false", statusIdle, "",
			gUser("go"),
			`{"type":"CORTEX_STEP_TYPE_CODE_ACTION","codeAction":{"isArtifactFile":true,"actionResult":{"edit":{"absoluteUri":"file://`+home+`/brain/doc2.md","createFile":true}}}}`),
			setup: func(t *testing.T, h string) {
				mustWrite(t, filepath.Join(h, "brain", "doc2.md.metadata.json"), `{"requestFeedback":false}`)
			}},
		{name: "proceed_plan_home_metadata_fallback", json: gTraj("g-proceed-home", statusIdle, "",
			gUser("go"),
			`{"type":"CORTEX_STEP_TYPE_CODE_ACTION","codeAction":{"isArtifactFile":true,"actionSpec":{"createFile":{"path":{"absoluteUri":"file:///nowhere/implementation_plan.md"}}}}}`),
			setup: func(t *testing.T, h string) {
				mustWrite(t, filepath.Join(h, ".gemini/antigravity/brain/g-proceed-home/implementation_plan.md.metadata.json"), `{"requestFeedback":true}`)
			}},
		{name: "proceed_plan_edit_not_create_skips_disk", json: gTraj("g-proceed-edit-only", statusIdle, "",
			gUser("go"), gArtifact("file://"+home+"/brain/doc3.md", false, false)),
			setup: func(t *testing.T, h string) {
				mustWrite(t, filepath.Join(h, "brain", "doc3.md.metadata.json"), `{"requestFeedback":true}`)
			}},

		// ---- pending interaction ----
		{name: "interaction_permission_read", json: gTraj("g-int-read", statusRunning, "",
			gUser("go"), gWaiting(`{"permission":{"resource":{"action":"read_file","target":"/etc/hosts"},"actionDescription":"Read hosts"}}`))},
		{name: "interaction_permission_command", json: gTraj("g-int-cmd", statusRunning, "",
			gUser("go"), gWaiting(`{"permission":{"resource":{"action":"run_command","target":"rm -rf x"}}}`))},
		{name: "interaction_permission_write", json: gTraj("g-int-write", statusRunning, "",
			gUser("go"), gWaiting(`{"permission":{"resource":{"action":"edit_file","target":"/a/b"}}}`))},
		{name: "interaction_permission_other_with_description", json: gTraj("g-int-desc", statusRunning, "",
			gUser("go"), gWaiting(`{"permission":{"resource":{"action":"browse","target":"https://x"},"actionDescription":"Open a page"}}`))},
		{name: "interaction_permission_other_no_description", json: gTraj("g-int-nodesc", statusRunning, "",
			gUser("go"), gWaiting(`{"permission":{"resource":{"action":"browse","target":"https://x"}}}`))},
		{name: "interaction_ask_question_multi", json: gTraj("g-int-ask", statusRunning, "",
			gUser("go"), gWaiting(`{"askQuestion":{"questions":[{"question":"Which DB?","options":[{"id":"a","text":"PG"},{"id":"b","text":"MySQL"}]},{"question":"Pick tags","isMultiSelect":true,"options":[{"id":"x","text":"X"}]},{"question":"Free text only"}]}}`))},
		{name: "interaction_file_permission", json: gTraj("g-int-file", statusRunning, "",
			gUser("go"), gWaiting(`{"filePermission":{"absolutePathUri":"file:///outside/file.txt"}}`))},
		{name: "interaction_run_command", json: gTraj("g-int-run", statusRunning, "",
			gUser("go"), gWaiting(`{"runCommand":{"commandLine":"npm test"}}`))},
		{name: "interaction_empty_ask_question_has_no_type", json: gTraj("g-int-empty", statusRunning, "",
			gUser("go"), gWaiting(`{"askQuestion":{"questions":[]}}`))},
		{name: "interaction_dropped_when_not_running", json: gTraj("g-int-idle", statusIdle, "",
			gUser("go"), gWaiting(`{"runCommand":{"commandLine":"npm test"}}`))},
		{name: "interaction_latest_waiting_step_wins", json: gTraj("g-int-latest", statusRunning, "",
			gWaiting(`{"runCommand":{"commandLine":"first"}}`), gWaiting(`{"runCommand":{"commandLine":"second"}}`))},

		// ---- queued agent messages ----
		{name: "queued_messages_filtering", json: `{"status":"CASCADE_RUN_STATUS_RUNNING","trajectory":{"cascadeId":"g-queued","steps":[]},"pendingAgentMessages":[` +
			`{"id":"q1","content":"visible one","timestamp":"2026-01-01T00:00:00Z"},` +
			`{"id":"q2","content":"hidden","hideFromUser":true},` +
			`{"id":"q3","content":"system generated","sender":"system-bot"},` +
			`{"id":"q4","content":"task result","sender":"user","timestamp":"2026-01-02T00:00:00Z"},` +
			`{"id":"q5","content":"Task id \"abc\" completed with result: ok"},` +
			`{"id":"q6","content":"next invocation only","deliveryStrategy":"DELIVERY_STRATEGY_NEXT_INVOCATION"},` +
			`{"id":"q7","content":"when idle numeric","deliveryStrategy":2,"timestamp":{"seconds":1767225600,"nanos":0}},` +
			`{"id":"q8","content":"","sourceMetadata":{}},` +
			`{"id":"q9","content":"tagged","sourceMetadata":{"origin":"cron"}}]}`},

		// ---- running tasks ----
		{name: "running_tasks_variants", json: gTraj("g-tasks", statusRunning, "",
			gUser("go"),
			gRunning(`"taskDetails":{"id":"bg-1","logUri":"file:///logs/1","description":"npm run dev"},"metadata":{"createdAt":"2026-01-01T00:00:00Z","toolSummary":"dev server"}`),
			gRunning(`"runCommand":{"commandLine":"go test ./..."},"metadata":{"toolCall":{"name":"run_command"}}`),
			gRunning(`"runCommand":{"proposedCommandLine":"make build"}`),
			gRunning(`"metadata":{"toolCall":{"name":"browser_subagent","argumentsJson":"{\"url\":\"x\"}"}}`),
			gRunning(`"metadata":{"toolAction":"Waiting on build","sourceTrajectoryStepInfo":{"stepIndex":42}}`),
			gRunning(``+`"taskDetails":{}`),
		)},
		{name: "running_tasks_ignored_when_done", json: gTraj("g-tasks-done", statusIdle, "",
			gUser("go"), gTool("CORTEX_STEP_TYPE_RUN_COMMAND", `{"toolAction":"done"}`))},

		// ---- status / error resolution ----
		{name: "status_error_in_latest_turn", json: gTraj("g-status-err", statusIdle, "",
			gUser("go"), gPlanner("trying"), gErrorStep("quota exhausted", true))},
		{name: "status_error_resolved_by_later_planner", json: gTraj("g-status-resolved", statusIdle, "",
			gUser("go"), gErrorStep("flaky", true), gPlanner("recovered"))},
		{name: "status_error_resolved_by_later_command", json: gTraj("g-status-resolved-cmd", statusIdle, "",
			gUser("go"), gErrorStep("flaky", true),
			`{"type":"CORTEX_STEP_TYPE_RUN_COMMAND","runCommand":{"commandLine":"ls"}}`)},
		{name: "status_historical_error_previous_turn", json: gTraj("g-status-hist", statusIdle, "",
			gUser("one"), gErrorStep("old failure", true), gUser("two"), gPlanner("fine"))},
		{name: "status_running_with_trailing_error", json: gTraj("g-status-run-err", statusRunning, "",
			gUser("go"), gErrorStep("died", true))},
		{name: "status_running_with_mid_turn_error", json: gTraj("g-status-run-mid", statusRunning, "",
			gUser("go"), gErrorStep("hiccup", true), gPlanner("retrying"))},
		{name: "status_hidden_error_not_an_error", json: gTraj("g-status-hidden", statusIdle, "",
			gUser("go"), gHiddenError("internal retry"))},
		{name: "status_error_cascade_status_passthrough", json: gTraj("g-status-pass", "CASCADE_RUN_STATUS_ERROR", "", gUser("go"))},
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func resetTrajectoryGlobals() {
	c := defaultTrajCache
	c.lastKnownConfigMu.Lock()
	c.lastKnownConfig = nil
	c.lastKnownConfigMu.Unlock()
}

func TestParseTrajectoryDetails_Golden(t *testing.T) {
	for _, sc := range goldenScenarios() {
		sc := sc
		t.Run(sc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			resetTrajectoryGlobals()
			if sc.setup != nil {
				sc.setup(t, home)
			}

			var raw upstreamTrajectoryResp
			if err := json.Unmarshal([]byte(strings.ReplaceAll(sc.json, "{{HOME}}", home)), &raw); err != nil {
				t.Fatalf("bad fixture JSON: %v", err)
			}

			p := &Proxy{}
			details := p.ParseTrajectoryDetails(&raw)

			// Side effects are part of the contract too.
			defaultTrajCache.cascadeTitlesMu.Lock()
			cachedTitle := defaultTrajCache.cascadeTitles[raw.Trajectory.CascadeID]
			defaultTrajCache.cascadeTitlesMu.Unlock()
			defaultTrajCache.lastKnownConfigMu.RLock()
			lastKnown := string(defaultTrajCache.lastKnownConfig)
			defaultTrajCache.lastKnownConfigMu.RUnlock()

			details.Steps = nil // echoed input; keeps goldens readable
			out := struct {
				Details         TrajectoryDetails `json:"details"`
				CachedTitle     string            `json:"cachedTitle"`
				LastKnownConfig string            `json:"lastKnownConfig"`
			}{details, cachedTitle, lastKnown}

			got, err := json.MarshalIndent(out, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			gotStr := strings.ReplaceAll(string(got), home, "{{HOME}}") + "\n"

			path := filepath.Join("testdata", "parse_golden", sc.name+".json")
			if *updateGolden {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(gotStr), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("missing golden %s (run with -update-golden): %v", path, err)
			}
			if string(want) != gotStr {
				t.Errorf("ParseTrajectoryDetails output changed for %s\n--- want\n%s\n--- got\n%s", sc.name, want, gotStr)
			}
		})
	}
}
