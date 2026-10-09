package proxy

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func setupInbox(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "inbox")
	t.Setenv("MGY_INBOX_DIR", dir)
	return dir
}

func upload(t *testing.T, name string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/attachments", bytes.NewReader(body))
	req.Header.Set("X-File-Name", url.PathEscape(name))
	rec := httptest.NewRecorder()
	(&Proxy{}).HandleAttachmentUpload(rec, req)
	return rec
}

func decodeAttachment(t *testing.T, rec *httptest.ResponseRecorder) Attachment {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var a Attachment
	if err := json.Unmarshal(rec.Body.Bytes(), &a); err != nil {
		t.Fatal(err)
	}
	return a
}

func TestAttachmentUploadStoresFile(t *testing.T) {
	inbox := setupInbox(t)
	a := decodeAttachment(t, upload(t, "季度 报告🙂.docx", []byte("PK\x03\x04hello")))

	if a.Kind != "document" || a.Size != 9 || len(a.ID) != attachmentIDLen {
		t.Fatalf("unexpected attachment: %+v", a)
	}
	if !strings.HasPrefix(a.Path, inbox) || !strings.HasSuffix(a.Path, "-季度 报告🙂.docx") {
		t.Fatalf("unexpected path: %s", a.Path)
	}
	st, err := os.Stat(a.Path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Errorf("file mode = %v, want 0600", st.Mode().Perm())
	}
	if tmps, _ := os.ReadDir(filepath.Join(inbox, ".tmp")); len(tmps) != 0 {
		t.Errorf("temp files left behind: %d", len(tmps))
	}
}

func TestAttachmentUploadDeduplicates(t *testing.T) {
	setupInbox(t)
	a := decodeAttachment(t, upload(t, "a.txt", []byte("same bytes")))
	b := decodeAttachment(t, upload(t, "a.txt", []byte("same bytes")))
	if a.ID != b.ID || a.Path != b.Path {
		t.Fatalf("expected dedup, got %+v vs %+v", a, b)
	}
}

func TestAttachmentUploadRejects(t *testing.T) {
	setupInbox(t)
	cases := []struct {
		name   string
		file   string
		body   []byte
		status int
	}{
		{"unsupported ext", "tool.exe", []byte("hi"), http.StatusUnsupportedMediaType},
		{"no ext", "README", []byte("hi"), http.StatusUnsupportedMediaType},
		{"empty", "a.txt", nil, http.StatusBadRequest},
		{"elf renamed", "run.txt", []byte("\x7fELF\x02\x01"), http.StatusUnsupportedMediaType},
		{"pe renamed", "run.pdf", []byte("MZ\x90\x00"), http.StatusUnsupportedMediaType},
		{"macho renamed", "run.zip", []byte{0xcf, 0xfa, 0xed, 0xfe, 0, 0}, http.StatusUnsupportedMediaType},
	}
	for _, c := range cases {
		if rec := upload(t, c.file, c.body); rec.Code != c.status {
			t.Errorf("%s: status = %d, want %d (%s)", c.name, rec.Code, c.status, rec.Body.String())
		}
	}
}

func TestAttachmentUploadSizeLimit(t *testing.T) {
	setupInbox(t)
	t.Setenv("MGY_MAX_UPLOAD_MB", "1")

	// Declared Content-Length over the limit is rejected up front.
	if rec := upload(t, "big.txt", bytes.Repeat([]byte("a"), 1<<20+1)); rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("over-limit status = %d, want 413", rec.Code)
	}
	// Exactly at the limit is accepted.
	if rec := upload(t, "ok.txt", bytes.Repeat([]byte("a"), 1<<20)); rec.Code != http.StatusOK {
		t.Errorf("at-limit status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
}

func TestAttachmentUploadStreamingLimitWithoutContentLength(t *testing.T) {
	inbox := setupInbox(t)
	t.Setenv("MGY_MAX_UPLOAD_MB", "1")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/attachments",
		struct{ *bytes.Reader }{bytes.NewReader(bytes.Repeat([]byte("a"), 2<<20))})
	req.ContentLength = -1
	req.Header.Set("X-File-Name", "chunked.txt")
	rec := httptest.NewRecorder()
	(&Proxy{}).HandleAttachmentUpload(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", rec.Code)
	}
	if tmps, _ := os.ReadDir(filepath.Join(inbox, ".tmp")); len(tmps) != 0 {
		t.Errorf("temp files left behind after rejection")
	}
}

func TestAttachmentUploadQuota(t *testing.T) {
	setupInbox(t)
	t.Setenv("MGY_INBOX_QUOTA_MB", "1")
	t.Setenv("MGY_MAX_UPLOAD_MB", "5")
	decodeAttachment(t, upload(t, "fill.txt", bytes.Repeat([]byte("x"), 1<<20)))
	if rec := upload(t, "more.txt", []byte("more")); rec.Code != http.StatusInsufficientStorage {
		t.Errorf("status = %d, want 507", rec.Code)
	}
}

func TestSanitizeAttachmentName(t *testing.T) {
	cases := map[string]string{
		"../../etc/passwd":                "passwd",
		`C:\Users\x\report.docx`:          "report.docx",
		"a/b/c.md":                        "c.md",
		"..":                              "file",
		"":                                "file",
		"  .hidden.txt  ":                 "hidden.txt",
		"bad\x00name\n.txt":               "badname.txt",
		"rtl\u202egpj.exe":                "rtlgpj.exe",
		"周报.xlsx":                         "周报.xlsx",
		strings.Repeat("长", 100) + ".pdf": "",
	}
	for in, want := range cases {
		got := sanitizeAttachmentName(in)
		if want == "" { // length-clamped case
			if len(got) > maxAttachmentNameLen || !strings.HasSuffix(got, ".pdf") {
				t.Errorf("long name not clamped sanely: %q (%d bytes)", got, len(got))
			}
			continue
		}
		if got != want {
			t.Errorf("sanitize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestApplyAttachmentsToMessage(t *testing.T) {
	setupInbox(t)
	a := decodeAttachment(t, upload(t, "plan.md", []byte("# plan")))

	msg := map[string]interface{}{
		"cascadeId":   "c1",
		"text":        "看看这个",
		"items":       []interface{}{map[string]interface{}{"text": "看看这个"}},
		"attachments": []interface{}{map[string]interface{}{"id": a.ID}},
	}
	changed, err := applyAttachmentsToMessage(msg)
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	if _, still := msg["attachments"]; still {
		t.Error("attachments key must be removed before forwarding")
	}
	itemText := msg["items"].([]interface{})[0].(map[string]interface{})["text"].(string)
	for _, got := range []string{msg["text"].(string), itemText} {
		if !strings.HasPrefix(got, "看看这个\n\n"+attachmentHeader) || !strings.Contains(got, "- "+a.Path+" (md, 6 B)") {
			t.Errorf("unexpected text: %q", got)
		}
	}
}

func slashItem(name string) map[string]interface{} {
	return map[string]interface{}{"item": map[string]interface{}{"slashCommand": map[string]interface{}{"info": map[string]interface{}{"name": name}}}}
}

// 斜杠命令条目排在最前时，附件块必须并入后面的文本条目，而不是塞进命令条目（text 与 item 是互斥分支）。
func TestApplyAttachmentsWithSlashCommandFirst(t *testing.T) {
	setupInbox(t)
	a := decodeAttachment(t, upload(t, "plan.md", []byte("# plan")))
	msg := map[string]interface{}{
		"cascadeId":   "c1",
		"items":       []interface{}{slashItem("plan"), map[string]interface{}{"text": " 看这个"}},
		"attachments": []interface{}{map[string]interface{}{"id": a.ID}},
	}
	if _, err := applyAttachmentsToMessage(msg); err != nil {
		t.Fatal(err)
	}
	items := msg["items"].([]interface{})
	if len(items) != 2 {
		t.Fatalf("items = %v", items)
	}
	first := items[0].(map[string]interface{})
	if _, hasText := first["text"]; hasText {
		t.Errorf("slash item must not gain a text field: %v", first)
	}
	if got := items[1].(map[string]interface{})["text"].(string); !strings.HasPrefix(got, " 看这个\n\n"+attachmentHeader) || !strings.Contains(got, a.Path) {
		t.Errorf("attachment block not merged into the text item: %q", got)
	}
}

func TestApplyAttachmentsWithSlashCommandOnly(t *testing.T) {
	setupInbox(t)
	a := decodeAttachment(t, upload(t, "x.csv", []byte("a,b")))
	msg := map[string]interface{}{
		"cascadeId":   "c1",
		"items":       []interface{}{slashItem("plan")},
		"attachments": []interface{}{map[string]interface{}{"id": a.ID}},
	}
	if _, err := applyAttachmentsToMessage(msg); err != nil {
		t.Fatal(err)
	}
	items := msg["items"].([]interface{})
	if len(items) != 2 {
		t.Fatalf("a text item must be appended after the command: %v", items)
	}
	if _, hasText := items[0].(map[string]interface{})["text"]; hasText {
		t.Errorf("slash item polluted: %v", items[0])
	}
	if !strings.Contains(items[1].(map[string]interface{})["text"].(string), a.Path) {
		t.Errorf("appended text item missing the block: %v", items[1])
	}
}

func TestApplyAttachmentsOnlyNoText(t *testing.T) {
	setupInbox(t)
	a := decodeAttachment(t, upload(t, "x.csv", []byte("a,b")))
	msg := map[string]interface{}{"cascadeId": "c1", "attachments": []interface{}{map[string]interface{}{"id": a.ID}}}
	if _, err := applyAttachmentsToMessage(msg); err != nil {
		t.Fatal(err)
	}
	items := msg["items"].([]interface{})
	if len(items) != 1 || !strings.Contains(items[0].(map[string]interface{})["text"].(string), a.Path) {
		t.Errorf("items not synthesized: %v", msg)
	}
}

func TestApplyAttachmentsErrors(t *testing.T) {
	setupInbox(t)
	mk := func(ids ...string) map[string]interface{} {
		var l []interface{}
		for _, id := range ids {
			l = append(l, map[string]interface{}{"id": id})
		}
		return map[string]interface{}{"text": "t", "attachments": l}
	}
	if _, err := applyAttachmentsToMessage(mk("0123456789abcdef")); err == nil {
		t.Error("unknown id should fail")
	}
	if _, err := applyAttachmentsToMessage(mk("../../etc/passwd")); err == nil {
		t.Error("path-like id should fail")
	}
	if _, err := applyAttachmentsToMessage(mk("a", "b", "c", "d", "e", "f")); err == nil {
		t.Error("too many attachments should fail")
	}
	if changed, err := applyAttachmentsToMessage(map[string]interface{}{"text": "t"}); changed || err != nil {
		t.Error("no attachments key must be a no-op")
	}
}

func TestCleanupInbox(t *testing.T) {
	inbox := setupInbox(t)
	a := decodeAttachment(t, upload(t, "old.txt", []byte("old")))
	b := decodeAttachment(t, upload(t, "new.txt", []byte("new")))
	old := time.Now().Add(-40 * 24 * time.Hour)
	if err := os.Chtimes(a.Path, old, old); err != nil {
		t.Fatal(err)
	}
	if n := CleanupInbox(inbox, 30*24*time.Hour); n != 1 {
		t.Fatalf("removed %d, want 1", n)
	}
	if _, err := os.Stat(a.Path); !os.IsNotExist(err) {
		t.Error("expired file should be gone")
	}
	if _, err := os.Stat(b.Path); err != nil {
		t.Error("fresh file must remain")
	}
}
