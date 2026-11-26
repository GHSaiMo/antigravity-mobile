package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"antigravity-mobile/internal/config"
)

const (
	backgroundCheckInterval  = 12 * time.Hour
	interactiveCheckInterval = 15 * time.Minute
	backgroundStartupDelay   = 2 * time.Second
	backgroundFetchTimeout   = 6 * time.Second
)

type updateCheckCache struct {
	CheckedAt time.Time `json:"checked_at"`
	LatestTag string    `json:"latest_tag"`
}

// CheckLatestCached returns the latest release tag using a short-lived local cache (15m)
// unless force is true. If a live refresh fails, any previously cached tag is still returned
// alongside the error so callers can degrade gracefully offline.
func CheckLatestCached(ctx context.Context, currentVersion string, force bool, opts Options) (string, time.Time, error) {
	cachePath := filepath.Join(config.GetDataDir(), "update_check.json")
	return checkLatestWithCache(ctx, currentVersion, cachePath, time.Now(), interactiveCheckInterval, force, func(c context.Context) (string, error) {
		o := opts
		if o.CurrentVersion == "" {
			o.CurrentVersion = currentVersion
		}
		return FetchLatestTag(c, o)
	})
}

// SaveCachedLatestTag persists the latest known tag into the local update check cache.
func SaveCachedLatestTag(latestTag string) {
	tag, err := EnsureTag(latestTag)
	if err != nil {
		return
	}
	cachePath := filepath.Join(config.GetDataDir(), "update_check.json")
	saveUpdateCheckCache(cachePath, updateCheckCache{
		CheckedAt: time.Now().UTC(),
		LatestTag: tag,
	})
}

func checkLatestWithCache(
	ctx context.Context,
	currentVersion string,
	cachePath string,
	now time.Time,
	ttl time.Duration,
	force bool,
	fetchFn func(context.Context) (string, error),
) (string, time.Time, error) {
	var cached updateCheckCache
	if data, err := os.ReadFile(cachePath); err == nil {
		_ = json.Unmarshal(data, &cached)
	}
	if !force && cached.LatestTag != "" && !cached.CheckedAt.IsZero() && now.Sub(cached.CheckedAt) < ttl {
		return cached.LatestTag, cached.CheckedAt, nil
	}
	if ctx.Err() != nil {
		return cached.LatestTag, cached.CheckedAt, ctx.Err()
	}

	latestTag, err := fetchFn(ctx)
	if err != nil || latestTag == "" {
		if err == nil {
			err = fmt.Errorf("empty latest release tag")
		}
		return cached.LatestTag, cached.CheckedAt, err
	}

	checkedAt := now.UTC()
	saveUpdateCheckCache(cachePath, updateCheckCache{
		CheckedAt: checkedAt,
		LatestTag: latestTag,
	})
	return latestTag, checkedAt, nil
}

// IsBackgroundCheckDisabled reports whether startup update checks are opted out via environment variables.
func IsBackgroundCheckDisabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("MULTIGRAVITY_NO_UPDATE_CHECK"))) {
	case "1", "true", "yes", "on":
		return true
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("MULTIGRAVITY_UPDATE_CHECK"))) {
	case "0", "false", "no", "off":
		return true
	}
	return false
}

// StartBackgroundUpdateCheck runs a single non-blocking, rate-limited version check shortly after gateway startup.
// Network failures are silently ignored so offline environments are never disturbed.
func StartBackgroundUpdateCheck(ctx context.Context, currentVersion string) {
	if IsBackgroundCheckDisabled() || strings.TrimSpace(currentVersion) == "" {
		return
	}

	cachePath := filepath.Join(config.GetDataDir(), "update_check.json")
	go func() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(backgroundStartupDelay):
		}

		latestTag, hasUpdate := checkForUpdateWithCache(ctx, currentVersion, cachePath, time.Now(), func(c context.Context) (string, error) {
			fctx, cancel := context.WithTimeout(c, backgroundFetchTimeout)
			defer cancel()
			return FetchLatestTag(fctx, Options{CurrentVersion: currentVersion})
		})
		if hasUpdate {
			slog.Info(fmt.Sprintf("🆕 发现新版本 %s (当前 v%s)，执行 `mgy update` 即可一键升级", latestTag, NormalizeVersion(currentVersion)))
		}
	}()
}

func checkForUpdateWithCache(
	ctx context.Context,
	currentVersion string,
	cachePath string,
	now time.Time,
	fetchFn func(context.Context) (string, error),
) (string, bool) {
	var cached updateCheckCache
	if data, err := os.ReadFile(cachePath); err == nil {
		if err := json.Unmarshal(data, &cached); err == nil {
			if cached.LatestTag != "" && !cached.CheckedAt.IsZero() && now.Sub(cached.CheckedAt) < backgroundCheckInterval {
				return cached.LatestTag, IsNewer(cached.LatestTag, currentVersion)
			}
		}
	}

	if ctx.Err() != nil {
		return "", false
	}

	latestTag, err := fetchFn(ctx)
	if err != nil || latestTag == "" {
		return "", false
	}

	saveUpdateCheckCache(cachePath, updateCheckCache{
		CheckedAt: now.UTC(),
		LatestTag: latestTag,
	})
	return latestTag, IsNewer(latestTag, currentVersion)
}

func saveUpdateCheckCache(cachePath string, c updateCheckCache) {
	if cachePath == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(cachePath), 0700); err != nil {
		return
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return
	}
	tmp := cachePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return
	}
	if err := os.Rename(tmp, cachePath); err != nil {
		_ = os.Remove(tmp)
	}
}
