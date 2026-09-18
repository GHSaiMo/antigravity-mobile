package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const adminTokenFileName = "admin_token"

// GetAdminToken returns the currently configured admin token from the environment.
func GetAdminToken() string {
	tok := strings.TrimSpace(os.Getenv("MULTIGRAVITY_ADMIN_TOKEN"))
	if tok != "" {
		return tok
	}
	return strings.TrimSpace(os.Getenv("ADMIN_TOKEN"))
}

// DefaultAdminTokenPath is where a generated admin token is stored.
func DefaultAdminTokenPath() string {
	dir := DefaultDataDir()
	newPath := filepath.Join(dir, adminTokenFileName)
	if home, err := os.UserHomeDir(); err == nil {
		oldPath := filepath.Join(home, ".antigravity-mobile", adminTokenFileName)
		if _, err := os.Stat(newPath); os.IsNotExist(err) {
			if _, errOld := os.Stat(oldPath); errOld == nil {
				return oldPath
			}
		}
	}
	return newPath
}

// EnsureAdminToken loads ADMIN_TOKEN from the environment or a 0600 file.
// When a tunnel is enabled and no token exists, a random token is generated so
// local tools such as `make pair` can authenticate without trusting loopback.
func EnsureAdminToken(tunnelEnabled bool) (path string, generated bool, err error) {
	if strings.TrimSpace(os.Getenv("ADMIN_TOKEN")) != "" {
		return "", false, nil
	}

	path = DefaultAdminTokenPath()
	if b, readErr := os.ReadFile(path); readErr == nil {
		if tok := strings.TrimSpace(string(b)); tok != "" {
			if setErr := os.Setenv("ADMIN_TOKEN", tok); setErr != nil {
				return path, false, setErr
			}
			return path, false, nil
		}
	}

	if !tunnelEnabled {
		return "", false, nil
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", false, fmt.Errorf("generate admin token: %w", err)
	}
	tok := hex.EncodeToString(raw)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return "", false, err
	}
	if err := os.WriteFile(path, []byte(tok+"\n"), 0600); err != nil {
		return "", false, err
	}
	if err := os.Setenv("ADMIN_TOKEN", tok); err != nil {
		return path, true, err
	}
	return path, true, nil
}
