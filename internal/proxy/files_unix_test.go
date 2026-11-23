//go:build !windows

package proxy

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"syscall"
	"testing"
)

func TestHandleFileRaw_TempDirFIFODoesNotHang(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ALLOWED_WORKSPACE_ROOTS", "")
	t.Setenv("MULTIGRAVITY_ALLOWED_WORKSPACE_ROOTS", "")
	resetWorkspaceRootsForTest()
	t.Cleanup(resetWorkspaceRootsForTest)

	fifoPath := filepath.Join(t.TempDir(), "test.fifo")
	if err := syscall.Mkfifo(fifoPath, 0o600); err != nil {
		t.Fatalf("failed to create fifo: %v", err)
	}

	p := &Proxy{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/files/raw?uri="+url.QueryEscape(fifoPath), nil)
	rr := httptest.NewRecorder()
	p.HandleFileRaw(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for non-regular FIFO file, got %d: %s", rr.Code, rr.Body.String())
	}
}
