package proxy

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func buildTestZip(t *testing.T, path string, entries map[string][]byte) {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip create %s: %v", name, err)
		}
		if _, err := w.Write(content); err != nil {
			t.Fatalf("zip write %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatalf("write zip %s: %v", path, err)
	}
}

func TestIsWeChatArchiveName(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"聊天记录_20261118_003107.zip", true},
		{"聊天记录.zip", true},
		{"/tmp/93c0f67aa7e94bf7-聊天记录_20261118_003107.zip", false}, // caller strips 16-char hash prefix first; raw basename starts with 93c...
		{"archive.zip", false},
		{"聊天记录_20261118.txt", false},
	}
	for _, tc := range cases {
		if got := IsWeChatArchiveName(tc.name); got != tc.want {
			t.Errorf("IsWeChatArchiveName(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestParseWeChatZipFileMultiDayAndMedia(t *testing.T) {
	tmpHome := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(tmpHome); err == nil {
		tmpHome = resolved
	}
	t.Setenv("HOME", tmpHome)
	resetWorkspaceRootsForTest()
	t.Cleanup(resetWorkspaceRootsForTest)

	zipPath := filepath.Join(tmpHome, "93c0f67aa7e94bf7-聊天记录_20261118_003107.zip")
	chatTxt := "\ufeff·[聊天记录]\n\n" +
		"    ·嘿;-)\n" +
		"    2026年11月17日 22:22\n" +
		"    [动画表情] 红温\n\n" +
		"    ·嘿;-)\n" +
		"    2026年11月17日 22:23\n" +
		"    @蛤尔9000\u2005中文提示词：\n" +
		"    第一行内容\n\n" +
		"    ·陶九镇\n" +
		"    2026年11月17日 22:29\n" +
		"    [图片] \n\n" +
		"    ·蛤尔9000\n" +
		"    2026年11月17日 22:56\n" +
		"    [视频] 微信视频_202611180031_1.mp4\n\n" +
		"    ·陶九镇\n" +
		"    2026年11月18日 00:07\n" +
		"    [图片] 微信图片_202611180031_5.jpg\n"

	buildTestZip(t, zipPath, map[string][]byte{
		"聊天记录.txt": []byte(chatTxt),
		"聊天记录内的图片、视频和文件/微信视频_202611180031_1.mp4": []byte("FAKE_MP4_DATA_123456"),
		"聊天记录内的图片、视频和文件/微信图片_202611180031_5.jpg": []byte("FAKE_JPG_DATA"),
	})

	archive, err := ParseWeChatZipFile(zipPath)
	if err != nil {
		t.Fatalf("ParseWeChatZipFile error: %v", err)
	}
	if !archive.IsWeChatArchive {
		t.Fatalf("expected IsWeChatArchive=true")
	}
	if archive.FileName != "聊天记录_20261118_003107.zip" {
		t.Errorf("FileName = %q", archive.FileName)
	}
	if !archive.IsMultiDay || archive.DateRange != "2026年11月17日 至 2026年11月18日" {
		t.Errorf("IsMultiDay=%v DateRange=%q", archive.IsMultiDay, archive.DateRange)
	}
	if len(archive.Messages) != 5 {
		t.Fatalf("expected 5 messages, got %d", len(archive.Messages))
	}
	if archive.Messages[0].Kind != "sticker" || archive.Messages[0].Text != "红温" {
		t.Errorf("msg[0] = %+v", archive.Messages[0])
	}
	if archive.Messages[1].Kind != "text" || archive.Messages[1].Text != "@蛤尔9000 中文提示词：\n第一行内容" {
		t.Errorf("msg[1] = %+v", archive.Messages[1])
	}
	if archive.Messages[2].Kind != "missing_image" {
		t.Errorf("msg[2] = %+v", archive.Messages[2])
	}
	if archive.Messages[3].Kind != "video" || archive.Messages[3].MediaEntry != "聊天记录内的图片、视频和文件/微信视频_202611180031_1.mp4" {
		t.Errorf("msg[3] = %+v", archive.Messages[3])
	}
	if archive.Messages[4].Kind != "image" || archive.Messages[4].DisplayTime != "11月18日 00:07" {
		t.Errorf("msg[4] = %+v", archive.Messages[4])
	}
}

func TestHandleWeChatChatArchiveAndZipEntry(t *testing.T) {
	tmpHome := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(tmpHome); err == nil {
		tmpHome = resolved
	}
	t.Setenv("HOME", tmpHome)
	resetWorkspaceRootsForTest()
	t.Cleanup(resetWorkspaceRootsForTest)

	zipPath := filepath.Join(tmpHome, "聊天记录_20261117_125524.zip")
	buildTestZip(t, zipPath, map[string][]byte{
		"聊天记录.txt": []byte("·[聊天记录]\n\n    ·测试用户\n    2026年11月17日 10:13\n    [图片] pic1.jpg\n"),
		"聊天记录内的图片、视频和文件/pic1.jpg": []byte("0123456789ABCDEF"),
	})

	p := &Proxy{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/files/wechat-chat?uri="+url.QueryEscape(zipPath), nil)
	rec := httptest.NewRecorder()
	p.HandleWeChatChatArchive(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("HandleWeChatChatArchive status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out WeChatChatArchive
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}
	if !out.IsWeChatArchive || out.IsMultiDay || out.DateRange != "2026年11月17日" {
		t.Fatalf("unexpected archive metadata: %+v", out)
	}
	if len(out.Messages) != 1 || out.Messages[0].DisplayTime != "10:13" {
		t.Fatalf("unexpected messages: %+v", out.Messages)
	}

	// Range request against zip-entry
	entryReq := httptest.NewRequest(http.MethodGet,
		"/api/v1/files/zip-entry?uri="+url.QueryEscape(zipPath)+"&entry="+url.QueryEscape(out.Messages[0].MediaEntry),
		nil)
	entryReq.Header.Set("Range", "bytes=4-9")
	entryRec := httptest.NewRecorder()
	p.HandleZipEntry(entryRec, entryReq)
	if entryRec.Code != http.StatusPartialContent {
		t.Fatalf("HandleZipEntry Range status=%d body=%s", entryRec.Code, entryRec.Body.String())
	}
	if got := entryRec.Body.String(); got != "456789" {
		t.Fatalf("Range body = %q, want %q", got, "456789")
	}

	// Non-WeChat zip fallback
	plainZip := filepath.Join(tmpHome, "other.zip")
	buildTestZip(t, plainZip, map[string][]byte{"readme.md": []byte("# hello")})
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/files/wechat-chat?uri="+url.QueryEscape(plainZip), nil)
	rec2 := httptest.NewRecorder()
	p.HandleWeChatChatArchive(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("non-wechat zip status=%d", rec2.Code)
	}
	var out2 WeChatChatArchive
	_ = json.Unmarshal(rec2.Body.Bytes(), &out2)
	if out2.IsWeChatArchive {
		t.Fatalf("expected IsWeChatArchive=false for non-wechat zip")
	}
	if len(out2.Entries) != 1 || out2.Entries[0].Name != "readme.md" {
		t.Fatalf("expected fallback entries, got %+v", out2.Entries)
	}

	// Moved Inbox attachment fallback -> ~/.gemini/antigravity/brain/<cascadeID>/.user_uploaded/<id16>-<name>
	inboxPath := filepath.Join(tmpHome, "Multigravity", "Inbox", "2026-11", "2da0790d771e9c2c-聊天记录_20261118_010406.zip")
	movedPath := filepath.Join(tmpHome, ".gemini", "antigravity", "brain", "a899f51f-test", ".user_uploaded", "2da0790d771e9c2c-聊天记录_20261118_010406.zip")
	if err := os.MkdirAll(filepath.Dir(movedPath), 0o755); err != nil {
		t.Fatalf("mkdir movedPath: %v", err)
	}
	buildTestZip(t, movedPath, map[string][]byte{
		"聊天记录.txt": []byte("·[聊天记录]\n\n    ·陶九镇\n    2026年11月18日 01:04\n    测试搬迁路径\n"),
	})
	reqMoved := httptest.NewRequest(http.MethodGet,
		"/api/v1/files/wechat-chat?uri="+url.QueryEscape(inboxPath)+"&cascade_id=a899f51f-test",
		nil)
	recMoved := httptest.NewRecorder()
	p.HandleWeChatChatArchive(recMoved, reqMoved)
	if recMoved.Code != http.StatusOK {
		t.Fatalf("moved Inbox zip status=%d body=%s", recMoved.Code, recMoved.Body.String())
	}
	var outMoved WeChatChatArchive
	_ = json.Unmarshal(recMoved.Body.Bytes(), &outMoved)
	if !outMoved.IsWeChatArchive || len(outMoved.Messages) != 1 || outMoved.Messages[0].Text != "测试搬迁路径" {
		t.Fatalf("unexpected moved archive result: %+v", outMoved)
	}
}
