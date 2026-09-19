package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"antigravity-mobile/internal/inspector"
)

func setupTestHomeWithCascade(t *testing.T, cascadeID, title string, viewTime time.Time) string {
	t.Helper()
	ClearNewestAnnotationCache()
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("USERPROFILE", tempHome)

	// Create brain directory
	brainDir := filepath.Join(tempHome, ".gemini", "antigravity", "brain", cascadeID)
	if err := os.MkdirAll(brainDir, 0755); err != nil {
		t.Fatalf("Failed to create test brain dir: %v", err)
	}

	// Create annotations directory and file
	annDir := filepath.Join(tempHome, ".gemini", "antigravity", "annotations")
	if err := os.MkdirAll(annDir, 0755); err != nil {
		t.Fatalf("Failed to create test annotations dir: %v", err)
	}

	if title != "" {
		pbtxt := filepath.Join(annDir, cascadeID+".pbtxt")
		content := "title: \"" + title + "\" last_user_view_time: { seconds: " +
			timeToSecStr(viewTime) + " nanos: 0 }\n"
		if err := os.WriteFile(pbtxt, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to write annotation file: %v", err)
		}
	}

	return tempHome
}

func timeToSecStr(t time.Time) string {
	return time.Duration(t.Unix()).String()[:0] + string([]byte(time.Unix(t.Unix(), 0).Format("20060102150405")))[:0] +
		jsonNumber(t.Unix())
}

func jsonNumber(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func TestUnifiedCursor_ArbitratePriority(t *testing.T) {
	tempHome := setupTestHomeWithCascade(t, "cascade-desktop-1", "Desktop Task", time.Now().Add(-10*time.Minute))
	// Also create brain dir for mobile cascade
	mobileBrain := filepath.Join(tempHome, ".gemini", "antigravity", "brain", "cascade-mobile-1")
	_ = os.MkdirAll(mobileBrain, 0755)

	mockInsp := &stubDiscoverer{info: &inspector.InstanceInfo{
		Port:      12345,
		CSRFToken: "test-token",
		IsHealthy: true,
	}}
	p := NewProxy(mockInsp)
	p.mobileStickyDuration = 10 * time.Minute

	// Scenario 1: No focus set -> falls back to scanning disk annotations
	cursor := p.ArbitrateCursor()
	if cursor == nil {
		t.Fatalf("Expected cursor from disk scan, got nil")
	}
	if cursor.CascadeID != "cascade-desktop-1" || cursor.Source != CursorSourceDesktop {
		t.Errorf("Expected desktop cursor 'cascade-desktop-1', got %+v", cursor)
	}

	// Scenario 2: Mobile focus reported (more recent than desktop) -> Mobile Sticky
	p.SetMobileFocus("cascade-mobile-1", "Mobile Task")
	cursor = p.ArbitrateCursor()
	if cursor == nil {
		t.Fatalf("Expected mobile cursor, got nil")
	}
	if cursor.CascadeID != "cascade-mobile-1" || cursor.Source != CursorSourceMobile || !cursor.IsSticky {
		t.Errorf("Expected mobile sticky cursor, got %+v", cursor)
	}

	// Scenario 3: Mobile active stream connected -> Mobile Active (not sticky)
	p.SetActiveStream("cascade-mobile-1", "Mobile Task")
	cursor = p.ArbitrateCursor()
	if cursor == nil {
		t.Fatalf("Expected mobile active cursor, got nil")
	}
	if cursor.CascadeID != "cascade-mobile-1" || cursor.IsSticky {
		t.Errorf("Expected mobile active (is_sticky=false) cursor, got %+v", cursor)
	}
	p.ClearActiveStream("cascade-mobile-1")

	// Scenario 4: Desktop focus updated to newer than mobile -> Desktop takes over
	newerTime := time.Now().Add(1 * time.Minute)
	p.SetDesktopFocus("cascade-desktop-1", "Desktop Task New", newerTime)
	cursor = p.ArbitrateCursor()
	if cursor == nil {
		t.Fatalf("Expected desktop cursor, got nil")
	}
	if cursor.CascadeID != "cascade-desktop-1" || cursor.Source != CursorSourceDesktop {
		t.Errorf("Expected desktop cursor after desktop focus update, got %+v", cursor)
	}
}

func TestUnifiedCursor_GhostSessionFiltering(t *testing.T) {
	ClearNewestAnnotationCache()
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("USERPROFILE", tempHome)

	mockInsp := &stubDiscoverer{}
	p := NewProxy(mockInsp)

	// Ghost session: No brain directory created
	p.SetMobileFocus("ghost-cascade", "Ghost Title")
	p.SetDesktopFocus("ghost-desktop", "Ghost Desktop", time.Now())

	cursor := p.ArbitrateCursor()
	if cursor != nil {
		t.Errorf("Expected nil cursor for ghost sessions without brain dir, got %+v", cursor)
	}
}

func TestUnifiedCursor_AntiReflectionProtection(t *testing.T) {
	tempHome := setupTestHomeWithCascade(t, "mobile-session-99", "Mobile App Work", time.Now())
	mockInsp := &stubDiscoverer{}
	p := NewProxy(mockInsp)

	// 1. Setting mobile focus should establish anti-reflection suppression window
	p.SetMobileFocus("mobile-session-99", "Mobile App Work")

	p.cursorMu.RLock()
	suppressUntil := p.suppressDesktopFocusUntil
	p.cursorMu.RUnlock()

	if time.Until(suppressUntil) < 1*time.Second {
		t.Errorf("Expected suppressDesktopFocusUntil to be >= 1s into the future, got %v", time.Until(suppressUntil))
	}

	// 2. Start watcher with short context and verify it does NOT overwrite mobile session with desktop
	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Millisecond)
	defer cancel()

	// Write an annotation modification to mobile-session-99
	annPath := filepath.Join(tempHome, ".gemini", "antigravity", "annotations", "mobile-session-99.pbtxt")
	_ = os.WriteFile(annPath, []byte("title: \"Mobile App Work\" last_user_view_time: { seconds: 1999999999 nanos: 0 }\n"), 0644)

	go p.StartDesktopFocusWatcher(ctx)
	time.Sleep(350 * time.Millisecond)

	// Since suppression is active, desktop focus must not have overwritten it
	p.cursorMu.RLock()
	desktopID := p.desktopCascadeID
	p.cursorMu.RUnlock()

	if desktopID == "mobile-session-99" {
		t.Errorf("Anti-reflection failure: mobile read receipt triggered desktop focus switch!")
	}
}

func TestHandleFocusSessionEndpoint(t *testing.T) {
	tempHome := setupTestHomeWithCascade(t, "cascade-test-endpoint", "Endpoint Title", time.Now())
	_ = tempHome

	mockInsp := &stubDiscoverer{}
	p := NewProxy(mockInsp)

	// 1. Invalid method (GET)
	reqGet := httptest.NewRequest(http.MethodGet, "/gateway/cascade/focus", nil)
	recGet := httptest.NewRecorder()
	p.ServeHTTP(recGet, reqGet)
	if recGet.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected 405 Method Not Allowed, got %d", recGet.Code)
	}

	// 2. Missing cascadeId
	reqBad := httptest.NewRequest(http.MethodPost, "/gateway/cascade/focus", bytes.NewBufferString(`{"source":"ios"}`))
	recBad := httptest.NewRecorder()
	p.ServeHTTP(recBad, reqBad)
	if recBad.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 Bad Request, got %d", recBad.Code)
	}

	// 3. Valid focus request
	reqOk := httptest.NewRequest(http.MethodPost, "/gateway/cascade/focus", bytes.NewBufferString(`{"cascadeId":"cascade-test-endpoint","source":"ios"}`))
	recOk := httptest.NewRecorder()
	p.ServeHTTP(recOk, reqOk)
	if recOk.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK, got %d: %s", recOk.Code, recOk.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(recOk.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response JSON: %v", err)
	}
	if resp["cascade_id"] != "cascade-test-endpoint" {
		t.Errorf("Expected cascade_id 'cascade-test-endpoint', got %v", resp["cascade_id"])
	}

	// 4. Verify gateway status reflects the unified cursor
	reqStatus := httptest.NewRequest(http.MethodGet, "/gateway/status", nil)
	recStatus := httptest.NewRecorder()
	p.ServeHTTP(recStatus, reqStatus)
	if recStatus.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK for /gateway/status, got %d", recStatus.Code)
	}

	var statusResp GatewayStatus
	if err := json.Unmarshal(recStatus.Body.Bytes(), &statusResp); err != nil {
		t.Fatalf("Failed to parse status response: %v", err)
	}
	if statusResp.UnifiedCursor == nil {
		t.Fatalf("Expected non-nil unified_cursor in /gateway/status")
	}
	if statusResp.UnifiedCursor.CascadeID != "cascade-test-endpoint" {
		t.Errorf("Expected unified_cursor cascadeId 'cascade-test-endpoint', got %s", statusResp.UnifiedCursor.CascadeID)
	}
	if statusResp.UnifiedCursor.Source != CursorSourceMobile {
		t.Errorf("Expected source 'mobile', got %s", statusResp.UnifiedCursor.Source)
	}
}
