package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// FileContentResult represents the structured response of file reading.
type FileContentResult struct {
	URI             string `json:"uri"`
	Filename        string `json:"filename"`
	Content         string `json:"content"`
	Summary         string `json:"summary,omitempty"`
	RequestFeedback bool   `json:"request_feedback,omitempty"`
	UserFacing      bool   `json:"user_facing,omitempty"`
}

// ArtifactMetadata represents the JSON metadata file generated alongside artifacts.
type ArtifactMetadata struct {
	Summary         string `json:"summary"`
	RequestFeedback bool   `json:"requestFeedback"`
	UserFacing      bool   `json:"userFacing"`
	UpdatedAt       string `json:"updatedAt"`
}

const maxReadSizeBytes = 10 * 1024 * 1024 // 10MB limit

// ResolveLocalFilePath normalizes a URI, relative path, or artifact path to an absolute filesystem path.
func ResolveLocalFilePath(rawURI, cascadeID string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("unable to determine user home directory: %w", err)
	}

	clean := strings.TrimSpace(rawURI)

	// URL unescape if needed (e.g. file:///path/with%20space)
	if unescaped, err := url.PathUnescape(clean); err == nil {
		clean = unescaped
	}

	// 1. Artifact static route: /static/artifacts/<cascadeId>/<filename>
	if strings.HasPrefix(clean, "/static/artifacts/") {
		sub := strings.TrimPrefix(clean, "/static/artifacts/")
		return filepath.Join(home, ".gemini/antigravity/brain", filepath.Clean(sub)), nil
	}

	// 2. file:// protocol
	if strings.HasPrefix(clean, "file://") {
		clean = strings.TrimPrefix(clean, "file://")
	}

	// 3. Brain path detection: if it points to .gemini/antigravity/brain or /brain/
	if idx := strings.Index(clean, "/brain/"); idx != -1 {
		sub := clean[idx+len("/brain/"):]
		return filepath.Join(home, ".gemini/antigravity/brain", filepath.Clean(sub)), nil
	}

	// 4. Bare filename like "implementation_plan.md" or "walkthrough.md" with cascadeID
	if !strings.Contains(clean, "/") && cascadeID != "" {
		return filepath.Join(home, ".gemini/antigravity/brain", filepath.Clean(cascadeID), filepath.Clean(clean)), nil
	}

	// 5. If clean path starts with "~"
	if strings.HasPrefix(clean, "~/") {
		clean = filepath.Join(home, clean[2:])
	}

	// 6. Absolute path on local filesystem
	if filepath.IsAbs(clean) {
		return filepath.Clean(clean), nil
	}

	// 7. If relative path and cascadeID is provided, check brain directory first
	if cascadeID != "" {
		planPath := filepath.Join(home, ".gemini/antigravity/brain", filepath.Clean(cascadeID), filepath.Clean(clean))
		if _, err := os.Stat(planPath); err == nil {
			return planPath, nil
		}
	}

	return filepath.Clean(clean), nil
}

// IsSafeFilePath validates that the requested path is not in a sensitive system or credential location.
func IsSafeFilePath(path string) bool {
	clean := filepath.Clean(path)

	// Prevent path traversal
	if strings.Contains(clean, "..") {
		return false
	}

	home, err := os.UserHomeDir()
	if err == nil {
		sensitiveUserDirs := []string{
			filepath.Join(home, ".ssh"),
			filepath.Join(home, ".gnupg"),
			filepath.Join(home, ".aws"),
			filepath.Join(home, ".config/gcloud"),
			filepath.Join(home, ".kube"),
		}
		for _, s := range sensitiveUserDirs {
			if strings.HasPrefix(clean, s) {
				return false
			}
		}
	}

	// Disallow sensitive system directories
	if strings.HasPrefix(clean, "/etc") || strings.HasPrefix(clean, "/private/etc") || strings.HasPrefix(clean, "/var/root") {
		return false
	}

	// Sensitive file names
	base := strings.ToLower(filepath.Base(clean))
	if base == "id_rsa" || base == "id_ed25519" || base == ".env" || strings.HasSuffix(base, ".pem") || strings.HasSuffix(base, ".key") {
		return false
	}

	return true
}

// GetFileContent retrieves the file content and companion metadata if present.
func GetFileContent(rawURI, cascadeID string) (*FileContentResult, error) {
	if strings.TrimSpace(rawURI) == "" && strings.TrimSpace(cascadeID) != "" {
		rawURI = "implementation_plan.md"
	}

	filePath, err := ResolveLocalFilePath(rawURI, cascadeID)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve file path: %w", err)
	}

	if !IsSafeFilePath(filePath) {
		return nil, fmt.Errorf("access to file is restricted for security: %s", filePath)
	}

	fi, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("file not found: %s", filePath)
	}
	if fi.IsDir() {
		return nil, fmt.Errorf("path is a directory: %s", filePath)
	}
	if fi.Size() > maxReadSizeBytes {
		return nil, fmt.Errorf("file size exceeds 10MB limit: %d bytes", fi.Size())
	}

	contentBytes, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	res := &FileContentResult{
		URI:      "file://" + filePath,
		Filename: filepath.Base(filePath),
		Content:  string(contentBytes),
	}

	// Check companion metadata file
	metaFile := filePath + ".metadata.json"
	if metaBytes, err := os.ReadFile(metaFile); err == nil {
		var meta ArtifactMetadata
		if err := json.Unmarshal(metaBytes, &meta); err == nil {
			res.Summary = strings.TrimSpace(meta.Summary)
			res.RequestFeedback = meta.RequestFeedback
			res.UserFacing = meta.UserFacing
		}
	}

	return res, nil
}

// HandleFileContent handles GET /api/v1/files/content?uri=...&cascade_id=...
func (p *Proxy) HandleFileContent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	q := r.URL.Query()
	uri := strings.TrimSpace(q.Get("uri"))
	cascadeID := strings.TrimSpace(q.Get("cascade_id"))

	if uri == "" && cascadeID == "" {
		http.Error(w, "uri or cascade_id is required", http.StatusBadRequest)
		return
	}

	result, err := GetFileContent(uri, cascadeID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(result)
}
