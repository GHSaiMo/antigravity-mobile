package proxy

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyTrajectoryContextEnrichmentPostCompactAndBreakdown(t *testing.T) {
	step0 := 0
	step1 := 1
	step2 := 2

	heavyOut := strings.Repeat("build output log line with compiler diagnostics\n", 300)
	steps := []TrajectoryStep{
		{
			Type: "CORTEX_STEP_TYPE_USER_INPUT",
			UserInput: &TrajectoryUserInput{
				UserResponse: "Fix the failing unit test in auth.go",
			},
		},
		{
			Type: "CORTEX_STEP_TYPE_RUN_COMMAND",
			RunCommand: &TrajectoryRunCommand{
				CommandLine:    "go test ./...",
				CombinedOutput: &TrajectoryCommandOutput{Full: heavyOut},
			},
		},
		{
			Type: "CORTEX_STEP_TYPE_PLANNER_RESPONSE",
			PlannerResponse: &TrajectoryPlannerResponse{
				Thinking: strings.Repeat("analyzing stack trace carefully ", 80),
				Response: "I found the bug and fixed it.",
			},
		},
	}
	msgs := []CascadeMessageItem{
		{ID: "msg-user-0", Role: "user", StepIndex: &step0},
		{ID: "msg-agent-2", Role: "agent", StepIndex: &step2, ToolDetails: []ToolDetailItem{{StepIndex: step1, Name: "run_command"}}},
	}

	// 1. Active session (no checkpoint yet): 4-category breakdown reconciles to UsedTokens + TopConsumers resolve MessageID.
	base := &ContextMeterItem{
		UsedTokens: 120_000,
		MaxTokens:  200_000,
		Percentage: 60.0,
	}
	enriched := applyTrajectoryContextEnrichment(base, steps, msgs)
	if enriched == nil || enriched.Breakdown == nil {
		t.Fatalf("expected enriched meter with breakdown, got %+v", enriched)
	}
	if enriched.Compacted {
		t.Fatalf("expected Compacted=false before checkpoint")
	}
	var catSum int64
	for _, c := range enriched.Breakdown.Categories {
		catSum += c.Tokens
	}
	if catSum != 120_000 {
		t.Fatalf("category token sum = %d, want 120000 (categories=%+v)", catSum, enriched.Breakdown.Categories)
	}
	if len(enriched.Breakdown.TopConsumers) == 0 {
		t.Fatalf("expected non-empty TopConsumers")
	}
	if enriched.Breakdown.TopConsumers[0].MessageID != "msg-agent-2" {
		t.Fatalf("top consumer messageId = %q, want msg-agent-2", enriched.Breakdown.TopConsumers[0].MessageID)
	}

	// 2. Append a successful CHECKPOINT with no planner step after it -> immediate watermark drop.
	var cpStep TrajectoryStep
	if err := json.Unmarshal([]byte(`{"type":"CORTEX_STEP_TYPE_CHECKPOINT","status":"CORTEX_STEP_STATUS_DONE","checkpoint":{"sessionSummary":"User asked to fix auth.go test; root cause identified and patched."}}`), &cpStep); err != nil {
		t.Fatal(err)
	}
	stepsWithCP := append(append([]TrajectoryStep(nil), steps...), cpStep)
	baseHigh := &ContextMeterItem{
		UsedTokens: 185_000,
		MaxTokens:  200_000,
		Percentage: 92.5,
		IsOverflow: true,
	}
	compacted := applyTrajectoryContextEnrichment(baseHigh, stepsWithCP, msgs)
	if !compacted.Compacted {
		t.Fatalf("expected Compacted=true immediately after successful CHECKPOINT")
	}
	if compacted.CheckpointCount != 1 || compacted.LastCheckpointStep == nil || *compacted.LastCheckpointStep != 3 {
		t.Fatalf("unexpected checkpoint metadata: count=%d last=%v", compacted.CheckpointCount, compacted.LastCheckpointStep)
	}
	if compacted.UsedTokens >= 30_000 || compacted.Percentage >= 20.0 || compacted.IsOverflow {
		t.Fatalf("expected immediate watermark drop after compact, got used=%d pct=%.1f overflow=%v",
			compacted.UsedTokens, compacted.Percentage, compacted.IsOverflow)
	}
	// Ensure original base struct was not mutated.
	if baseHigh.UsedTokens != 185_000 {
		t.Fatalf("base ContextMeterItem was mutated: %d", baseHigh.UsedTokens)
	}
}

func TestEstimateAttachmentFileTokens(t *testing.T) {
	tmp := t.TempDir()

	// 1. Valid 1000x800 PNG => ~1000*800/750 = 1067 tokens
	imgPath := filepath.Join(tmp, "shot.png")
	img := image.NewRGBA(image.Rect(0, 0, 1000, 800))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	f, err := os.Create(imgPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	_ = f.Close()
	fi, _ := os.Stat(imgPath)
	imgTok := estimateAttachmentFileTokens(imgPath, "shot.png", "image/png", "image", fi.Size())
	if imgTok < 900 || imgTok > 1200 {
		t.Fatalf("image token estimate = %d, want ~1067", imgTok)
	}

	// 2. CJK text file (1000 Chinese chars => ~1200 tokens)
	txtPath := filepath.Join(tmp, "notes.md")
	txtData := []byte(strings.Repeat("测试文档内容", 200)) // 1200 CJK runes
	if err := os.WriteFile(txtPath, txtData, 0600); err != nil {
		t.Fatal(err)
	}
	txtTok := estimateAttachmentFileTokens(txtPath, "notes.md", "text/markdown", "code", int64(len(txtData)))
	if txtTok < 1200 || txtTok > 1600 {
		t.Fatalf("CJK text token estimate = %d, want ~1440", txtTok)
	}

	// 3. PDF heuristic
	pdfTok := estimateAttachmentFileTokens("", "report.pdf", "application/pdf", "file", 240_000)
	if pdfTok != 10_000 {
		t.Fatalf("PDF token estimate = %d, want 10000", pdfTok)
	}
}

func TestPostCompactRehydrateHookLifecycleAndExecution(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	hooksPath := filepath.Join(tmp, "hooks.json")
	t.Setenv("MGY_HOOKS_CONFIG_PATH", hooksPath)

	// Pre-populate hooks.json with an unrelated custom user hook to verify preservation.
	if err := os.WriteFile(hooksPath, []byte(`{"user-custom-hook":{"enabled":true}}`), 0600); err != nil {
		t.Fatal(err)
	}

	if IsPostCompactRehydrateHookInstalled() {
		t.Fatalf("expected hook not installed initially")
	}
	if err := SyncPostCompactRehydrateHook(true); err != nil {
		t.Fatalf("SyncPostCompactRehydrateHook(true) failed: %v", err)
	}
	if !IsPostCompactRehydrateHookInstalled() {
		t.Fatalf("expected hook installed after SyncPostCompactRehydrateHook(true)")
	}

	rawHooks, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rawHooks), `"user-custom-hook"`) || !strings.Contains(string(rawHooks), `"mgy-post-compact-rehydrate"`) {
		t.Fatalf("hooks.json missing expected entries: %s", string(rawHooks))
	}

	// Disable and verify custom hook remains intact.
	if err := SyncPostCompactRehydrateHook(false); err != nil {
		t.Fatalf("SyncPostCompactRehydrateHook(false) failed: %v", err)
	}
	if IsPostCompactRehydrateHookInstalled() {
		t.Fatalf("expected hook uninstalled after SyncPostCompactRehydrateHook(false)")
	}
	rawHooks, _ = os.ReadFile(hooksPath)
	if !strings.Contains(string(rawHooks), `"user-custom-hook"`) {
		t.Fatalf("user-custom-hook was lost on disable: %s", string(rawHooks))
	}

	// Prepare workspace with a normal code file and a sensitive .env file.
	wsDir := filepath.Join(tmp, "workspace")
	if err := os.MkdirAll(wsDir, 0700); err != nil {
		t.Fatal(err)
	}
	codeFile := filepath.Join(wsDir, "handler.go")
	if err := os.WriteFile(codeFile, []byte("package main\n// token=sk-live1234567890abcdef12345\nfunc Handle() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	envFile := filepath.Join(wsDir, ".env")
	if err := os.WriteFile(envFile, []byte("DATABASE_SECRET=supersecretvalue123456\n"), 0600); err != nil {
		t.Fatal(err)
	}

	// Build a transcript JSONL with user input, edit on handler.go & .env, and a CHECKPOINT at step 3.
	convDir := filepath.Join(tmp, "conv-1")
	if err := os.MkdirAll(convDir, 0700); err != nil {
		t.Fatal(err)
	}
	transcriptPath := filepath.Join(convDir, "transcript.jsonl")
	lines := []map[string]any{
		{"step_index": 0, "type": "CORTEX_STEP_TYPE_USER_INPUT", "status": "DONE", "created_at": "2026-11-27T01:00:00Z", "content": "Refactor handler.go with key Bearer abcdefghijklmnop1234"},
		{
			"step_index": 1,
			"type":       "CORTEX_STEP_TYPE_PLANNER_RESPONSE",
			"status":     "DONE",
			"created_at": "2026-11-27T01:00:05Z",
			"content":    "Updating handler.go and inspecting env",
			"tool_calls": []map[string]any{
				{"name": "edit_file", "args": map[string]any{"Path": codeFile}},
				{"name": "read_file", "args": map[string]any{"Path": envFile}},
			},
		},
		{"step_index": 2, "type": "CORTEX_STEP_TYPE_CHECKPOINT", "status": "CORTEX_STEP_STATUS_DONE", "created_at": "2026-11-27T01:00:10Z", "content": "Compacted history"},
	}
	var transcriptBuf bytes.Buffer
	for _, row := range lines {
		b, _ := json.Marshal(row)
		transcriptBuf.Write(b)
		transcriptBuf.WriteByte('\n')
	}
	if err := os.WriteFile(transcriptPath, transcriptBuf.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}

	hookReq, _ := json.Marshal(rehydrateHookInput{
		ConversationID:  "conv-1",
		WorkspacePaths:  []string{wsDir},
		TranscriptPath:  transcriptPath,
		InitialNumSteps: 4,
	})

	// 1st run: should inject rehydration systemMessage, redact secrets, and exclude .env.
	var out1 bytes.Buffer
	if err := RunRehydrateHook(bytes.NewReader(hookReq), &out1); err != nil {
		t.Fatalf("RunRehydrateHook 1st call failed: %v", err)
	}
	var parsed1 rehydrateHookOutput
	if err := json.Unmarshal(out1.Bytes(), &parsed1); err != nil {
		t.Fatalf("invalid hook JSON output: %v (%s)", err, out1.String())
	}
	if len(parsed1.InjectSteps) != 1 || parsed1.InjectSteps[0].SystemMessage == nil {
		t.Fatalf("expected 1 injected systemMessage, got %s", out1.String())
	}
	injected := parsed1.InjectSteps[0].SystemMessage.SystemMessage
	if !strings.Contains(injected, "Refactor handler.go") {
		t.Fatalf("expected user intent in injected message, got:\n%s", injected)
	}
	if !strings.Contains(injected, "func Handle()") {
		t.Fatalf("expected modified workspace file snippet in injected message, got:\n%s", injected)
	}
	if strings.Contains(injected, "sk-live1234567890abcdef") || strings.Contains(injected, "abcdefghijklmnop1234") {
		t.Fatalf("secret was not redacted in injected message:\n%s", injected)
	}
	if strings.Contains(injected, "supersecretvalue123456") {
		t.Fatalf("sensitive .env content leaked into injected message:\n%s", injected)
	}

	// 2nd run on same transcript: must dedup and output `{}`.
	var out2 bytes.Buffer
	if err := RunRehydrateHook(bytes.NewReader(hookReq), &out2); err != nil {
		t.Fatalf("RunRehydrateHook 2nd call failed: %v", err)
	}
	if strings.TrimSpace(out2.String()) != "{}" {
		t.Fatalf("expected empty JSON {} on deduped 2nd call, got: %s", out2.String())
	}

	// 3rd test: cold-start protection on an old conversation where checkpoint is > 16 steps behind.
	oldDir := filepath.Join(tmp, "conv-old")
	_ = os.MkdirAll(oldDir, 0700)
	oldTranscript := filepath.Join(oldDir, "transcript.jsonl")
	_ = os.WriteFile(oldTranscript, transcriptBuf.Bytes(), 0600)
	oldReq, _ := json.Marshal(rehydrateHookInput{
		ConversationID:  "conv-old",
		WorkspacePaths:  []string{wsDir},
		TranscriptPath:  oldTranscript,
		InitialNumSteps: 40, // 40 - 3 = 37 > 16
	})
	var out3 bytes.Buffer
	if err := RunRehydrateHook(bytes.NewReader(oldReq), &out3); err != nil {
		t.Fatalf("RunRehydrateHook cold-start failed: %v", err)
	}
	if strings.TrimSpace(out3.String()) != "{}" {
		t.Fatalf("expected cold-start guard to return {}, got: %s", out3.String())
	}
}

func TestCollectNextPromptVerificationEvidence(t *testing.T) {
	exitZero := 0
	exitOne := 1
	details := &TrajectoryDetails{
		AllMessages: []CascadeMessageItem{
			{ID: "u1", Role: "user", Text: "Run unit tests and fix failures"},
			{
				ID:   "a1",
				Role: "agent",
				Text: "Ran tests; one failure remains in auth.",
				ToolDetails: []ToolDetailItem{
					{Name: "run_command", Command: "go vet ./...", ExitCode: &exitZero, Output: "clean"},
					{Name: "run_command", Command: "go test ./internal/auth -H Bearer secretToken99999999", ExitCode: &exitOne, ErrorText: "FAIL TestLogin: expected 200 got 401"},
				},
			},
		},
	}

	prompt, _ := buildNextPromptInput("test-cascade", details)
	if !strings.Contains(prompt, "[VERIFICATION_EVIDENCE]") {
		t.Fatalf("expected [VERIFICATION_EVIDENCE] block in next-prompt input:\n%s", prompt)
	}
	if !strings.Contains(prompt, "exit=0") || !strings.Contains(prompt, "exit=1") || !strings.Contains(prompt, "FAIL TestLogin") {
		t.Fatalf("expected exit codes and failure summary in verification evidence:\n%s", prompt)
	}
	if strings.Contains(prompt, "secretToken99999999") {
		t.Fatalf("expected bearer token to be redacted in verification evidence:\n%s", prompt)
	}
}
