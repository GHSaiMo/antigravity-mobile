package auth

import "log/slog"

// Audit events are emitted as structured records: "audit <EVENT> k=v ...".
// Failures and rate limits log at WARN so they survive MULTIGRAVITY_LOG_LEVEL=warn.

func auditInfo(event string, args ...any)  { slog.Info("audit "+event, args...) }
func auditWarn(event string, args ...any)  { slog.Warn("audit "+event, args...) }
func auditError(event string, args ...any) { slog.Error("audit "+event, args...) }
