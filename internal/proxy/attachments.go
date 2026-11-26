package proxy

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

// File attachments: mobile clients upload a document to the gateway host, get back an
// attachment id, and later reference that id from SendUserCascadeMessage. The gateway rewrites
// the message text so the agent sees the absolute path of the uploaded file.
//
// Layout: <inbox>/<yyyy-MM>/<id16>-<sanitized name>, where id16 is the first 16 hex chars of the
// file's sha256. Because the id is part of the file name, lookups are stateless (a glob) and
// identical uploads deduplicate naturally.

const (
	defaultMaxUploadMB         = 50
	defaultMaxMediaUploadMB    = 200 // Antigravity 2.33.0 supports audio/video uploads up to 200 MB
	defaultInboxQuotaMB        = 2048
	defaultInboxTTLDays        = 30
	maxAttachmentsPerMsg       = 5
	maxAttachmentsTotal        = 100 * 1024 * 1024
	maxMediaAttachmentsTotal   = 400 * 1024 * 1024
	maxAttachmentNameLen       = 120
	attachmentIDLen            = 16
	maxNativeAttachmentBytes   = 20 * 1024 * 1024  // 20 MB (Antigravity 2.28 native attachment threshold)
	maxNativeMediaAttachBytes  = 200 * 1024 * 1024 // 200 MB for supported audio/video media
)

var attachmentIDPattern = regexp.MustCompile(`^[0-9a-f]{16}$`)

// attachmentExts maps known file extensions (lower-case, without dot) to their human category.
var attachmentExts = buildAttachmentExts()

func buildAttachmentExts() map[string]string {
	m := map[string]string{}
	add := func(kind string, exts ...string) {
		for _, e := range exts {
			m[e] = kind
		}
	}
	add("document", "doc", "docx", "dot", "dotx", "rtf", "odt", "pages", "pdf", "md", "markdown", "txt", "log", "epub")
	add("spreadsheet", "xls", "xlsx", "xlsm", "csv", "tsv", "ods", "numbers")
	add("presentation", "ppt", "pptx", "pps", "ppsx", "odp", "key")
	add("audio", "mp3", "m4a", "wav", "aac", "flac", "ogg", "oga", "opus", "aif", "aiff", "wma", "amr", "caf")
	add("video", "mp4", "mov", "mkv", "webm", "avi", "flv", "wmv", "m4v", "3gp")
	add("design", "psd", "ai", "fig", "sketch", "xd", "blend")
	add("program", "exe", "dll", "dylib", "so", "bin", "apk", "deb", "rpm", "msi", "app", "dmg")
	add("archive", "zip", "tar", "gz", "tgz", "7z")
	add("code",
		"py", "js", "mjs", "cjs", "ts", "tsx", "jsx", "go", "rs", "java", "kt", "kts", "swift", "m", "mm",
		"c", "cc", "cpp", "cxx", "h", "hpp", "cs", "rb", "php", "lua", "dart", "scala", "r", "rmd", "rmarkdown", "pl",
		"sh", "bash", "zsh", "bat", "ps1", "sql", "vue", "svelte", "gradle", "proto",
		"json", "jsonl", "yaml", "yml", "toml", "ini", "cfg", "conf", "xml", "html", "htm", "css", "scss",
		"less", "ipynb", "dockerfile", "makefile")
	return m
}

// attachmentKind returns the category for a file name. With Antigravity 2.28, all file types are allowed.
func attachmentKind(name string) string {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))
	if ext == "" {
		// Extension-less well-known names (Dockerfile, Makefile, README).
		ext = strings.ToLower(name)
	}
	if k, ok := attachmentExts[ext]; ok {
		return k
	}
	if ext != "" {
		return ext
	}
	return "file"
}

// looksLikeExecutable reports whether the head of a file carries a native-executable magic
// number (PE, ELF, Mach-O, dex). It is retained for classification/informational purposes.
func looksLikeExecutable(head []byte) bool {
	switch {
	case bytes.HasPrefix(head, []byte("MZ")),
		bytes.HasPrefix(head, []byte("\x7fELF")),
		bytes.HasPrefix(head, []byte("dex\n")),
		bytes.HasPrefix(head, []byte{0xfe, 0xed, 0xfa, 0xce}),
		bytes.HasPrefix(head, []byte{0xfe, 0xed, 0xfa, 0xcf}),
		bytes.HasPrefix(head, []byte{0xce, 0xfa, 0xed, 0xfe}),
		bytes.HasPrefix(head, []byte{0xcf, 0xfa, 0xed, 0xfe}),
		bytes.HasPrefix(head, []byte{0xca, 0xfe, 0xba, 0xbe}):
		return true
	}
	return false
}

// sanitizeAttachmentName turns a client supplied name into a safe single path component.
func sanitizeAttachmentName(raw string) string {
	name := raw
	name = strings.ReplaceAll(name, "\\", "/")
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == 0x7f || r == '‮' {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(strings.Trim(strings.TrimSpace(name), "."))
	if name == "" {
		return "file"
	}
	if len(name) > maxAttachmentNameLen {
		ext := filepath.Ext(name)
		if len(ext) > 16 {
			ext = ""
		}
		stem := name[:maxAttachmentNameLen-len(ext)]
		for !utf8.ValidString(stem) && len(stem) > 0 {
			stem = stem[:len(stem)-1]
		}
		name = stem + ext
	}
	return name
}

func envInt(key string, def int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

// AttachmentInboxDir returns the directory uploaded attachments are stored in.
// It deliberately lives outside ~/.multigravity so IsSafeFilePath still allows previews.
func AttachmentInboxDir() (string, error) {
	if d := strings.TrimSpace(os.Getenv("MGY_INBOX_DIR")); d != "" {
		return d, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Multigravity", "Inbox"), nil
}

// Attachment describes an uploaded file.
type Attachment struct {
	ID     string `json:"id"`
	Path   string `json:"path"`
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	Mime   string `json:"mime"`
	SHA256 string `json:"sha256"`
	Kind   string `json:"kind"`
	// Line is the exact "- <path> (ext, size)" line the gateway appends to the message text.
	// Clients use it to render the optimistic bubble identically to what the server stores.
	Line string `json:"line"`
}

func attachmentFromPath(path string) (Attachment, bool) {
	base := filepath.Base(path)
	if len(base) <= attachmentIDLen+1 || base[attachmentIDLen] != '-' {
		return Attachment{}, false
	}
	id := base[:attachmentIDLen]
	if !attachmentIDPattern.MatchString(id) {
		return Attachment{}, false
	}
	st, err := os.Stat(path)
	if err != nil || !st.Mode().IsRegular() {
		return Attachment{}, false
	}
	name := base[attachmentIDLen+1:]
	return Attachment{
		ID:   id,
		Path: path,
		Name: name,
		Size: st.Size(),
		Mime: mimeForName(name),
		Kind: attachmentKind(name),
	}, true
}

var extraMimeTypes = map[string]string{
	".amr":       "audio/amr",
	".aac":       "audio/aac",
	".ogg":       "audio/ogg",
	".oga":       "audio/ogg",
	".opus":      "audio/opus",
	".flac":      "audio/flac",
	".caf":       "audio/x-caf",
	".wma":       "audio/x-ms-wma",
	".m4a":       "audio/mp4",
	".mp3":       "audio/mpeg",
	".wav":       "audio/wav",
	".mp4":       "video/mp4",
	".webm":      "video/webm",
	".mov":       "video/quicktime",
	".m4v":       "video/x-m4v",
	".mkv":       "video/x-matroska",
	".avi":       "video/x-msvideo",
	".3gp":       "video/3gpp",
	".r":         "text/plain; charset=utf-8",
	".rmd":       "text/markdown; charset=utf-8",
	".rmarkdown": "text/markdown; charset=utf-8",
}

func isLargeMediaKind(kind string) bool {
	return kind == "audio" || kind == "video"
}

func maxUploadBytesForKind(kind string) int64 {
	if strings.TrimSpace(os.Getenv("MGY_MAX_UPLOAD_MB")) == "" && isLargeMediaKind(kind) {
		return int64(defaultMaxMediaUploadMB) * 1024 * 1024
	}
	return int64(envInt("MGY_MAX_UPLOAD_MB", defaultMaxUploadMB)) * 1024 * 1024
}

func supportsNativeAttachment(a Attachment) bool {
	if isLargeMediaKind(a.Kind) {
		return a.Size <= maxNativeMediaAttachBytes
	}
	return a.Size <= maxNativeAttachmentBytes
}

func mimeForName(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	if t, ok := extraMimeTypes[ext]; ok {
		return t
	}
	if t := mime.TypeByExtension(ext); t != "" {
		return t
	}
	return "application/octet-stream"
}

// findAttachment looks an attachment up by id.
func findAttachment(inbox, id string) (Attachment, bool) {
	if !attachmentIDPattern.MatchString(id) {
		return Attachment{}, false
	}
	matches, err := filepath.Glob(filepath.Join(inbox, "*", id+"-*"))
	if err != nil {
		return Attachment{}, false
	}
	for _, m := range matches {
		if a, ok := attachmentFromPath(m); ok {
			return a, true
		}
	}
	return Attachment{}, false
}

var (
	inboxQuotaMu      sync.Mutex
	trackedInboxDir   string
	trackedInboxBytes int64
	trackedInboxInit  bool
)

func inboxSizeScan(inbox string) int64 {
	var total int64
	_ = filepath.WalkDir(inbox, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if d.Name() == ".tmp" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.Contains(d.Name(), "upload-") {
			return nil
		}
		if info, err := d.Info(); err == nil {
			total += info.Size()
		}
		return nil
	})
	return total
}

func inboxSize(inbox string) int64 {
	clean := filepath.Clean(inbox)
	inboxQuotaMu.Lock()
	defer inboxQuotaMu.Unlock()
	if !trackedInboxInit || trackedInboxDir != clean {
		trackedInboxDir = clean
		trackedInboxBytes = inboxSizeScan(clean)
		trackedInboxInit = true
	}
	return trackedInboxBytes
}

func commitInboxBytes(inbox string, delta int64, quota int64) bool {
	clean := filepath.Clean(inbox)
	inboxQuotaMu.Lock()
	defer inboxQuotaMu.Unlock()
	if !trackedInboxInit || trackedInboxDir != clean {
		trackedInboxDir = clean
		trackedInboxBytes = inboxSizeScan(clean)
		trackedInboxInit = true
	}
	if quota > 0 && delta > 0 && trackedInboxBytes >= quota {
		return false
	}
	trackedInboxBytes += delta
	if trackedInboxBytes < 0 {
		trackedInboxBytes = 0
	}
	return true
}

func invalidateInboxSizeCache(inbox string) {
	clean := filepath.Clean(inbox)
	inboxQuotaMu.Lock()
	if trackedInboxDir == clean {
		trackedInboxInit = false
	}
	inboxQuotaMu.Unlock()
}

func writeAttachmentError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code, "message": msg})
}

// HandleAttachmentUpload implements POST /api/v1/attachments.
//
// The body is the raw file. Headers: X-File-Name (URL-encoded, required).
func (p *Proxy) HandleAttachmentUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	rawName := r.Header.Get("X-File-Name")
	if unescaped, err := url.PathUnescape(rawName); err == nil {
		rawName = unescaped
	}
	name := sanitizeAttachmentName(rawName)
	kind := attachmentKind(name)

	maxBytes := maxUploadBytesForKind(kind)
	if r.ContentLength > maxBytes {
		writeAttachmentError(w, http.StatusRequestEntityTooLarge, "too_large",
			fmt.Sprintf("文件超过 %dMB 上限", maxBytes/1024/1024))
		return
	}

	inbox, err := AttachmentInboxDir()
	if err != nil {
		writeAttachmentError(w, http.StatusInternalServerError, "inbox_unavailable", err.Error())
		return
	}
	tmpDir := filepath.Join(inbox, ".tmp")
	if err := os.MkdirAll(tmpDir, 0o700); err != nil {
		writeAttachmentError(w, http.StatusInternalServerError, "inbox_unavailable", err.Error())
		return
	}
	quota := int64(envInt("MGY_INBOX_QUOTA_MB", defaultInboxQuotaMB)) * 1024 * 1024
	if inboxSize(inbox) >= quota {
		writeAttachmentError(w, http.StatusInsufficientStorage, "quota_exceeded", "电脑端收件箱已满")
		return
	}

	tmp, err := os.CreateTemp(tmpDir, "upload-*")
	if err != nil {
		writeAttachmentError(w, http.StatusInternalServerError, "inbox_unavailable", err.Error())
		return
	}
	tmpPath := tmp.Name()
	committed := false
	defer func() {
		tmp.Close()
		if !committed {
			os.Remove(tmpPath)
		}
	}()
	_ = tmp.Chmod(0o600)

	hasher := sha256.New()
	body := http.MaxBytesReader(w, r.Body, maxBytes)
	head := &headCapture{limit: 8}
	n, err := io.Copy(io.MultiWriter(tmp, hasher, head), body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeAttachmentError(w, http.StatusRequestEntityTooLarge, "too_large",
				fmt.Sprintf("文件超过 %dMB 上限", maxBytes/1024/1024))
			return
		}
		writeAttachmentError(w, http.StatusBadRequest, "read_failed", "上传中断")
		return
	}
	if n == 0 {
		writeAttachmentError(w, http.StatusBadRequest, "empty_file", "文件为空")
		return
	}
	if err := tmp.Close(); err != nil {
		writeAttachmentError(w, http.StatusInternalServerError, "write_failed", err.Error())
		return
	}

	sum := hex.EncodeToString(hasher.Sum(nil))
	id := sum[:attachmentIDLen]

	att, exists := findAttachment(inbox, id)
	if !exists {
		if !commitInboxBytes(inbox, n, quota) {
			writeAttachmentError(w, http.StatusInsufficientStorage, "quota_exceeded", "电脑端收件箱已满")
			return
		}
		monthDir := filepath.Join(inbox, time.Now().Format("2006-01"))
		if err := os.MkdirAll(monthDir, 0o700); err != nil {
			commitInboxBytes(inbox, -n, 0)
			writeAttachmentError(w, http.StatusInternalServerError, "write_failed", err.Error())
			return
		}
		finalPath := filepath.Join(monthDir, id+"-"+name)
		if err := os.Rename(tmpPath, finalPath); err != nil {
			commitInboxBytes(inbox, -n, 0)
			writeAttachmentError(w, http.StatusInternalServerError, "write_failed", err.Error())
			return
		}
		committed = true
		att = Attachment{ID: id, Path: finalPath, Name: name, Size: n, Mime: mimeForName(name), Kind: kind}
	} else {
		// Touch so TTL cleanup keeps a re-uploaded file alive.
		now := time.Now()
		_ = os.Chtimes(att.Path, now, now)
	}
	att.SHA256 = sum
	att.Line = attachmentLine(att)

	slog.Info("[Attachments] stored", "id", id, "name", att.Name, "size", att.Size, "dedup", exists)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(att)
}

type headCapture struct {
	buf   []byte
	limit int
}

func (h *headCapture) Write(b []byte) (int, error) {
	if room := h.limit - len(h.buf); room > 0 {
		if len(b) < room {
			room = len(b)
		}
		h.buf = append(h.buf, b[:room]...)
	}
	return len(b), nil
}

// resolveAttachmentRefs looks up client supplied attachment ids and enforces per-message limits.
func resolveAttachmentRefs(ids []string) ([]Attachment, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	if len(ids) > maxAttachmentsPerMsg {
		return nil, fmt.Errorf("一条消息最多附带 %d 个文件", maxAttachmentsPerMsg)
	}
	inbox, err := AttachmentInboxDir()
	if err != nil {
		return nil, err
	}
	var out []Attachment
	var total int64
	hasMedia := false
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		a, ok := findAttachment(inbox, id)
		if !ok {
			return nil, fmt.Errorf("附件不存在或已过期: %s", id)
		}
		total += a.Size
		if isLargeMediaKind(a.Kind) {
			hasMedia = true
		}
		out = append(out, a)
	}
	totalLimit := int64(maxAttachmentsTotal)
	if hasMedia {
		totalLimit = maxMediaAttachmentsTotal
	}
	if total > totalLimit {
		return nil, fmt.Errorf("附件总大小超过 %dMB", totalLimit/1024/1024)
	}
	return out, nil
}

func humanSize(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}

// attachmentHeader is the first line of the block appended to a message. Clients recognise the
// "- <inbox path> (...)" lines that follow it to render file cards.
const attachmentHeader = "📎 附件（已上传到本机，可直接读取）："

// formatAttachmentBlock renders the text block appended to the user's message.
func formatAttachmentBlock(atts []Attachment) string {
	var b strings.Builder
	b.WriteString(attachmentHeader)
	for _, a := range atts {
		b.WriteString("\n")
		b.WriteString(attachmentLine(a))
	}
	return b.String()
}

func attachmentLine(a Attachment) string {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(a.Name)), ".")
	if ext == "" {
		ext = a.Kind
	}
	return fmt.Sprintf("- %s (%s, %s)", a.Path, ext, humanSize(a.Size))
}

// FindAttachmentByID looks up an uploaded attachment in the local inbox by its 16-char ID.
func FindAttachmentByID(id string) (Attachment, bool) {
	inbox, err := AttachmentInboxDir()
	if err != nil || inbox == "" {
		return Attachment{}, false
	}
	return findAttachment(inbox, id)
}

// ExtractAttachmentIDsFromText returns canonical 16-char attachment IDs embedded in an attachment block.
func ExtractAttachmentIDsFromText(text string) []string {
	return extractAttachmentIDsFromText(text)
}

// stageNativeMediaForUpstream creates a per-send staged link/copy under <inbox>/.tmp/media-<rand>/<base>
// so Antigravity Language Server can move media[].uri into brain/<cascadeID>/.user_uploaded/<base>
// without removing the canonical file at <inbox>/<yyyy-MM>/<id16>-<name> referenced in the prompt text.
func stageNativeMediaForUpstream(inbox string, a Attachment) string {
	if inbox == "" || a.Path == "" {
		return a.Path
	}
	var rnd [6]byte
	if _, err := rand.Read(rnd[:]); err != nil {
		return a.Path
	}
	stageDir := filepath.Join(inbox, ".tmp", "media-"+hex.EncodeToString(rnd[:]))
	if err := os.MkdirAll(stageDir, 0o700); err != nil {
		return a.Path
	}
	stagedPath := filepath.Join(stageDir, filepath.Base(a.Path))
	// Prefer O(1) zero-copy hard link on Unix; fall back to file copy on Windows or cross-device errors.
	if runtime.GOOS != "windows" {
		if err := os.Link(a.Path, stagedPath); err == nil {
			return stagedPath
		}
	}
	if err := copyAttachmentFile(a.Path, stagedPath); err == nil {
		return stagedPath
	}
	_ = os.RemoveAll(stageDir)
	return a.Path
}

func copyAttachmentFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		_ = os.Remove(dst)
		return err
	}
	return out.Close()
}

var attachmentLinePathRegex = regexp.MustCompile(`(?m)^-\s+(.+?)\s+\([^,()]*,\s*[^()]+\)\s*$`)

// extractAttachmentIDsFromText finds canonical <id16>-<name> attachment IDs already formatted
// inside an attachmentHeader block (e.g., when a queued message is resent via "Send Now").
func extractAttachmentIDsFromText(text string) []string {
	idx := strings.LastIndex(text, attachmentHeader)
	if idx < 0 {
		return nil
	}
	tail := text[idx+len(attachmentHeader):]
	matches := attachmentLinePathRegex.FindAllStringSubmatch(tail, maxAttachmentsPerMsg)
	if len(matches) == 0 {
		return nil
	}
	seen := map[string]bool{}
	var ids []string
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		base := filepath.Base(strings.TrimSpace(m[1]))
		if len(base) <= attachmentIDLen+1 || base[attachmentIDLen] != '-' {
			continue
		}
		id := base[:attachmentIDLen]
		if !attachmentIDPattern.MatchString(id) || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids
}

func appendStagedNativeMedia(rawMap map[string]interface{}, inbox string, atts []Attachment) bool {
	var smallAtts []Attachment
	for _, a := range atts {
		if supportsNativeAttachment(a) {
			smallAtts = append(smallAtts, a)
		}
	}
	if len(smallAtts) == 0 {
		return false
	}

	var mediaList []interface{}
	existingBases := map[string]bool{}
	if existing, ok := rawMap["media"].([]interface{}); ok {
		mediaList = existing
		for _, item := range existing {
			if m, ok := item.(map[string]interface{}); ok {
				if u, ok := m["uri"].(string); ok && u != "" {
					existingBases[filepath.Base(strings.TrimPrefix(u, "file://"))] = true
				}
			}
		}
	}

	added := false
	for _, a := range smallAtts {
		base := filepath.Base(a.Path)
		if existingBases[base] {
			continue
		}
		mimeType := a.Mime
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
		mediaPath := stageNativeMediaForUpstream(inbox, a)
		mediaList = append(mediaList, map[string]interface{}{
			"uri":      "file://" + mediaPath,
			"mimeType": mimeType,
		})
		existingBases[base] = true
		added = true
	}
	if added {
		rawMap["media"] = mediaList
	}
	return added
}

// applyAttachmentsToMessage consumes the "attachments" field of a SendUserCascadeMessage body and
// appends the attachment block to both "text" and "items[0].text". It returns whether the body
// was changed. The "attachments" key is always removed so it never reaches the language server.
func applyAttachmentsToMessage(rawMap map[string]interface{}) (bool, error) {
	inbox, _ := AttachmentInboxDir()

	raw, present := rawMap["attachments"]
	if !present {
		// Fallback for "Send Now" / resent queued messages whose text already embeds the attachment
		// block without the original attachments[] id list: re-attach staged native media when still in Inbox.
		if inbox == "" {
			return false, nil
		}
		var candidateTexts []string
		if t, ok := rawMap["text"].(string); ok && strings.Contains(t, attachmentHeader) {
			candidateTexts = append(candidateTexts, t)
		}
		if items, ok := rawMap["items"].([]interface{}); ok {
			for _, it := range items {
				if m, ok := it.(map[string]interface{}); ok {
					if t, ok := m["text"].(string); ok && strings.Contains(t, attachmentHeader) {
						candidateTexts = append(candidateTexts, t)
					}
				}
			}
		}
		seenIDs := map[string]bool{}
		var resolved []Attachment
		for _, txt := range candidateTexts {
			for _, id := range extractAttachmentIDsFromText(txt) {
				if seenIDs[id] {
					continue
				}
				seenIDs[id] = true
				if a, ok := findAttachment(inbox, id); ok {
					resolved = append(resolved, a)
				}
			}
		}
		if len(resolved) == 0 {
			return false, nil
		}
		return appendStagedNativeMedia(rawMap, inbox, resolved), nil
	}
	delete(rawMap, "attachments")

	list, _ := raw.([]interface{})
	var ids []string
	for _, it := range list {
		switch v := it.(type) {
		case map[string]interface{}:
			if id, ok := v["id"].(string); ok {
				ids = append(ids, id)
			}
		case string:
			ids = append(ids, v)
		}
	}
	atts, err := resolveAttachmentRefs(ids)
	if err != nil {
		return true, err
	}
	if len(atts) == 0 {
		return true, nil
	}

	appendStagedNativeMedia(rawMap, inbox, atts)

	block := formatAttachmentBlock(atts)
	appendBlock := func(s string) string {
		if strings.TrimSpace(s) == "" {
			return block
		}
		return strings.TrimRight(s, "\n") + "\n\n" + block
	}

	handled := false
	if items, ok := rawMap["items"].([]interface{}); ok && len(items) > 0 {
		// 文本块优先并入最后一个包含正文的非 scope 文本条目；
		// 避免把附件塞进多个斜杠命令之间用作分隔的纯空白条目 {"text": " "}。
		targetIdx := -1
		for i := len(items) - 1; i >= 0; i-- {
			m, ok := items[i].(map[string]interface{})
			if !ok {
				continue
			}
			if _, isScopeItem := m["item"]; isScopeItem {
				continue
			}
			t, hasText := m["text"].(string)
			if !hasText && m["text"] != nil {
				continue
			}
			if strings.TrimSpace(t) != "" {
				targetIdx = i
				break
			}
			// 末尾本身的空文本条目（不在其他 scope 条目之前）也可安全复用
			if i == len(items)-1 && targetIdx == -1 {
				targetIdx = i
			}
		}
		if targetIdx >= 0 {
			m := items[targetIdx].(map[string]interface{})
			t, _ := m["text"].(string)
			m["text"] = appendBlock(t)
			handled = true
		} else {
			// 只有 scope 条目（或仅含斜杠命令之间的空格分隔项）：在末尾追加文本条目承载附件块，
			// 并前置空格防止与紧邻的 "/cmd" 粘连破坏斜杠命令胶囊解析。
			prefix := ""
			if lastMap, ok := items[len(items)-1].(map[string]interface{}); ok {
				if _, isScopeItem := lastMap["item"]; isScopeItem {
					prefix = " "
				}
			}
			rawMap["items"] = append(items, map[string]interface{}{"text": prefix + block})
			handled = true
		}
	}
	if t, ok := rawMap["text"].(string); ok {
		rawMap["text"] = appendBlock(t)
		handled = true
	}
	if !handled {
		rawMap["items"] = []interface{}{map[string]interface{}{"text": block}}
		rawMap["text"] = block
	}
	return true, nil
}

// CleanupInbox removes attachments older than ttl and prunes empty month directories.
// It also prunes stale or consumed upstream media staging directories under .tmp/.
// It returns the number of expired attachment files removed.
func CleanupInbox(inbox string, ttl time.Duration) int {
	cutoff := time.Now().Add(-ttl)
	tmpTTL := 6 * time.Hour
	if ttl < tmpTTL {
		tmpTTL = ttl
	}
	tmpCutoff := time.Now().Add(-tmpTTL)
	tmpDir := filepath.Clean(filepath.Join(inbox, ".tmp"))
	removed := 0
	var bytesRemoved int64
	_ = filepath.WalkDir(inbox, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if filepath.Clean(path) == tmpDir {
				return filepath.SkipDir
			}
			return nil
		}
		if info, err := d.Info(); err == nil && info.ModTime().Before(cutoff) {
			sz := info.Size()
			if os.Remove(path) == nil {
				removed++
				bytesRemoved += sz
			}
		}
		return nil
	})
	if bytesRemoved > 0 {
		commitInboxBytes(inbox, -bytesRemoved, 0)
		invalidateInboxSizeCache(inbox)
	}
	if tmpEntries, err := os.ReadDir(tmpDir); err == nil {
		for _, e := range tmpEntries {
			entryPath := filepath.Join(tmpDir, e.Name())
			if e.IsDir() && strings.HasPrefix(e.Name(), "media-") {
				if os.Remove(entryPath) == nil {
					continue // pruned empty dir already consumed by upstream
				}
				if info, err := e.Info(); err == nil && info.ModTime().Before(tmpCutoff) {
					_ = os.RemoveAll(entryPath)
				}
				continue
			}
			if !e.IsDir() {
				if info, err := e.Info(); err == nil && info.ModTime().Before(tmpCutoff) {
					_ = os.Remove(entryPath)
				}
			}
		}
	}
	entries, _ := os.ReadDir(inbox)
	for _, e := range entries {
		if e.IsDir() && e.Name() != ".tmp" {
			_ = os.Remove(filepath.Join(inbox, e.Name())) // only succeeds when empty
		}
	}
	return removed
}

// StartInboxJanitor periodically cleans expired attachments and zip-entry temp cache until stop is closed.
func StartInboxJanitor(stop <-chan struct{}) {
	run := func() {
		if n := CleanupZipEntryCache(zipEntryCacheDir(), defaultZipCacheTTL, defaultZipCacheMaxBytes); n > 0 {
			slog.Info("[ZipCache] pruned cached zip entries", "count", n)
		}
		inbox, err := AttachmentInboxDir()
		if err != nil {
			return
		}
		ttl := time.Duration(envInt("MGY_INBOX_TTL_DAYS", defaultInboxTTLDays)) * 24 * time.Hour
		if n := CleanupInbox(inbox, ttl); n > 0 {
			slog.Info("[Attachments] expired files removed", "count", n)
		}
	}
	go func() {
		run()
		t := time.NewTicker(6 * time.Hour)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				run()
			case <-stop:
				return
			}
		}
	}()
}
