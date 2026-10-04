package proxy

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"antigravity-mobile/internal/config"
)

// WorkspaceAliasesStore manages custom display aliases for workspaces and paths.
type WorkspaceAliasesStore struct {
	Version   int               `json:"version"`
	UpdatedAt string            `json:"updated_at,omitempty"`
	Aliases   map[string]string `json:"aliases"`
}

func getWorkspaceAliasesFilePath() string {
	return filepath.Join(config.GetDataDir(), "workspace_aliases.json")
}

func normalizeWorkspaceKey(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "file://") {
		if u, err := url.Parse(raw); err == nil {
			raw = u.Path
		}
	}
	if runtime.GOOS == "windows" {
		raw = strings.TrimPrefix(raw, "/")
	}
	cleaned := filepath.Clean(raw)
	return strings.TrimRight(cleaned, "/\\")
}

func loadWorkspaceAliases() map[string]string {
	workspaceAliasesMu.RLock()
	defer workspaceAliasesMu.RUnlock()

	filePath := getWorkspaceAliasesFilePath()
	data, err := os.ReadFile(filePath)
	if err != nil {
		return make(map[string]string)
	}

	var store WorkspaceAliasesStore
	if err := json.Unmarshal(data, &store); err != nil {
		slog.Warn(fmt.Sprintf("[Projects] Warning: failed to parse workspace aliases from %s", filePath), "err", err)
		return make(map[string]string)
	}

	if store.Aliases == nil {
		return make(map[string]string)
	}
	return store.Aliases
}

func saveWorkspaceAlias(rawKey string, alias string) error {
	normKey := normalizeWorkspaceKey(rawKey)
	if normKey == "" {
		return fmt.Errorf("invalid path or uri")
	}

	workspaceAliasesMu.Lock()
	defer workspaceAliasesMu.Unlock()

	filePath := getWorkspaceAliasesFilePath()
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	store := WorkspaceAliasesStore{
		Version: 1,
		Aliases: make(map[string]string),
	}

	if data, err := os.ReadFile(filePath); err == nil {
		_ = json.Unmarshal(data, &store)
		if store.Aliases == nil {
			store.Aliases = make(map[string]string)
		}
	}

	cleanAlias := strings.TrimSpace(alias)
	if cleanAlias == "" {
		delete(store.Aliases, normKey)
		if strings.HasPrefix(rawKey, "file://") {
			if u, err := url.Parse(rawKey); err == nil {
				delete(store.Aliases, normalizeWorkspaceKey(u.Path))
			}
		}
	} else {
		store.Aliases[normKey] = cleanAlias
	}

	store.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	buf, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode workspace aliases: %w", err)
	}

	tmpFile := filePath + ".tmp"
	if err := os.WriteFile(tmpFile, buf, 0644); err != nil {
		return fmt.Errorf("failed to write temp aliases file: %w", err)
	}

	if err := os.Rename(tmpFile, filePath); err != nil {
		return fmt.Errorf("failed to replace aliases file: %w", err)
	}

	return nil
}

// UpdateProjectAliasRequest represents payload to set/clear workspace alias.
type UpdateProjectAliasRequest struct {
	Path  string `json:"path"`
	Alias string `json:"alias"`
}

// HandleProjectAlias handles POST /gateway/projects/alias.
func (p *Proxy) HandleProjectAlias(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req UpdateProjectAliasRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	targetPath := strings.TrimSpace(req.Path)
	if targetPath == "" {
		http.Error(w, "Path cannot be empty", http.StatusBadRequest)
		return
	}

	if err := saveWorkspaceAlias(targetPath, req.Alias); err != nil {
		slog.Warn(fmt.Sprintf("[Proxy] Failed to save workspace alias for %s", targetPath), "err", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}
