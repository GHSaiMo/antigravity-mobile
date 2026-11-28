package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"antigravity-mobile/internal/config"
)

const maxLaunchpadShortcuts = 4

// LaunchpadShortcut represents one pinned slot in the New Chat 2x2 quick-launch grid.
type LaunchpadShortcut struct {
	Kind     string `json:"kind"` // "workflow" | "slash" | "skill" | "prompt"
	Ref      string `json:"ref"`
	Title    string `json:"title,omitempty"`
	Subtitle string `json:"subtitle,omitempty"`
	SVGKey   string `json:"svgKey,omitempty"`
	Tint     string `json:"tint,omitempty"`
}

// LaunchpadShortcutsStore is the persisted JSON structure in launchpad_shortcuts.json.
type LaunchpadShortcutsStore struct {
	Version    int                 `json:"version"`
	UpdatedAt  string              `json:"updatedAt,omitempty"`
	Customized bool                `json:"customized"`
	Items      []LaunchpadShortcut `json:"items"`
}

// LaunchpadShortcutsResponse is returned by /gateway/launchpad-shortcuts.
type LaunchpadShortcutsResponse struct {
	Customized bool                `json:"customized"`
	Items      []LaunchpadShortcut `json:"items"`
	UpdatedAt  string              `json:"updatedAt,omitempty"`
}

var launchpadShortcutsMu sync.RWMutex

func getLaunchpadShortcutsFilePath() string {
	return filepath.Join(config.GetDataDir(), "launchpad_shortcuts.json")
}

func sanitizeLaunchpadTint(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "indigo", "blue", "teal", "cyan", "amber", "emerald", "coral":
		return strings.ToLower(strings.TrimSpace(raw))
	case "green":
		return "emerald"
	case "orange":
		return "amber"
	case "red":
		return "coral"
	case "purple", "pink", "magenta", "fuchsia":
		return "indigo"
	default:
		return ""
	}
}

func normalizeAndValidateLaunchpadShortcuts(raw []LaunchpadShortcut) ([]LaunchpadShortcut, error) {
	if len(raw) > maxLaunchpadShortcuts {
		return nil, fmt.Errorf("too many launchpad shortcuts (max %d)", maxLaunchpadShortcuts)
	}
	out := make([]LaunchpadShortcut, 0, len(raw))
	seen := make(map[string]bool, len(raw))

	for i, item := range raw {
		kind := strings.ToLower(strings.TrimSpace(item.Kind))
		switch kind {
		case "workflow", "slash", "skill", "prompt":
		default:
			return nil, fmt.Errorf("shortcut at index %d has invalid kind %q", i, item.Kind)
		}

		ref := strings.TrimSpace(item.Ref)
		if kind == "slash" || kind == "skill" {
			ref = strings.ToLower(strings.TrimLeft(ref, "/"))
		}
		if ref == "" {
			return nil, fmt.Errorf("shortcut at index %d has empty ref", i)
		}

		dedupKey := kind + ":" + strings.ToLower(ref)
		if seen[dedupKey] {
			continue
		}
		seen[dedupKey] = true

		out = append(out, LaunchpadShortcut{
			Kind:     kind,
			Ref:      ref,
			Title:    strings.TrimSpace(item.Title),
			Subtitle: strings.TrimSpace(item.Subtitle),
			SVGKey:   strings.TrimSpace(item.SVGKey),
			Tint:     sanitizeLaunchpadTint(item.Tint),
		})
	}
	return out, nil
}

func loadLaunchpadShortcuts() (LaunchpadShortcutsResponse, error) {
	launchpadShortcutsMu.RLock()
	defer launchpadShortcutsMu.RUnlock()

	filePath := getLaunchpadShortcutsFilePath()
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return LaunchpadShortcutsResponse{
				Customized: false,
				Items:      []LaunchpadShortcut{},
			}, nil
		}
		return LaunchpadShortcutsResponse{}, fmt.Errorf("failed to read launchpad shortcuts: %w", err)
	}

	var store LaunchpadShortcutsStore
	if err := json.Unmarshal(data, &store); err != nil {
		return LaunchpadShortcutsResponse{}, fmt.Errorf("failed to parse launchpad shortcuts: %w", err)
	}
	if store.Items == nil {
		store.Items = []LaunchpadShortcut{}
	}
	return LaunchpadShortcutsResponse{
		Customized: store.Customized && len(store.Items) > 0,
		Items:      store.Items,
		UpdatedAt:  store.UpdatedAt,
	}, nil
}

func saveLaunchpadShortcuts(raw []LaunchpadShortcut, customized *bool) (LaunchpadShortcutsResponse, error) {
	normalized, err := normalizeAndValidateLaunchpadShortcuts(raw)
	if err != nil {
		return LaunchpadShortcutsResponse{}, err
	}

	isCustom := len(normalized) > 0
	if customized != nil && !*customized {
		isCustom = false
		normalized = []LaunchpadShortcut{}
	}

	launchpadShortcutsMu.Lock()
	defer launchpadShortcutsMu.Unlock()

	filePath := getLaunchpadShortcutsFilePath()
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return LaunchpadShortcutsResponse{}, fmt.Errorf("failed to create data directory: %w", err)
	}

	updatedAt := time.Now().UTC().Format(time.RFC3339)
	store := LaunchpadShortcutsStore{
		Version:    1,
		UpdatedAt:  updatedAt,
		Customized: isCustom,
		Items:      normalized,
	}

	buf, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return LaunchpadShortcutsResponse{}, fmt.Errorf("failed to encode launchpad shortcuts: %w", err)
	}

	tmpFile := fmt.Sprintf("%s.tmp.%d", filePath, time.Now().UnixNano())
	if err := os.WriteFile(tmpFile, buf, 0644); err != nil {
		return LaunchpadShortcutsResponse{}, fmt.Errorf("failed to write temp launchpad shortcuts file: %w", err)
	}
	if err := os.Rename(tmpFile, filePath); err != nil {
		_ = os.Remove(tmpFile)
		return LaunchpadShortcutsResponse{}, fmt.Errorf("failed to replace launchpad shortcuts file: %w", err)
	}

	return LaunchpadShortcutsResponse{
		Customized: isCustom,
		Items:      normalized,
		UpdatedAt:  updatedAt,
	}, nil
}

func resetLaunchpadShortcuts() (LaunchpadShortcutsResponse, error) {
	launchpadShortcutsMu.Lock()
	defer launchpadShortcutsMu.Unlock()

	filePath := getLaunchpadShortcutsFilePath()
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return LaunchpadShortcutsResponse{}, fmt.Errorf("failed to remove launchpad shortcuts file: %w", err)
	}

	return LaunchpadShortcutsResponse{
		Customized: false,
		Items:      []LaunchpadShortcut{},
	}, nil
}

// HandleLaunchpadShortcuts handles GET, PUT/POST, and DELETE /gateway/launchpad-shortcuts.
func (p *Proxy) HandleLaunchpadShortcuts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		resp, err := loadLaunchpadShortcuts()
		if err != nil {
			writeJSONError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeWorkflowTemplatesJSON(w, http.StatusOK, resp)

	case http.MethodPut, http.MethodPost:
		var req struct {
			Customized *bool               `json:"customized,omitempty"`
			Items      []LaunchpadShortcut `json:"items"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 64*1024)).Decode(&req); err != nil {
			writeJSONError(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
			return
		}
		if req.Items == nil {
			writeJSONError(w, "items array is required", http.StatusBadRequest)
			return
		}
		resp, err := saveLaunchpadShortcuts(req.Items, req.Customized)
		if err != nil {
			writeJSONError(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeWorkflowTemplatesJSON(w, http.StatusOK, resp)

	case http.MethodDelete:
		resp, err := resetLaunchpadShortcuts()
		if err != nil {
			writeJSONError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeWorkflowTemplatesJSON(w, http.StatusOK, resp)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}
