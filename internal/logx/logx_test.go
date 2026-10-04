package logx

import (
	"bytes"
	"log"
	"log/slog"
	"strings"
	"testing"
)

func TestInferLevel(t *testing.T) {
	cases := map[string]slog.Level{
		"⚠️  Cloudflare 隧道注册失败: x":           slog.LevelWarn,
		"❌ Failed to bind":                   slog.LevelError,
		"[Stream Perf] writeJSON took 1s":    slog.LevelDebug,
		"[WS] Rejected WebSocket connection": slog.LevelWarn,
		"网关正在启动":                             slog.LevelInfo,
	}
	for msg, want := range cases {
		if got := InferLevel(msg); got != want {
			t.Errorf("InferLevel(%q) = %v, want %v", msg, got, want)
		}
	}
}

func TestCompactHandlerAndBridge(t *testing.T) {
	t.Setenv("MULTIGRAVITY_LOG_LEVEL", "info")
	t.Setenv("MULTIGRAVITY_LOG_FORMAT", "")
	var buf bytes.Buffer
	logger := Init(&buf)

	logger.Warn("audit AUTH_FAILURE", "ip", "1.2.3.4", "reason", "bad token")
	log.Printf("plain message %d", 1)
	log.Printf("[Stream Perf] hidden at info level")

	out := buf.String()
	for _, want := range []string{`[WARN] audit AUTH_FAILURE ip=1.2.3.4 reason="bad token"`, "plain message 1"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Stream Perf") {
		t.Errorf("debug line should be filtered at info level:\n%s", out)
	}
	if strings.Contains(out, "[INFO]") {
		t.Errorf("INFO label should be omitted:\n%s", out)
	}
}

func TestDebugLevelAndJSON(t *testing.T) {
	t.Setenv("MULTIGRAVITY_LOG_LEVEL", "debug")
	t.Setenv("MULTIGRAVITY_LOG_FORMAT", "json")
	var buf bytes.Buffer
	Init(&buf)
	log.Printf("[Stream Perf] shown")
	if !strings.Contains(buf.String(), `"level":"DEBUG"`) {
		t.Errorf("expected JSON debug record, got %s", buf.String())
	}
}
