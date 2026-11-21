// Package logx configures the process-wide structured logger (log/slog).
//
// Output stays terminal-friendly by default ("15:04:05 [WARN] msg k=v"); set
// MULTIGRAVITY_LOG_FORMAT=json for machine-readable output and
// MULTIGRAVITY_LOG_LEVEL=debug|info|warn|error to filter (default info; any
// MULTIGRAVITY_VERBOSE* flag implies debug).
//
// Legacy log.Printf call sites keep working: the standard logger is bridged into
// slog, with a level inferred from the message so they can be filtered too.
package logx

import (
	"context"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"strings"
	"sync"
)

// Init installs the default slog logger writing to w and bridges the standard
// library logger into it. It returns the configured logger.
func Init(w io.Writer) *slog.Logger {
	level := levelFromEnv()
	var h slog.Handler
	if strings.EqualFold(os.Getenv("MULTIGRAVITY_LOG_FORMAT"), "json") {
		h = slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level})
	} else {
		h = newCompactHandler(w, level)
	}
	logger := slog.New(h)
	slog.SetDefault(logger)

	// slog.SetDefault redirects the std logger at a fixed Info level; replace that
	// with a bridge that infers the level from the message text.
	log.SetFlags(0)
	log.SetOutput(&bridgeWriter{logger: logger})
	return logger
}

func levelFromEnv() slog.Level {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("MULTIGRAVITY_LOG_LEVEL"))) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	case "info":
		return slog.LevelInfo
	}
	for _, k := range []string{"MULTIGRAVITY_VERBOSE", "MULTIGRAVITY_LOG_RPC", "MULTIGRAVITY_VERBOSE_RPC", "GATEWAY_VERBOSE_RPC", "GATEWAY_LOG_RPC"} {
		if os.Getenv(k) != "" {
			return slog.LevelDebug
		}
	}
	return slog.LevelInfo
}

// bridgeWriter receives lines from the std logger and re-emits them through slog.
type bridgeWriter struct{ logger *slog.Logger }

func (b *bridgeWriter) Write(p []byte) (int, error) {
	msg := strings.TrimRight(string(p), "\r\n")
	if msg == "" {
		return len(p), nil
	}
	lvl := InferLevel(msg)
	if b.logger.Enabled(context.Background(), lvl) {
		b.logger.Log(context.Background(), lvl, msg)
	}
	return len(p), nil
}

// InferLevel guesses a severity for an unstructured log line.
func InferLevel(msg string) slog.Level {
	switch {
	case strings.Contains(msg, "❌") || strings.Contains(msg, "panic:"):
		return slog.LevelError
	case strings.Contains(msg, "⚠️") || strings.Contains(msg, "WARNING") ||
		strings.Contains(msg, "Rejected") || strings.Contains(msg, "failed") || strings.Contains(msg, "Failed"):
		return slog.LevelWarn
	case strings.HasPrefix(msg, "[Stream Perf]"):
		return slog.LevelDebug
	}
	return slog.LevelInfo
}

// compactHandler renders "15:04:05 [LEVEL] msg k=v ..." (level omitted for INFO).
type compactHandler struct {
	mu     *sync.Mutex
	w      io.Writer
	level  slog.Leveler
	attrs  []slog.Attr
	groups []string
}

func newCompactHandler(w io.Writer, level slog.Leveler) *compactHandler {
	return &compactHandler{mu: &sync.Mutex{}, w: w, level: level}
}

func (h *compactHandler) Enabled(_ context.Context, l slog.Level) bool {
	return l >= h.level.Level()
}

func (h *compactHandler) Handle(_ context.Context, r slog.Record) error {
	var sb strings.Builder
	sb.Grow(128)
	sb.WriteString(r.Time.Format("15:04:05"))
	sb.WriteByte(' ')
	if r.Level != slog.LevelInfo {
		sb.WriteByte('[')
		sb.WriteString(r.Level.String())
		sb.WriteString("] ")
	}
	sb.WriteString(sanitizeLogMessage(r.Message))
	prefix := sanitizeLogMessage(strings.Join(h.groups, "."))
	write := func(a slog.Attr) {
		a.Value = a.Value.Resolve()
		if a.Equal(slog.Attr{}) {
			return
		}
		sb.WriteByte(' ')
		if prefix != "" {
			sb.WriteString(prefix)
			sb.WriteByte('.')
		}
		sb.WriteString(sanitizeLogKey(a.Key))
		sb.WriteByte('=')
		sb.WriteString(quoteIfNeeded(a.Value.String()))
	}
	for _, a := range h.attrs {
		write(a)
	}
	r.Attrs(func(a slog.Attr) bool { write(a); return true })
	sb.WriteByte('\n')

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := io.WriteString(h.w, sb.String())
	return err
}

func (h *compactHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	c := *h
	c.attrs = append(append([]slog.Attr{}, h.attrs...), attrs...)
	return &c
}

func (h *compactHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	c := *h
	c.groups = append(append([]string{}, h.groups...), name)
	return &c
}

func hasControlRunes(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) {
			return true
		}
	}
	return false
}

func escapeControlRunes(s string, escapeQuotesAndSlashes bool) string {
	var sb strings.Builder
	sb.Grow(len(s) + 8)
	for _, r := range s {
		switch {
		case escapeQuotesAndSlashes && r == '\\':
			sb.WriteString(`\\`)
		case escapeQuotesAndSlashes && r == '"':
			sb.WriteString(`\"`)
		case r == '\n':
			sb.WriteString(`\n`)
		case r == '\r':
			sb.WriteString(`\r`)
		case r == '\t':
			sb.WriteString(`\t`)
		case r == 0x1b:
			sb.WriteString(`\x1b`)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&sb, `\x%02x`, r)
		case r >= 0x80 && r <= 0x9f:
			fmt.Fprintf(&sb, `\u%04x`, r)
		default:
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

func sanitizeLogMessage(s string) string {
	if !hasControlRunes(s) {
		return s
	}
	return escapeControlRunes(s, false)
}

func sanitizeLogKey(k string) string {
	k = sanitizeLogMessage(k)
	if strings.ContainsAny(k, " =\"") {
		k = strings.Map(func(r rune) rune {
			if r == ' ' || r == '=' || r == '"' {
				return '_'
			}
			return r
		}, k)
	}
	return k
}

func quoteIfNeeded(s string) string {
	ctrl := hasControlRunes(s)
	if s == "" || ctrl || strings.ContainsAny(s, " \t\"=") {
		return `"` + escapeControlRunes(s, true) + `"`
	}
	return s
}
