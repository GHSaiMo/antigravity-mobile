package proxy

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var (
	activeAntigravityDataDirMu sync.RWMutex
	activeAntigravityDataDir   string
)

// setActiveAntigravityDataDir records the app data directory (e.g. ~/.gemini/antigravity or
// ~/.gemini/antigravity-cli) associated with the currently connected upstream instance.
func setActiveAntigravityDataDir(dir string) {
	dir = strings.TrimSpace(dir)
	if dir != "" {
		dir = filepath.Clean(dir)
	}
	activeAntigravityDataDirMu.Lock()
	activeAntigravityDataDir = dir
	activeAntigravityDataDirMu.Unlock()
}

func isPathWithinHome(home, path string) bool {
	if home == "" || path == "" {
		return false
	}
	rel, err := filepath.Rel(filepath.Clean(home), filepath.Clean(path))
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func explicitAntigravityDataDirEnv() string {
	if v := strings.TrimSpace(os.Getenv("MGY_ANTIGRAVITY_DATA_DIR")); v != "" {
		return filepath.Clean(v)
	}
	if v := strings.TrimSpace(os.Getenv("MULTIGRAVITY_ANTIGRAVITY_DATA_DIR")); v != "" {
		return filepath.Clean(v)
	}
	return ""
}

func scopedAntigravityAppDataDirEnv(home string) string {
	v := strings.TrimSpace(os.Getenv("ANTIGRAVITY_APP_DATA_DIR"))
	if v == "" {
		return ""
	}
	cleaned := filepath.Clean(v)
	if home != "" && !isPathWithinHome(home, cleaned) {
		return ""
	}
	return cleaned
}

func getActiveAntigravityDataDirForHome(home string) string {
	activeAntigravityDataDirMu.RLock()
	dir := activeAntigravityDataDir
	activeAntigravityDataDirMu.RUnlock()
	if dir == "" {
		return ""
	}
	// Ignore a previously discovered directory that belongs to a different $HOME
	// (e.g. unit tests switching HOME via t.Setenv).
	if home != "" && !isPathWithinHome(home, dir) {
		return ""
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return ""
	}
	return dir
}

// antigravityHomeCandidatesForHome returns the ordered, symlink-deduplicated list of candidate
// Antigravity data directories:
//  1. MGY_ANTIGRAVITY_DATA_DIR / MULTIGRAVITY_ANTIGRAVITY_DATA_DIR (explicit user override)
//  2. ANTIGRAVITY_APP_DATA_DIR (when scoped under the current $HOME)
//  3. The active upstream's discovered data dir (when under the current $HOME and existing)
//  4. ~/.gemini/antigravity (Desktop / default)
//  5. ~/.gemini/antigravity-cli (Antigravity CLI / agy fallback)
func antigravityHomeCandidatesForHome(home string) []string {
	var raw []string
	if customDir := explicitAntigravityDataDirEnv(); customDir != "" {
		raw = append(raw, customDir)
	}
	if envDir := scopedAntigravityAppDataDirEnv(home); envDir != "" {
		raw = append(raw, envDir)
	}
	if activeDir := getActiveAntigravityDataDirForHome(home); activeDir != "" {
		raw = append(raw, activeDir)
	}
	if home != "" {
		raw = append(raw,
			filepath.Join(home, ".gemini", "antigravity"),
			filepath.Join(home, ".gemini", "antigravity-cli"),
		)
	}

	seen := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw))
	for _, dir := range raw {
		key := filepath.Clean(dir)
		if resolved, err := filepath.EvalSymlinks(dir); err == nil && resolved != "" {
			key = filepath.Clean(resolved)
		}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, dir)
	}
	return out
}

func antigravityHomeCandidates() []string {
	home, _ := os.UserHomeDir()
	return antigravityHomeCandidatesForHome(home)
}

// resolveAntigravityHomeDirForHome returns the primary Antigravity data directory.
// It honors MGY_ANTIGRAVITY_DATA_DIR first, then a $HOME-scoped ANTIGRAVITY_APP_DATA_DIR,
// the active discovered data dir, and ~/.gemini/antigravity, falling back to
// ~/.gemini/antigravity-cli when ~/.gemini/antigravity does not exist.
func resolveAntigravityHomeDirForHome(home string) string {
	if customDir := explicitAntigravityDataDirEnv(); customDir != "" {
		return customDir
	}
	if envDir := scopedAntigravityAppDataDirEnv(home); envDir != "" {
		return envDir
	}
	candidates := antigravityHomeCandidatesForHome(home)
	for _, dir := range candidates {
		if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
			return dir
		}
	}
	if len(candidates) > 0 {
		return candidates[0]
	}
	return ""
}

// resolveAntigravityHomeDir resolves the active Antigravity data root for the current user.
func resolveAntigravityHomeDir() string {
	home, _ := os.UserHomeDir()
	return resolveAntigravityHomeDirForHome(home)
}

// resolveAntigravitySubPath resolves a relative path under the Antigravity data directory.
// It returns the first candidate path that already exists on disk (allowing seamless fallback
// to ~/.gemini/antigravity-cli even when ~/.gemini/antigravity exists without that file),
// or falls back to joining elems under resolveAntigravityHomeDir().
func resolveAntigravitySubPath(elems ...string) string {
	candidates := antigravityHomeCandidates()
	for _, base := range candidates {
		parts := make([]string, 0, 1+len(elems))
		parts = append(parts, base)
		parts = append(parts, elems...)
		p := filepath.Join(parts...)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	base := resolveAntigravityHomeDir()
	if base == "" {
		return ""
	}
	parts := make([]string, 0, 1+len(elems))
	parts = append(parts, base)
	parts = append(parts, elems...)
	return filepath.Join(parts...)
}

// resolveAntigravityBrainPath safely joins sub under candidate brain/ directories using joinUnder,
// preferring the first candidate where the target already exists.
func resolveAntigravityBrainPath(home, sub string) (string, error) {
	candidates := antigravityHomeCandidatesForHome(home)
	var firstValid string
	for _, base := range candidates {
		p, err := joinUnder(filepath.Join(base, "brain"), sub)
		if err != nil {
			return "", err
		}
		if firstValid == "" {
			firstValid = p
		}
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	if primary := resolveAntigravityHomeDirForHome(home); primary != "" {
		return joinUnder(filepath.Join(primary, "brain"), sub)
	}
	if firstValid != "" {
		return firstValid, nil
	}
	return joinUnder(filepath.Join(home, ".gemini", "antigravity", "brain"), sub)
}
