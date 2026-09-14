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

// IsSafeFilePath validates that the resolved path is within an explicitly allowed directory.
// Uses a whitelist approach: only paths under the user's home .gemini/antigravity/brain/,
// .gemini/antigravity/conversations/, or the Antigravity workspace directories are permitted.
func IsSafeFilePath(path string) bool {
	clean := filepath.Clean(path)

	// Resolve symlinks to prevent symlink-based bypasses
	resolved, err := filepath.EvalSymlinks(clean)
	if err != nil {
		// If the file doesn't exist yet, use the cleaned path
		resolved = clean
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}

	// Whitelist: only these directory trees are allowed
	allowedPrefixes := []string{
		filepath.Join(home, ".gemini", "antigravity", "brain") + string(filepath.Separator),
		filepath.Join(home, ".gemini", "antigravity", "conversations") + string(filepath.Separator),
		filepath.Join(home, ".gemini", "antigravity", "annotations") + string(filepath.Separator),
		filepath.Join(home, "Projects") + string(filepath.Separator),
		filepath.Join(home, "Downloads") + string(filepath.Separator),
		filepath.Join(home, "Desktop") + string(filepath.Separator),
		filepath.Join(home, "Documents") + string(filepath.Separator),
		filepath.Join(home, "Pictures") + string(filepath.Separator),
		filepath.Clean(os.TempDir()) + string(filepath.Separator),
	}

	// Always block sensitive filenames even within allowed dirs
	base := strings.ToLower(filepath.Base(resolved))
	sensitiveNames := []string{"id_rsa", "id_ed25519", "id_ecdsa", "id_dsa", ".env", ".git-credentials", ".netrc"}
	for _, s := range sensitiveNames {
		if base == s || strings.HasPrefix(base, ".env.") {
			return false
		}
	}
	sensitiveExts := []string{".pem", ".key", ".p12", ".pfx", ".jks"}
	for _, ext := range sensitiveExts {
		if strings.HasSuffix(base, ext) {
			return false
		}
	}

	for _, prefix := range allowedPrefixes {
		if strings.HasPrefix(resolved, prefix) {
			return true
		}
	}

	return false
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

// HandleFileRaw streams binary or raw file contents with proper Content-Disposition and Range support.
// GET /api/v1/files/raw?uri=...&cascade_id=...
func (p *Proxy) HandleFileRaw(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
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

	filePath, err := ResolveLocalFilePath(uri, cascadeID)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to resolve file path: %v", err), http.StatusBadRequest)
		return
	}

	if !IsSafeFilePath(filePath) {
		http.Error(w, "access to file is restricted for security", http.StatusForbidden)
		return
	}

	fi, err := os.Stat(filePath)
	if err != nil {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}
	if fi.IsDir() {
		http.Error(w, "path is a directory", http.StatusBadRequest)
		return
	}
	if !fi.Mode().IsRegular() {
		http.Error(w, "not a regular file", http.StatusBadRequest)
		return
	}

	fileName := filepath.Base(filePath)
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".pptx":
		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.presentationml.presentation")
	case ".ppt":
		w.Header().Set("Content-Type", "application/vnd.ms-powerpoint")
	case ".docx":
		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
	case ".doc":
		w.Header().Set("Content-Type", "application/msword")
	case ".xlsx":
		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	case ".xls":
		w.Header().Set("Content-Type", "application/vnd.ms-excel")
	case ".pdf":
		w.Header().Set("Content-Type", "application/pdf")
	case ".html", ".htm":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; img-src data: https:;")
	case ".key":
		w.Header().Set("Content-Type", "application/x-iwork-keynote-sffkey")
	case ".png":
		w.Header().Set("Content-Type", "image/png")
	case ".jpg", ".jpeg":
		w.Header().Set("Content-Type", "image/jpeg")
	case ".gif":
		w.Header().Set("Content-Type", "image/gif")
	case ".webp":
		w.Header().Set("Content-Type", "image/webp")
	case ".svg":
		w.Header().Set("Content-Type", "image/svg+xml")
	case ".bmp":
		w.Header().Set("Content-Type", "image/bmp")
	case ".ico":
		w.Header().Set("Content-Type", "image/x-icon")
	}

	encodedName := url.PathEscape(fileName)
	// Force download for potentially dangerous file types to prevent stored XSS
	disposition := "inline"
	if ext == ".html" || ext == ".htm" || ext == ".svg" {
		disposition = "attachment"
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("%s; filename*=UTF-8''%s", disposition, encodedName))

	http.ServeFile(w, r, filePath)
}

