package proxy

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
)

var errAccessRestricted = errors.New("access to file is restricted")

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
	UserFacing      *bool  `json:"userFacing"`
	UpdatedAt       string `json:"updatedAt"`
}

// IsUserFacing returns true unless UserFacing is explicitly set to false.
func (m *ArtifactMetadata) IsUserFacing() bool {
	if m == nil || m.UserFacing == nil {
		return true
	}
	return *m.UserFacing
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
		if runtime.GOOS == "windows" {
			if len(clean) > 2 && (clean[0] == '/' || clean[0] == '\\') && isWindowsDriveLetter(clean[1]) && clean[2] == ':' {
				clean = clean[1:]
			}
		}
	}
	if strings.Contains(clean, "%") {
		if unescaped, err := url.PathUnescape(clean); err == nil {
			clean = unescaped
		}
	}

	// 3. Brain path detection: if it points to .gemini/antigravity/brain or /brain/
	slashClean := filepath.ToSlash(clean)
	if idx := strings.Index(slashClean, "/brain/"); idx != -1 {
		sub := clean[idx+len("/brain/"):]
		return joinUnder(filepath.Join(home, ".gemini", "antigravity", "brain"), sub)
	}

	// 4. Bare filename like "implementation_plan.md" or "walkthrough.md" with cascadeID
	if !strings.ContainsAny(clean, "/\\") && cascadeID != "" {
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
	if strings.HasPrefix(clean, "~/") || strings.HasPrefix(clean, "~\\") {
		clean = filepath.Join(home, clean[2:])
	}

	// 6. Absolute path on local filesystem
	if filepath.IsAbs(clean) {
		cleanPath := filepath.Clean(clean)
		if !IsSafeReadableFilePath(cleanPath) {
			return "", errAccessRestricted
		}
		if _, err := os.Stat(cleanPath); err == nil {
			return cleanPath, nil
		}
		if fallback, ok := resolveMovedInboxAttachment(home, cleanPath, cascadeID); ok {
			return fallback, nil
		}
		return cleanPath, nil
	}

	// 7. If relative path and cascadeID is provided, check brain directory first
	if cascadeID != "" {
		planPath, err := joinUnder(filepath.Join(home, ".gemini", "antigravity", "brain"), filepath.Join(cascadeID, clean))
		if err == nil {
			if _, err := os.Stat(planPath); err == nil {
				if !IsSafeReadableFilePath(planPath) {
					return "", errAccessRestricted
				}
				return planPath, nil
			}
		}
	}

	cleanPath := filepath.Clean(clean)
	if !IsSafeReadableFilePath(cleanPath) {
		return "", errAccessRestricted
	}
	return cleanPath, nil
}

func isWindowsDriveLetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// resolveMovedInboxAttachment locates an Inbox attachment that Antigravity moved into
// ~/.gemini/antigravity/brain/<cascadeID>/.user_uploaded/<id16>-<name>.
func resolveMovedInboxAttachment(home, cleanPath, cascadeID string) (string, bool) {
	slashPath := filepath.ToSlash(cleanPath)
	inboxDir, err := AttachmentInboxDir()
	isInboxPath := strings.Contains(slashPath, "/Multigravity/Inbox/") ||
		(err == nil && inboxDir != "" && strings.HasPrefix(cleanPath, filepath.Clean(inboxDir)+string(filepath.Separator)))
	if !isInboxPath {
		return "", false
	}

	base := filepath.Base(cleanPath)
	if base == "" || base == "." || base == ".." {
		return "", false
	}
	brainRoot := filepath.Join(home, ".gemini", "antigravity", "brain")

	if cascadeID != "" {
		candidate, err := joinUnder(brainRoot, filepath.Join(cascadeID, ".user_uploaded", base))
		if err == nil {
			if st, err := os.Stat(candidate); err == nil && st.Mode().IsRegular() && IsSafeReadableFilePath(candidate) {
				return candidate, true
			}
		}
	}

	// Fallback across brain sessions when base carries the unique 16-hex content prefix.
	if len(base) > attachmentIDLen+1 && base[attachmentIDLen] == '-' && attachmentIDPattern.MatchString(base[:attachmentIDLen]) {
		entries, err := os.ReadDir(brainRoot)
		if err == nil {
			for _, e := range entries {
				if !e.IsDir() {
					continue
				}
				candidate, err := joinUnder(brainRoot, filepath.Join(e.Name(), ".user_uploaded", base))
				if err != nil {
					continue
				}
				if st, err := os.Stat(candidate); err == nil && st.Mode().IsRegular() && IsSafeReadableFilePath(candidate) {
					return candidate, true
				}
			}
		}
	}
	return "", false
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
	workspaceRootsMu    sync.RWMutex
	workspaceRootsHome  string
	workspaceRootsExtra string
	workspaceRoots      []string
	readableRootsTmpDir string
	readableFileRoots   []string
)

// evalSymlinksExistingPrefix resolves symlinks along the longest existing ancestor prefix of p,
// re-attaching any not-yet-created trailing path segments cleanly. This prevents parent-directory
// symlink escapes even before a new file or subdirectory is created.
func evalSymlinksExistingPrefix(p string) string {
	clean := filepath.Clean(p)
	if abs, err := filepath.Abs(clean); err == nil {
		clean = abs
	}
	if resolved, err := filepath.EvalSymlinks(clean); err == nil {
		return resolved
	}
	cur := clean
	var tail []string
	for {
		parent := filepath.Dir(cur)
		base := filepath.Base(cur)
		if parent == cur {
			break
		}
		tail = append(tail, base)
		cur = parent
		if resolvedAnc, err := filepath.EvalSymlinks(cur); err == nil {
			out := resolvedAnc
			for i := len(tail) - 1; i >= 0; i-- {
				out = filepath.Join(out, tail[i])
			}
			return out
		}
	}
	return clean
}

func appendNormalizedRootPrefix(dst []string, raw string) []string {
	sep := string(filepath.Separator)
	clean := filepath.Clean(raw)
	if abs, err := filepath.Abs(clean); err == nil {
		clean = abs
	}
	add := func(candidate string) {
		p := candidate
		if !strings.HasSuffix(p, sep) {
			p += sep
		}
		for _, existing := range dst {
			if existing == p {
				return
			}
		}
		dst = append(dst, p)
	}
	add(clean)
	if resolved := evalSymlinksExistingPrefix(clean); resolved != "" && resolved != clean {
		add(resolved)
	}
	return dst
}

// AllowedWorkspaceRoots returns directory prefixes that may be served via the file APIs.
// ~/.gemini/antigravity is always included. If ALLOWED_WORKSPACE_ROOTS is set,
// those roots replace the default. Otherwise the paired computer's home directory is allowed;
// IsSafeFilePath still rejects credential stores and secret filenames.
func AllowedWorkspaceRoots() []string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil
	}
	extra := strings.TrimSpace(os.Getenv("MULTIGRAVITY_ALLOWED_WORKSPACE_ROOTS"))
	if extra == "" {
		extra = strings.TrimSpace(os.Getenv("ALLOWED_WORKSPACE_ROOTS"))
	}

	workspaceRootsMu.RLock()
	if workspaceRoots != nil && workspaceRootsHome == home && workspaceRootsExtra == extra {
		cached := workspaceRoots
		workspaceRootsMu.RUnlock()
		return cached
	}
	workspaceRootsMu.RUnlock()

	var roots []string
	roots = appendNormalizedRootPrefix(roots, filepath.Join(home, ".gemini", "antigravity"))
	if extra != "" {
		for _, raw := range filepath.SplitList(extra) {
			raw = strings.TrimSpace(raw)
			if raw == "" {
				continue
			}
			if raw == "~" {
				raw = home
			} else if strings.HasPrefix(raw, "~/") || strings.HasPrefix(raw, "~\\") {
				raw = filepath.Join(home, raw[2:])
			}
			roots = appendNormalizedRootPrefix(roots, raw)
		}
	} else {
		roots = appendNormalizedRootPrefix(roots, home)
	}

	workspaceRootsMu.Lock()
	workspaceRootsHome = home
	workspaceRootsExtra = extra
	workspaceRoots = roots
	readableRootsTmpDir = ""
	readableFileRoots = nil
	workspaceRootsMu.Unlock()
	return roots
}

// AllowedReadableFileRoots returns directory prefixes that may be served read-only
// via file preview/download endpoints (/api/v1/files/raw, /api/v1/files/content, etc.).
// It always includes AllowedWorkspaceRoots(). When ALLOWED_WORKSPACE_ROOTS is unset
// (default paired-computer mode), system temporary directories (os.TempDir() and /tmp
// on Unix) are also included so agent-generated temporary media/artifacts can be previewed
// or downloaded without exposing temp directories as writable workspace roots.
func AllowedReadableFileRoots() []string {
	wsRoots := AllowedWorkspaceRoots()
	if len(wsRoots) == 0 {
		return nil
	}
	extra := strings.TrimSpace(os.Getenv("MULTIGRAVITY_ALLOWED_WORKSPACE_ROOTS"))
	if extra == "" {
		extra = strings.TrimSpace(os.Getenv("ALLOWED_WORKSPACE_ROOTS"))
	}
	if extra != "" {
		return wsRoots
	}

	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return wsRoots
	}
	tmpDir := strings.TrimSpace(os.TempDir())

	workspaceRootsMu.RLock()
	if readableFileRoots != nil && workspaceRootsHome == home && workspaceRootsExtra == extra && readableRootsTmpDir == tmpDir {
		cached := readableFileRoots
		workspaceRootsMu.RUnlock()
		return cached
	}
	workspaceRootsMu.RUnlock()

	roots := make([]string, len(wsRoots))
	copy(roots, wsRoots)
	if tmpDir != "" {
		cleanTmp := filepath.Clean(tmpDir)
		resolvedTmp := evalSymlinksExistingPrefix(cleanTmp)
		if !isDriveRoot(cleanTmp) && !isDriveRoot(resolvedTmp) {
			roots = appendNormalizedRootPrefix(roots, cleanTmp)
		}
	}
	if runtime.GOOS != "windows" {
		roots = appendNormalizedRootPrefix(roots, "/tmp")
	}

	workspaceRootsMu.Lock()
	if workspaceRootsHome == home && workspaceRootsExtra == extra {
		readableRootsTmpDir = tmpDir
		readableFileRoots = roots
	}
	workspaceRootsMu.Unlock()
	return roots
}

func resetWorkspaceRootsForTest() {
	workspaceRootsMu.Lock()
	workspaceRootsHome = ""
	workspaceRootsExtra = ""
	workspaceRoots = nil
	readableRootsTmpDir = ""
	readableFileRoots = nil
	workspaceRootsMu.Unlock()
}

// IsSafeFilePath validates that the resolved path is within an allowed workspace directory
// and is not a credential or secret file. Default allowlist is the user's home
// (agent-generated PPTX/PDF/HTML/images live on Desktop, Downloads, Websites, etc.).
func IsSafeFilePath(path string) bool {
	return isSafePathUnderRoots(path, AllowedWorkspaceRoots())
}

// IsSafeReadableFilePath validates that the resolved path is within an allowed read-only
// directory (AllowedReadableFileRoots, including system temp directories in default mode)
// and is not a credential or secret file.
func IsSafeReadableFilePath(path string) bool {
	return isSafePathUnderRoots(path, AllowedReadableFileRoots())
}

func isSafePathUnderRoots(path string, roots []string) bool {
	if strings.TrimSpace(path) == "" || strings.ContainsRune(path, 0) {
		return false
	}
	clean := filepath.Clean(path)
	resolved := evalSymlinksExistingPrefix(clean)

	// Lower-cased: macOS/Windows file systems are case-insensitive, so ~/.AWS must match /.aws/.
	slashPath := strings.ToLower(filepath.ToSlash(resolved))
	sensitiveDirs := []string{
		"/.ssh/", "/.gnupg/", "/.aws/", "/.docker/", "/Library/Keychains/",
		"/Library/Application Support/",
		"/Library/Cookies/", "/Library/Safari/", "/Library/Mail/",
		"/Library/Messages/", "/Library/Containers/", "/Library/Group Containers/",
		"/.kube/", "/.config/gcloud/", "/gcloud/",
		"/.multigravity/",
		"/.antigravity-mobile/",
		"/.antigravity_cockpit/", "/.cockpit-tools/", "/.hal9000/",
		"/.claude/", "/.codex/", "/.cursor/",
		"/.acme.sh/", "/.lego/", "/.certbot/",
		"/.config/gh/", "/.config/hub/", "/.config/rclone/", "/.config/op/", "/.config/doctl/",
		"/.config/google-chrome/", "/.config/chromium/", "/.config/bravesoftware/", "/.mozilla/",
		"/.azure/", "/.oci/", "/.terraform.d/", "/.password-store/", "/.local/share/keyrings/",
		"/.cargo/", "/.gem/", "/.m2/", "/.gradle/", "/.config/pip/",
	}
	for _, sd := range sensitiveDirs {
		sd = strings.ToLower(sd)
		if strings.Contains(slashPath, sd) || strings.HasSuffix(slashPath, strings.TrimSuffix(sd, "/")) {
			return false
		}
	}
	for _, sf := range []string{
		"/.gemini/settings.json",
		"/.gemini/config/config.json",
		"/.gemini/antigravity/mcp_config.json",
	} {
		if strings.HasSuffix(slashPath, sf) {
			return false
		}
	}

	base := strings.ToLower(filepath.Base(resolved))
	sensitiveNames := []string{
		"id_rsa", "id_ed25519", "id_ecdsa", "id_dsa",
		".env", ".envrc", ".git-credentials", ".gitconfig", ".netrc", ".dockercfg", ".npmrc", ".pypirc",
		".bash_history", ".zsh_history", ".python_history", ".node_repl_history", ".psql_history", ".mysql_history",
		".zshrc", ".zprofile", ".zshenv", ".bashrc", ".bash_profile", ".profile",
		".pgpass", ".my.cnf", ".vault-token", ".s3cfg", ".boto", "credentials", "credentials.toml",
		"hosts.yml", "secrets.json", "secrets.yml", "secrets.yaml", "service-account.json",
		"oauth_creds.json", "jetski-standalone-oauth-token", "google_accounts.json", "mcp_config.json",
		"auth_store.json", "credentials.db", "credentials.json",
		"secure-account-storage.key", "cf_tunnel.json", "admin_token",
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
	for _, ext := range []string{".pem", ".p12", ".pfx", ".jks", ".keystore"} {
		if strings.HasSuffix(base, ext) {
			return false
		}
	}
	if strings.HasSuffix(base, ".key") {
		sensitiveKeyKeywords := []string{
			"id_", "private", "secret", "tls", "ssl", "server", "client",
			"cert", "ca", "rsa", "ecdsa", "ed25519", "dsa", "priv", "domain", "host",
			"storage", "account", "master", "encrypt", "auth", "token", "cred", "api",
		}
		for _, kw := range sensitiveKeyKeywords {
			if strings.Contains(base, kw) {
				return false
			}
		}
	}

	sep := string(filepath.Separator)
	for _, prefix := range roots {
		if strings.HasPrefix(resolved, prefix) || resolved+sep == prefix {
			return true
		}
	}
	return false
}

// validateWorkspaceDir resolves symlinks and enforces that rawPath is an absolute directory
// inside AllowedWorkspaceRoots() and passes IsSafeFilePath.
func validateWorkspaceDir(rawPath string, mustExist bool) (string, error) {
	p := strings.TrimSpace(rawPath)
	p = strings.Trim(p, `"'`)
	p = strings.TrimSpace(p)
	if p == "" || strings.ContainsRune(p, 0) {
		return "", fmt.Errorf("无效的工作区路径")
	}
	if strings.HasPrefix(p, "file://") {
		p = uriToPath(p)
	}
	if strings.HasPrefix(p, "-") {
		return "", fmt.Errorf("非法的工作区路径")
	}
	if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return "", fmt.Errorf("无法解析用户主目录")
		}
		if p == "~" {
			p = home
		} else {
			p = filepath.Join(home, p[2:])
		}
	}
	if !filepath.IsAbs(p) {
		return "", fmt.Errorf("工作区路径必须为绝对路径")
	}
	clean := filepath.Clean(p)
	if isDriveRoot(clean) {
		return "", fmt.Errorf("不能将系统根目录作为工作区")
	}
	resolved := evalSymlinksExistingPrefix(clean)
	if isDriveRoot(resolved) || strings.HasPrefix(resolved, "-") {
		return "", fmt.Errorf("非法的工作区路径")
	}
	if !IsSafeFilePath(resolved) {
		return "", fmt.Errorf("工作区路径不在允许访问的目录范围内")
	}
	if fi, err := os.Stat(resolved); err == nil {
		if !fi.IsDir() {
			return "", fmt.Errorf("工作区路径不是目录")
		}
	} else if mustExist {
		return "", fmt.Errorf("工作区目录不存在")
	}
	return resolved, nil
}

// readRegularFileLimitedNoFollow opens a regular file without following symlinks and reads at most maxBytes.
func readRegularFileLimitedNoFollow(path string, maxBytes int64) ([]byte, error) {
	f, err := openRegularNoFollow(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("not a regular file")
	}
	if maxBytes > 0 && fi.Size() > maxBytes {
		return nil, fmt.Errorf("file exceeds size limit")
	}
	if maxBytes <= 0 {
		maxBytes = maxReadSizeBytes
	}
	return io.ReadAll(io.LimitReader(f, maxBytes))
}

// ensureNotSymlinkTarget refuses writes or deletes when path itself is a symbolic link.
func ensureNotSymlinkTarget(path string) error {
	if fi, err := os.Lstat(path); err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("拒绝操作符号链接目标")
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	return nil
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

// isBinaryContentExtension reports whether an extension represents binary files (images, audio, video, archives, compiled docs)
// that cannot be displayed as plain text in the markdown/artifact content viewer.
func isBinaryContentExtension(ext string) bool {
	switch strings.ToLower(ext) {
	case ".png", ".jpg", ".jpeg", ".webp", ".gif", ".heic", ".heif", ".bmp", ".ico", ".tiff", ".tif",
		".mp4", ".mov", ".m4v", ".webm", ".avi", ".mkv", ".3gp", ".ts", ".flv",
		".mp3", ".wav", ".m4a", ".aac", ".flac", ".ogg",
		".pdf", ".pptx", ".ppt", ".key", ".docx", ".doc", ".xlsx", ".xls",
		".zip", ".tar", ".gz", ".7z", ".rar", ".zst", ".bz2", ".xz",
		".exe", ".dll", ".so", ".dylib", ".bin":
		return true
	default:
		return false
	}
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
	if !IsSafeReadableFilePath(filePath) {
		return nil, errAccessRestricted
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

	ext := strings.ToLower(filepath.Ext(filePath))
	if isBinaryContentExtension(ext) {
		return nil, fmt.Errorf("binary file %s cannot be viewed as text", filepath.Base(filePath))
	}

	contentBytes, err := io.ReadAll(io.LimitReader(f, maxReadSizeBytes+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read file")
	}

	if !utf8.Valid(contentBytes) {
		return nil, fmt.Errorf("file contains binary data and cannot be displayed as text")
	}

	res := &FileContentResult{
		URI:      "file://" + filePath,
		Filename: filepath.Base(filePath),
		Content:  string(contentBytes),
	}

	// Check companion metadata file
	metaFile := filePath + ".metadata.json"
	if IsSafeReadableFilePath(metaFile) {
		if metaBytes, err := os.ReadFile(metaFile); err == nil {
			var meta ArtifactMetadata
			if err := json.Unmarshal(metaBytes, &meta); err == nil {
				res.Summary = strings.TrimSpace(meta.Summary)
				res.RequestFeedback = meta.RequestFeedback
				res.UserFacing = meta.IsUserFacing()
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

	// Transparently bridge MCP App ui:// resources into the standard file content endpoint
	// so <agent-embed src="ui://..."> renders cleanly across all clients without extra UI.
	if strings.HasPrefix(strings.ToLower(uri), "ui://") {
		content, _, _, status, err := p.fetchMcpAppResourceHTML(r.Context(), uri, strings.TrimSpace(q.Get("serverName")))
		if err != nil {
			if status < 400 {
				status = http.StatusBadGateway
			}
			errBytes, _ := json.Marshal(map[string]string{"error": err.Error()})
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.Header().Set("Content-Length", strconv.Itoa(len(errBytes)))
			w.WriteHeader(status)
			_, _ = w.Write(errBytes)
			return
		}
		name := "mcp_app.html"
		if u, uErr := url.Parse(uri); uErr == nil {
			if base := strings.TrimSpace(filepath.Base(u.Path)); base != "" && base != "." && base != "/" {
				name = base
			} else if strings.TrimSpace(u.Host) != "" {
				name = strings.TrimSpace(u.Host)
			}
		}
		data, err := json.Marshal(&FileContentResult{
			URI:      uri,
			Filename: name,
			Content:  content,
		})
		if err != nil {
			writeJSONError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
		return
	}

	result, err := GetFileContent(uri, cascadeID)
	if err != nil {
		errBytes, _ := json.Marshal(map[string]string{"error": err.Error()})
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Content-Length", strconv.Itoa(len(errBytes)))
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write(errBytes)
		return
	}

	data, err := json.Marshal(result)
	if err != nil {
		errBytes, _ := json.Marshal(map[string]string{"error": err.Error()})
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Content-Length", strconv.Itoa(len(errBytes)))
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write(errBytes)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
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
		if errors.Is(err, errAccessRestricted) {
			slog.Info("[Files] denied raw download", "uri", uri)
			http.Error(w, "access to file is restricted", http.StatusForbidden)
			return
		}
		http.Error(w, fmt.Sprintf("failed to resolve file path: %v", err), http.StatusBadRequest)
		return
	}

	if resolved, err := filepath.EvalSymlinks(filePath); err == nil {
		filePath = resolved
	}
	if !IsSafeReadableFilePath(filePath) {
		slog.Info("[Files] denied raw download", "path", filePath, "uri", uri)
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
		slog.Info(fmt.Sprintf("[Files] denied raw download of private key header in %s", filePath))
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

	// Dual-tier mobile preview transcode: ?preview=thumb (bubble ~40-90KB) or ?preview=1 (viewer ~150-450KB).
	// Unset preview or non-image/small/animated/unsupported files fall through untouched to raw streaming.
	if previewMode := parseImagePreviewMode(q.Get("preview")); previewMode != imagePreviewNone && isTranscodablePreviewExt(ext) {
		previewETag := fmt.Sprintf(`"%x-%x-p-%s"`, fi.ModTime().UnixNano(), fi.Size(), previewMode)
		if strings.TrimSpace(r.Header.Get("If-None-Match")) == previewETag {
			w.Header().Set("ETag", previewETag)
			w.Header().Set("Cache-Control", "private, max-age=604800, must-revalidate")
			w.WriteHeader(http.StatusNotModified)
			return
		}
		if previewBytes, ok := getOrBuildImagePreview(filePath, ext, fi, f, previewMode); ok {
			previewName := strings.TrimSuffix(fileName, filepath.Ext(fileName)) + ".jpg"
			encodedPreviewName := url.PathEscape(previewName)
			w.Header().Set("Content-Type", "image/jpeg")
			w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q; filename*=UTF-8''%s", previewName, encodedPreviewName))
			w.Header().Set("ETag", previewETag)
			w.Header().Set("Cache-Control", "private, max-age=604800, must-revalidate")
			http.ServeContent(w, r, previewName, fi.ModTime(), bytes.NewReader(previewBytes))
			return
		}
		_, _ = f.Seek(0, io.SeekStart)
	}

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
	case ".html", ".htm", ".xhtml", ".mhtml":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox; style-src 'unsafe-inline'; img-src data: https:;")
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
		// SEC-AUDIT L-5: Restrictive CSP for SVG to prevent script execution in older browsers
		w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox; style-src 'unsafe-inline'")
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
		w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
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
	case ".amr":
		w.Header().Set("Content-Type", "audio/amr")
	case ".aac":
		w.Header().Set("Content-Type", "audio/aac")
	case ".ogg", ".oga":
		w.Header().Set("Content-Type", "audio/ogg")
	case ".opus":
		w.Header().Set("Content-Type", "audio/opus")
	case ".flac":
		w.Header().Set("Content-Type", "audio/flac")
	case ".caf":
		w.Header().Set("Content-Type", "audio/x-caf")
	case ".wma":
		w.Header().Set("Content-Type", "audio/x-ms-wma")
	case ".mp4":
		w.Header().Set("Content-Type", "video/mp4")
	case ".mov":
		w.Header().Set("Content-Type", "video/quicktime")
	case ".webm":
		w.Header().Set("Content-Type", "video/webm")
	case ".m4v":
		w.Header().Set("Content-Type", "video/x-m4v")
	case ".mkv":
		w.Header().Set("Content-Type", "video/x-matroska")
	case ".avi":
		w.Header().Set("Content-Type", "video/x-msvideo")
	case ".3gp":
		w.Header().Set("Content-Type", "video/3gpp")
	case ".r", ".rmd", ".rmarkdown":
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	}

	encodedName := url.PathEscape(fileName)
	// Force download for potentially dangerous file types to prevent stored XSS (SEC-04)
	disposition := "inline"
	switch ext {
	case ".html", ".htm", ".svg", ".xml", ".xhtml", ".mhtml":
		disposition = "attachment"
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("%s; filename=%q; filename*=UTF-8''%s", disposition, fileName, encodedName))

	etag := fmt.Sprintf(`"%x-%x"`, fi.ModTime().UnixNano(), fi.Size())
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "private, max-age=604800, must-revalidate")

	http.ServeContent(w, r, fileName, fi.ModTime(), f)
}
