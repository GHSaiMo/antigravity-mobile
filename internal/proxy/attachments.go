package proxy

import (
	"bytes"
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
	"strconv"
	"strings"
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
	defaultMaxUploadMB   = 50
	defaultInboxQuotaMB  = 2048
	defaultInboxTTLDays  = 30
	maxAttachmentsPerMsg = 5
	maxAttachmentsTotal  = 100 * 1024 * 1024
	maxAttachmentNameLen = 120
	attachmentIDLen      = 16
)

var attachmentIDPattern = regexp.MustCompile(`^[0-9a-f]{16}$`)

// attachmentExts is the allow-list of file extensions (lower-case, without dot) and their kind.
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
	add("archive", "zip", "tar", "gz", "tgz", "7z")
	add("code",
		"py", "js", "mjs", "cjs", "ts", "tsx", "jsx", "go", "rs", "java", "kt", "kts", "swift", "m", "mm",
		"c", "cc", "cpp", "cxx", "h", "hpp", "cs", "rb", "php", "lua", "dart", "scala", "r", "pl",
		"sh", "bash", "zsh", "bat", "ps1", "sql", "vue", "svelte", "gradle", "proto",
		"json", "jsonl", "yaml", "yml", "toml", "ini", "cfg", "conf", "xml", "html", "htm", "css", "scss",
		"less", "ipynb", "dockerfile", "makefile")
	return m
}

// attachmentKind returns the kind for a file name, or "" when the extension is not allowed.
func attachmentKind(name string) string {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))
	if ext == "" {
		// Extension-less well-known names (Dockerfile, Makefile).
		ext = strings.ToLower(name)
	}
	return attachmentExts[ext]
}

// looksLikeExecutable reports whether the head of a file carries a native-executable magic
// number (PE, ELF, Mach-O, dex). It blocks binaries renamed to an allowed extension.
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

func mimeForName(name string) string {
	if t := mime.TypeByExtension(strings.ToLower(filepath.Ext(name))); t != "" {
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

func inboxSize(inbox string) int64 {
	var total int64
	_ = filepath.WalkDir(inbox, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, err := d.Info(); err == nil {
			total += info.Size()
		}
		return nil
	})
	return total
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
	if kind == "" {
		writeAttachmentError(w, http.StatusUnsupportedMediaType, "unsupported_type",
			"不支持的文件类型: "+filepath.Ext(name))
		return
	}

	maxBytes := int64(envInt("MGY_MAX_UPLOAD_MB", defaultMaxUploadMB)) * 1024 * 1024
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
	if looksLikeExecutable(head.buf) {
		slog.Warn("[Attachments] rejected executable payload", "name", name)
		writeAttachmentError(w, http.StatusUnsupportedMediaType, "executable_rejected", "不允许上传可执行文件")
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
		monthDir := filepath.Join(inbox, time.Now().Format("2006-01"))
		if err := os.MkdirAll(monthDir, 0o700); err != nil {
			writeAttachmentError(w, http.StatusInternalServerError, "write_failed", err.Error())
			return
		}
		finalPath := filepath.Join(monthDir, id+"-"+name)
		if err := os.Rename(tmpPath, finalPath); err != nil {
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
		out = append(out, a)
	}
	if total > maxAttachmentsTotal {
		return nil, fmt.Errorf("附件总大小超过 %dMB", maxAttachmentsTotal/1024/1024)
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

// applyAttachmentsToMessage consumes the "attachments" field of a SendUserCascadeMessage body and
// appends the attachment block to both "text" and "items[0].text". It returns whether the body
// was changed. The "attachments" key is always removed so it never reaches the language server.
func applyAttachmentsToMessage(rawMap map[string]interface{}) (bool, error) {
	raw, present := rawMap["attachments"]
	if !present {
		return false, nil
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
	block := formatAttachmentBlock(atts)

	appendBlock := func(s string) string {
		if strings.TrimSpace(s) == "" {
			return block
		}
		return strings.TrimRight(s, "\n") + "\n\n" + block
	}

	handled := false
	if items, ok := rawMap["items"].([]interface{}); ok && len(items) > 0 {
		// 文本块并入第一个「文本条目」；斜杠命令等 scope 条目（带 item 字段）不能同时再带 text。
		for _, it := range items {
			m, ok := it.(map[string]interface{})
			if !ok {
				continue
			}
			if _, isScopeItem := m["item"]; isScopeItem {
				continue
			}
			if t, ok := m["text"].(string); ok || m["text"] == nil {
				m["text"] = appendBlock(t)
				handled = true
			}
			break
		}
		if !handled {
			// 只有 scope 条目（例如单独一个斜杠命令）：补一个文本条目承载附件块
			rawMap["items"] = append(items, map[string]interface{}{"text": block})
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
// It returns the number of files removed.
func CleanupInbox(inbox string, ttl time.Duration) int {
	cutoff := time.Now().Add(-ttl)
	removed := 0
	_ = filepath.WalkDir(inbox, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, err := d.Info(); err == nil && info.ModTime().Before(cutoff) {
			if os.Remove(path) == nil {
				removed++
			}
		}
		return nil
	})
	entries, _ := os.ReadDir(inbox)
	for _, e := range entries {
		if e.IsDir() && e.Name() != ".tmp" {
			_ = os.Remove(filepath.Join(inbox, e.Name())) // only succeeds when empty
		}
	}
	return removed
}

// StartInboxJanitor periodically cleans expired attachments until stop is closed.
func StartInboxJanitor(stop <-chan struct{}) {
	run := func() {
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
