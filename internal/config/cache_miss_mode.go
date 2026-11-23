package config

import (
	"os"
	"strings"
)

const (
	// CacheMissModeShadow runs cache-miss detection silently in the background,
	// suppresses user push notifications and client-facing orange miss_streak badges,
	// and writes the observation report to Private Docs.
	CacheMissModeShadow = "shadow"
	// CacheMissModeLive enables user-facing push alerts and client miss_streak badges
	// while still recording background audit telemetry.
	CacheMissModeLive = "live"
	// CacheMissModeOff disables both user-facing alerts/badges and background auditing.
	CacheMissModeOff = "off"
)

// CacheMissAlertMode resolves the active cache-miss alert delivery mode.
// Priority:
//  1. CACHE_MISS_ALERT_MODE ("shadow" | "live" | "off")
//  2. Legacy CACHE_MISS_ALERT_ENABLED ("false"/"0" -> "off", "shadow"/"audit" -> "shadow", "true"/"1"/"live" -> "live")
//  3. Default: "shadow" (background audit only, zero user push or capsule badge interruption)
func CacheMissAlertMode() string {
	if raw := strings.ToLower(strings.TrimSpace(os.Getenv("CACHE_MISS_ALERT_MODE"))); raw != "" {
		switch raw {
		case "off", "0", "false", "no", "disabled":
			return CacheMissModeOff
		case "live", "notify", "push", "on", "1", "true", "yes":
			return CacheMissModeLive
		case "shadow", "audit", "dry-run", "dryrun":
			return CacheMissModeShadow
		}
	}
	if raw := strings.ToLower(strings.TrimSpace(os.Getenv("CACHE_MISS_ALERT_ENABLED"))); raw != "" {
		switch raw {
		case "0", "false", "off", "no", "disabled":
			return CacheMissModeOff
		case "shadow", "audit", "dry-run", "dryrun":
			return CacheMissModeShadow
		case "live", "notify", "push", "1", "true", "yes", "on":
			return CacheMissModeLive
		}
	}
	return CacheMissModeShadow
}

// IsCacheMissAlertLive reports whether user-facing cache-miss push alerts and
// in-app Context capsule miss_streak warning badges are enabled.
func IsCacheMissAlertLive() bool {
	return CacheMissAlertMode() == CacheMissModeLive
}
