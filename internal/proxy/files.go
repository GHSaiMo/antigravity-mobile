package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
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
		return joinUnder(filepath.Join(home, ".gemini", "antigravity", "brain"), sub)
	}

	// 2. file:// protocol
	if strings.HasPrefix(clean, "file://") {
		clean = strings.TrimPrefix(clean, "file://")
	}

	// 3. Brain path detection: if it points to .gemini/antigravity/brain or /brain/
	if idx := strings.Index(clean, "/brain/"); idx != -1 {
		sub := clean[idx+len("/brain/"):]
		return joinUnder(filepath.Join(home, ".gemini", "antigravity", "brain"), sub)
	}

	// 4. Bare filename like "implementation_plan.md" or "walkthrough.md" with cascadeID
	if !strings.Contains(clean, "/") && cascadeID != "" {
		planPath, err := joinUnder(filepath.Join(home, ".gemini", "antigravity", "brain"), filepath.Join(cascadeID, clean))
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(planPath); err == nil {
			return planPath, nil
		}
		return planPath, nil
	}

	// 5. If clean path starts with "~"
	if strings.HasPrefix(clean, "~/") {
		clean = filepath.Join(home, clean[2:])
	}

	// 6. Absolute path on local filesystem
	if filepath.IsAbs(clean) {
		cleanPath := filepath.Clean(clean)
		if !IsSafeFilePath(cleanPath) {
			return "", fmt.Errorf("access to file is restricted")
		}
		return cleanPath, nil
	}

	// 7. If relative path and cascadeID is provided, check brain directory first
	if cascadeID != "" {
		planPath, err := joinUnder(filepath.Join(home, ".gemini", "antigravity", "brain"), filepath.Join(cascadeID, clean))
		if err == nil {
			if _, err := os.Stat(planPath); err == nil {
				if !IsSafeFilePath(planPath) {
					return "", fmt.Errorf("access to file is restricted")
				}
				return planPath, nil
			}
		}
	}

	cleanPath := filepath.Clean(clean)
	if !IsSafeFilePath(cleanPath) {
		return "", fmt.Errorf("access to file is restricted")
	}
	return cleanPath, nil
}

func joinUnder(root, extra string) (string, error) {
	root = filepath.Clean(root)
	cleaned := filepath.Clean(extra)
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes allowlisted root")
	}
	full := filepath.Join(root, cleaned)
	rel, err := filepath.Rel(root, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes allowlisted root")
	}
	return full, nil
}

var (
	workspaceRootsOnce sync.Once
	workspaceRoots     []string
)

// AllowedWorkspaceRoots returns directory prefixes that may be served via the file APIs.
// ~/.gemini/antigravity is always included. If ALLOWED_WORKSPACE_ROOTS is set (colon-separated),
// those roots replace the default. Otherwise the paired Mac's home directory is allowed;
// IsSafeFilePath still rejects credential stores and secret filenames.
func AllowedWorkspaceRoots() []string {
	workspaceRootsOnce.Do(func() {
		home, err := os.UserHomeDir()
		if err != nil {
			return
		}
		sep := string(filepath.Separator)
		always := []string{filepath.Join(home, ".gemini", "antigravity") + sep}
		if extra := strings.TrimSpace(os.Getenv("ALLOWED_WORKSPACE_ROOTS")); extra != "" {
			workspaceRoots = always
			for _, raw := range strings.Split(extra, ":") {
				raw = strings.TrimSpace(raw)
				if raw == "" {
					continue
				}
				if strings.HasPrefix(raw, "~/") {
					raw = filepath.Join(home, raw[2:])
				}
				workspaceRoots = append(workspaceRoots, filepath.Clean(raw)+sep)
			}
			return
		}
		workspaceRoots = append(always, home+sep)
	})
	return workspaceRoots
}

func resetWorkspaceRootsForTest() {
	workspaceRootsOnce = sync.Once{}
	workspaceRoots = nil
}

// IsSafeFilePath validates that the resolved path is within an allowed directory
// and is not a credential or secret file. Default allowlist is the user's home
// (agent-generated PPTX/PDF/HTML/images live on Desktop, Downloads, Websites, etc.).
func IsSafeFilePath(path string) bool {
	clean := filepath.Clean(path)

	resolved, err := filepath.EvalSymlinks(clean)
	if err != nil {
		resolved = clean
	}

	slashPath := filepath.ToSlash(resolved)
	sensitiveDirs := []string{
		"/.ssh/", "/.gnupg/", "/.aws/", "/.docker/", "/Library/Keychains/",
		"/Library/Application Support/",
		"/.kube/", "/.config/gcloud/",
		"/.multigravity/",
		"/.antigravity-mobile/",
		"/.acme.sh/", "/.lego/", "/.certbot/",
	}
	for _, sd := range sensitiveDirs {
		if strings.Contains(slashPath, sd) || strings.HasSuffix(slashPath, strings.TrimSuffix(sd, "/")) {
			return false
		}
	}

	base := strings.ToLower(filepath.Base(resolved))
	sensitiveNames := []string{
		"id_rsa", "id_ed25519", "id_ecdsa", "id_dsa",
		".env", ".envrc", ".git-credentials", ".netrc", ".dockercfg", ".npmrc", ".pypirc",
		".bash_history", ".zsh_history",
		"oauth_creds.json", "jetski-standalone-oauth-token", "google_accounts.json",
		"auth_store.json", "credentials.db", "credentials.json",
		"server.key", "client.key", "ca.key", "tls.key", "ssl.key", "privkey.key",
		"privkey.pem", "domain.key", "host.key", "cert.key", "root.key",
	}
	for _, s := range sensitiveNames {
		if base == s || strings.HasPrefix(base, ".env.") {
			return false
		}
	}
	if strings.HasPrefix(base, "id_rsa") || strings.HasPrefix(base, "id_ed25519") {
		return false
	}
	// .key is not blanket-blocked: Keynote decks use that extension.
	for _, ext := range []string{".pem", ".p12", ".pfx", ".jks"} {
		if strings.HasSuffix(base, ext) {
			return false
		}
	}
	if strings.HasSuffix(base, ".key") {
		sensitiveKeyKeywords := []string{
			"id_", "private", "secret", "tls", "ssl", "server", "client",
			"cert", "ca", "rsa", "ecdsa", "ed25519", "dsa", "priv", "domain", "host",
		}
		for _, kw := range sensitiveKeyKeywords {
			if strings.Contains(base, kw) {
				return false
			}
		}
	}

	for _, prefix := range AllowedWorkspaceRoots() {
		if strings.HasPrefix(resolved, prefix) {
			return true
		}
	}
	return false
}

func openRegularNoFollow(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
}

// isPrivateKeyFileHeader probes the start of a regular file to detect PEM/OpenSSH private key banners.
func isPrivateKeyFileHeader(f *os.File) bool {
	buf := make([]byte, 256)
	n, err := f.ReadAt(buf, 0)
	if err != nil && err != io.EOF {
		return false
	}
	sample := string(buf[:n])
	return strings.Contains(sample, "-----BEGIN ") ||
		strings.Contains(sample, "PRIVATE KEY")
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

	if resolved, err := filepath.EvalSymlinks(filePath); err == nil {
		filePath = resolved
	}
	if !IsSafeFilePath(filePath) {
		return nil, fmt.Errorf("access to file is restricted")
	}

	f, err := openRegularNoFollow(filePath)
	if err != nil {
		return nil, fmt.Errorf("file not found")
	}
	defer f.Close()
	if strings.HasSuffix(strings.ToLower(filePath), ".key") && isPrivateKeyFileHeader(f) {
		return nil, fmt.Errorf("access to file is restricted")
	}
	fi, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("file not found")
	}
	if fi.IsDir() || !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("not a regular file")
	}
	if fi.Size() > maxReadSizeBytes {
		return nil, fmt.Errorf("file size exceeds 10MB limit")
	}

	contentBytes, err := io.ReadAll(io.LimitReader(f, maxReadSizeBytes+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read file")
	}

	res := &FileContentResult{
		URI:      "file://" + filePath,
		Filename: filepath.Base(filePath),
		Content:  string(contentBytes),
	}

	// Check companion metadata file
	metaFile := filePath + ".metadata.json"
	if IsSafeFilePath(metaFile) {
		if metaBytes, err := os.ReadFile(metaFile); err == nil {
			var meta ArtifactMetadata
			if err := json.Unmarshal(metaBytes, &meta); err == nil {
				res.Summary = strings.TrimSpace(meta.Summary)
				res.RequestFeedback = meta.RequestFeedback
				res.UserFacing = meta.UserFacing
			}
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

	if resolved, err := filepath.EvalSymlinks(filePath); err == nil {
		filePath = resolved
	}
	if !IsSafeFilePath(filePath) {
		log.Printf("[Files] denied raw download path=%s uri=%q", filePath, uri)
		http.Error(w, "access to file is restricted", http.StatusForbidden)
		return
	}

	f, err := openRegularNoFollow(filePath)
	if err != nil {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}
	defer f.Close()
	if strings.HasSuffix(strings.ToLower(filePath), ".key") && isPrivateKeyFileHeader(f) {
		log.Printf("[Files] denied raw download of private key header in %s", filePath)
		http.Error(w, "access to file is restricted", http.StatusForbidden)
		return
	}
	fi, err := f.Stat()
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
	case ".json":
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
	case ".log", ".txt":
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	case ".csv":
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	case ".md", ".markdown":
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	case ".xml":
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	case ".yaml", ".yml":
		w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
	case ".zip":
		w.Header().Set("Content-Type", "application/zip")
	case ".tar":
		w.Header().Set("Content-Type", "application/x-tar")
	case ".gz":
		w.Header().Set("Content-Type", "application/gzip")
	case ".mp3":
		w.Header().Set("Content-Type", "audio/mpeg")
	case ".wav":
		w.Header().Set("Content-Type", "audio/wav")
	case ".m4a":
		w.Header().Set("Content-Type", "audio/mp4")
	case ".mp4":
		w.Header().Set("Content-Type", "video/mp4")
	case ".mov":
		w.Header().Set("Content-Type", "video/quicktime")
	}

	encodedName := url.PathEscape(fileName)
	// Force download for potentially dangerous file types to prevent stored XSS
	disposition := "inline"
	if ext == ".html" || ext == ".htm" || ext == ".svg" {
		disposition = "attachment"
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("%s; filename=%q; filename*=UTF-8''%s", disposition, fileName, encodedName))

	http.ServeContent(w, r, fileName, fi.ModTime(), f)
}
