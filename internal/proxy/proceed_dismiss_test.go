package proxy

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

type recordingAlertClearSink struct {
	mu     sync.Mutex
	events []struct {
		cascadeID   string
		dismissKey  string
		finalStatus string
	}
	done chan struct{}
}

func (s *recordingAlertClearSink) OnTrajectoryUpdate(*TrajectoryDetails) {}

func (s *recordingAlertClearSink) OnCascadeAlertCleared(cascadeID, dismissKey, finalStatus string) {
	s.mu.Lock()
	s.events = append(s.events, struct {
		cascadeID   string
		dismissKey  string
		finalStatus string
	}{cascadeID, dismissKey, finalStatus})
	s.mu.Unlock()
	if s.done != nil {
		select {
		case s.done <- struct{}{}:
		default:
		}
	}
}

func TestProceedDismissPersistenceAndStepAdvancement(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cid := "11111111-1111-1111-1111-111111111111"

	if IsProceedDismissed(cid, 122) {
		t.Fatalf("expected fresh cascade not dismissed")
	}

	MarkProceedDismissed(cid, 122)
	if !IsProceedDismissed(cid, 122) {
		t.Fatalf("expected cascade dismissed at step 122")
	}
	if !IsProceedDismissed(cid, 100) {
		t.Fatalf("expected cascade dismissed for step <= 122")
	}
	if IsProceedDismissed(cid, 123) {
		t.Fatalf("expected dismissal to auto-expire when conversation advances past step 122")
	}

	// Simulate gateway restart by resetting in-memory loaded flag and verifying reload from disk.
	dismissedProceedMu.Lock()
	dismissedProceedLoaded = false
	dismissedProceedSteps = nil
	dismissedProceedMu.Unlock()

	if !IsProceedDismissed(cid, 122) {
		t.Fatalf("expected dismissal to persist across reload from dismissed_proceed.json")
	}

	ClearProceedDismissed(cid)
	if IsProceedDismissed(cid, 122) {
		t.Fatalf("expected ClearProceedDismissed to remove persisted record")
	}
}

func TestHandleDismissProceedAndArchiveTriggerAlertClear(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cid := "22222222-2222-2222-2222-222222222222"

	p := &Proxy{}
	sink := &recordingAlertClearSink{done: make(chan struct{}, 4)}
	p.SetNotificationSink(sink)

	// 1. POST /gateway/cascade/proceed/dismiss
	body, _ := json.Marshal(map[string]interface{}{
		"cascadeId":  cid,
		"totalSteps": 122,
	})
	req := httptest.NewRequest(http.MethodPost, "/gateway/cascade/proceed/dismiss", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	p.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from /gateway/cascade/proceed/dismiss, got %d: %s", rec.Code, rec.Body.String())
	}
	if !IsProceedDismissed(cid, 122) {
		t.Fatalf("expected cascade to be marked dismissed at step 122")
	}
	select {
	case <-sink.done:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for OnCascadeAlertCleared after dismiss")
	}

	// 2. POST /gateway/cascade/archive (archived=true) should also clear alerts
	archBody, _ := json.Marshal(map[string]interface{}{
		"cascadeId": cid,
		"archived":  true,
	})
	archReq := httptest.NewRequest(http.MethodPost, "/gateway/cascade/archive", bytes.NewReader(archBody))
	archRec := httptest.NewRecorder()
	p.ServeHTTP(archRec, archReq)

	if archRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from /gateway/cascade/archive, got %d: %s", archRec.Code, archRec.Body.String())
	}
	select {
	case <-sink.done:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for OnCascadeAlertCleared after archive")
	}

	sink.mu.Lock()
	defer sink.mu.Unlock()
	if len(sink.events) != 2 {
		t.Fatalf("expected 2 alert-clear events, got %d", len(sink.events))
	}
	if sink.events[0].cascadeID != cid || sink.events[0].dismissKey != "proceed:122" {
		t.Fatalf("unexpected dismiss event: %+v", sink.events[0])
	}
	if sink.events[1].cascadeID != cid || sink.events[1].dismissKey != "archived" {
		t.Fatalf("unexpected archive event: %+v", sink.events[1])
	}
}

func TestIsCascadeArchivedChecksSummaryAndDisk(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cid := "33333333-3333-3333-3333-333333333333"

	if IsCascadeArchived(cid, map[string]interface{}{}) {
		t.Fatalf("expected unarchived session to return false")
	}
	if !IsCascadeArchived(cid, map[string]interface{}{
		"annotations": map[string]interface{}{"archived": true},
	}) {
		t.Fatalf("expected summary annotations.archived=true to return true")
	}

	writeAnnotationArchived(cid, true)
	if !IsCascadeArchived(cid, map[string]interface{}{}) {
		t.Fatalf("expected disk annotation archived:true to return true even when summary omits it")
	}
	writeAnnotationArchived(cid, false)
	if IsCascadeArchived(cid, map[string]interface{}{}) {
		t.Fatalf("expected unarchived disk annotation to return false")
	}
}
