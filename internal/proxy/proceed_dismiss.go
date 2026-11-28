package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"antigravity-mobile/internal/config"
)

type dismissedProceedStore struct {
	Version int            `json:"version"`
	Steps   map[string]int `json:"steps"`
}

var (
	dismissedProceedMu       sync.RWMutex
	dismissedProceedLoaded   bool
	dismissedProceedFilePath string
	dismissedProceedSteps    map[string]int
)

func getDismissedProceedFilePath() string {
	return filepath.Join(config.GetDataDir(), "dismissed_proceed.json")
}

func ensureDismissedProceedLoadedLocked() {
	path := getDismissedProceedFilePath()
	if dismissedProceedLoaded && dismissedProceedFilePath == path {
		return
	}
	dismissedProceedFilePath = path
	dismissedProceedSteps = make(map[string]int)
	dismissedProceedLoaded = true

	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var store dismissedProceedStore
	if err := json.Unmarshal(data, &store); err != nil || store.Steps == nil {
		return
	}
	for cid, step := range store.Steps {
		if cid = strings.TrimSpace(cid); cid != "" {
			dismissedProceedSteps[cid] = step
		}
	}
}

func saveDismissedProceedLocked() {
	path := dismissedProceedFilePath
	if path == "" {
		path = getDismissedProceedFilePath()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		slog.Warn("[Proxy] Failed to create data dir for dismissed_proceed.json", "err", err)
		return
	}
	store := dismissedProceedStore{
		Version: 1,
		Steps:   dismissedProceedSteps,
	}
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		slog.Warn("[Proxy] Failed to write dismissed_proceed.json temp file", "err", err)
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		slog.Warn("[Proxy] Failed to atomically replace dismissed_proceed.json", "err", err)
	}
}

// MarkProceedDismissed records that the user explicitly dismissed the plan-proceed reminder
// for cascadeID at stepCount. It remains dismissed until the conversation advances past stepCount
// or ClearProceedDismissed is called (e.g. on new user message).
func MarkProceedDismissed(cascadeID string, stepCount int) {
	cascadeID = strings.TrimSpace(cascadeID)
	if cascadeID == "" {
		return
	}
	if stepCount < 0 {
		stepCount = 0
	}
	dismissedProceedMu.Lock()
	defer dismissedProceedMu.Unlock()
	ensureDismissedProceedLoadedLocked()
	if cur, ok := dismissedProceedSteps[cascadeID]; ok && cur == stepCount {
		return
	}
	dismissedProceedSteps[cascadeID] = stepCount
	saveDismissedProceedLocked()
}

// ClearProceedDismissed removes any persisted proceed dismissal for cascadeID.
func ClearProceedDismissed(cascadeID string) {
	cascadeID = strings.TrimSpace(cascadeID)
	if cascadeID == "" {
		return
	}
	dismissedProceedMu.Lock()
	defer dismissedProceedMu.Unlock()
	ensureDismissedProceedLoadedLocked()
	if _, ok := dismissedProceedSteps[cascadeID]; !ok {
		return
	}
	delete(dismissedProceedSteps, cascadeID)
	saveDismissedProceedLocked()
}

// IsProceedDismissed reports whether the plan-proceed reminder at currentTotalSteps
// was previously dismissed by the user.
func IsProceedDismissed(cascadeID string, currentTotalSteps int) bool {
	cascadeID = strings.TrimSpace(cascadeID)
	if cascadeID == "" {
		return false
	}
	dismissedProceedMu.RLock()
	path := getDismissedProceedFilePath()
	if dismissedProceedLoaded && dismissedProceedFilePath == path {
		dismissedAt, ok := dismissedProceedSteps[cascadeID]
		dismissedProceedMu.RUnlock()
		if !ok {
			return false
		}
		if dismissedAt <= 0 {
			return true
		}
		return currentTotalSteps <= dismissedAt
	}
	dismissedProceedMu.RUnlock()

	dismissedProceedMu.Lock()
	ensureDismissedProceedLoadedLocked()
	dismissedAt, ok := dismissedProceedSteps[cascadeID]
	dismissedProceedMu.Unlock()
	if !ok {
		return false
	}
	if dismissedAt <= 0 {
		return true
	}
	return currentTotalSteps <= dismissedAt
}

// isSummaryArchived reports whether a trajectory summary map is archived either via
// upstream annotations or local disk annotation backfill.
func isSummaryArchived(s map[string]interface{}) bool {
	if s == nil {
		return false
	}
	if ann, ok := s["annotations"].(map[string]interface{}); ok && ann != nil {
		if archived, ok := ann["archived"].(bool); ok && archived {
			return true
		}
	}
	return false
}

// IsCascadeArchived reports whether cascadeID is marked archived in summary annotations
// or in the local annotation .pbtxt file on disk.
func IsCascadeArchived(cascadeID string, summary map[string]interface{}) bool {
	if isSummaryArchived(summary) {
		return true
	}
	if cascadeID != "" {
		if diskArchived, found := readAnnotationArchived(cascadeID); found && diskArchived {
			return true
		}
	}
	return false
}

func (p *Proxy) notifyCascadeAlertCleared(cascadeID, dismissKey, finalStatus string) {
	if p == nil || cascadeID == "" {
		return
	}
	p.mu.RLock()
	sink := p.notifier
	p.mu.RUnlock()
	if clearer, ok := sink.(CascadeAlertClearSink); ok && clearer != nil {
		go clearer.OnCascadeAlertCleared(cascadeID, dismissKey, finalStatus)
	}
}

// HandleDismissProceed serves POST /gateway/cascade/proceed/dismiss.
// It suppresses recurring Lock Screen / Dynamic Island / list-badge "Action Needed" reminders
// for an implementation plan awaiting Proceed without destroying the in-chat Proceed button.
func (p *Proxy) HandleDismissProceed(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		CascadeID  string `json:"cascadeId"`
		TotalSteps int    `json:"totalSteps"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 64*1024)).Decode(&req); err != nil {
		writeJSONError(w, "invalid request: "+err.Error(), http.StatusBadRequest)
		return
	}
	cascadeID := strings.TrimSpace(req.CascadeID)
	if cascadeID == "" || !cascadeIDRe.MatchString(cascadeID) {
		writeJSONError(w, "invalid cascadeId", http.StatusBadRequest)
		return
	}

	stepCount := req.TotalSteps
	if stepCount <= 0 {
		if details, err := p.FetchTrajectoryDetails(cascadeID, 500*time.Millisecond); err == nil && details != nil && details.TotalSteps > 0 {
			stepCount = details.TotalSteps
		}
	}

	MarkProceedDismissed(cascadeID, stepCount)
	ClearTrajectoryCache(cascadeID)
	p.invalidateAllTrajectories()
	p.notifyStreamTouch(cascadeID)
	p.notifyCascadeAlertCleared(cascadeID, fmt.Sprintf("proceed:%d", stepCount), "COMPLETED")

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":              true,
		"cascadeId":       cascadeID,
		"dismissedAtStep": stepCount,
	})
}
