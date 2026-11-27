package proxy

import (
	"bytes"
	"compress/gzip"
	"context"
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
)

// stitchPostStartDelay adds a brief, human-paced delay between StartCascade and the subsequent
// SendUserCascadeMessage during account-switch rollover so the two RPCs are not fired 1ms apart.
// Overridable in tests.
var stitchPostStartDelay = 300 * time.Millisecond

const (
	stitchPromptHeaderMarker       = "<!-- CONTEXT_HANDOVER_SUMMARY -->"
	stitchPromptEndMarker          = "<!-- /CONTEXT_HANDOVER_SUMMARY -->"
	legacyStitchPromptHeaderMarker = "<!-- MGY_COMPACT_HANDOVER_V1 -->"
	legacyStitchPromptEndMarker    = "<!-- /MGY_COMPACT_HANDOVER_V1 -->"
)

// CascadeStitchSegment represents one stitched underlying cascade segment in a virtual conversation chain.
type CascadeStitchSegment struct {
	CascadeID string `json:"cascadeId"`
	AccountID string `json:"accountId,omitempty"`
	CreatedAt string `json:"createdAt,omitempty"`
	Epoch     uint64 `json:"epoch,omitempty"`
}

// CascadeStitchChain tracks the transparent mapping from a visible root cascade ID (A)
// to one or more underlying continuation cascade IDs ([A, B, C...]) created across account switches.
type CascadeStitchChain struct {
	RootCascadeID string                 `json:"rootCascadeId"`
	Segments      []CascadeStitchSegment `json:"segments"`
	UpdatedAt     string                 `json:"updatedAt"`
}

type cascadeStitchStore struct {
	mu           sync.RWMutex
	loaded       bool
	switchEpoch  uint64
	lastAccount  string
	chainsByRoot map[string]*CascadeStitchChain
	rootByChild  map[string]string
	cascadeEpoch map[string]uint64
	creatingMu   sync.Mutex
}

var defaultStitchStore = &cascadeStitchStore{
	chainsByRoot: make(map[string]*CascadeStitchChain),
	rootByChild:  make(map[string]string),
	cascadeEpoch: make(map[string]uint64),
}

// ResetCascadeStitchStoreForTest resets the in-memory stitch store for isolated unit tests.
func ResetCascadeStitchStoreForTest() {
	defaultStitchStore.mu.Lock()
	defaultStitchStore.loaded = true
	defaultStitchStore.switchEpoch = 0
	defaultStitchStore.lastAccount = ""
	defaultStitchStore.chainsByRoot = make(map[string]*CascadeStitchChain)
	defaultStitchStore.rootByChild = make(map[string]string)
	defaultStitchStore.cascadeEpoch = make(map[string]uint64)
	defaultStitchStore.mu.Unlock()
}

// IsSwitchCompactHandoverEnabled reports whether account-switch compact handover stitching is enabled.
// Enabled by default; set MGY_SWITCH_COMPACT_HANDOVER=off/false/0 to disable.
func IsSwitchCompactHandoverEnabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("MGY_SWITCH_COMPACT_HANDOVER")))
	switch v {
	case "0", "false", "off", "no", "disabled":
		return false
	default:
		return true
	}
}

func stitchStateFilePath() string {
	if custom := strings.TrimSpace(os.Getenv("MGY_STITCH_STATE_PATH")); custom != "" {
		return filepath.Clean(custom)
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".multigravity", "cascade_stitches.json")
}

func stitchSegmentCacheDir() string {
	statePath := stitchStateFilePath()
	if statePath == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(statePath), "stitch_cache")
}

// IsSealedHistoricalStitchSegment reports whether cascadeID is a completed non-leaf historical
// segment in a stitched chain (i.e. an older account's segment that must not be eagerly probed
// via LoadTrajectory on subsequent accounts).
func IsSealedHistoricalStitchSegment(cascadeID string) bool {
	cascadeID = strings.TrimSpace(cascadeID)
	if cascadeID == "" {
		return false
	}
	segs := GetStitchChainSegments(cascadeID)
	if len(segs) <= 1 {
		return false
	}
	activeLeaf := segs[len(segs)-1].CascadeID
	return cascadeID != activeLeaf
}

// loadSealedSegmentFromDisk reads a previously persisted immutable historical segment snapshot
// from ~/.multigravity/stitch_cache/<cascadeID>.json.gz so gateway restarts do not need to call
// LoadTrajectory / GetCascadeTrajectory on a new account's language_server for old-account segments.
func loadSealedSegmentFromDisk(cascadeID string) *upstreamTrajectoryResp {
	if cascadeID == "" || !cascadeIDRe.MatchString(cascadeID) {
		return nil
	}
	dir := stitchSegmentCacheDir()
	if dir == "" {
		return nil
	}
	p := filepath.Join(dir, cascadeID+".json.gz")
	f, err := os.Open(p)
	if err != nil {
		return nil
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil
	}
	defer gz.Close()
	const maxSealedBytes = 64 * 1024 * 1024
	raw, err := io.ReadAll(io.LimitReader(gz, maxSealedBytes))
	if err != nil || len(raw) == 0 {
		return nil
	}
	var resp upstreamTrajectoryResp
	if json.Unmarshal(raw, &resp) != nil {
		return nil
	}
	if len(resp.Trajectory.Steps) == 0 || resp.Status == "CASCADE_RUN_STATUS_RUNNING" {
		return nil
	}
	return &resp
}

// saveSealedSegmentToDisk persists an immutable non-empty historical segment snapshot locally.
// No-op if the snapshot already exists on disk.
func saveSealedSegmentToDisk(cascadeID string, data *upstreamTrajectoryResp) {
	if cascadeID == "" || !cascadeIDRe.MatchString(cascadeID) || data == nil {
		return
	}
	if len(data.Trajectory.Steps) == 0 || data.Status == "CASCADE_RUN_STATUS_RUNNING" {
		return
	}
	dir := stitchSegmentCacheDir()
	if dir == "" {
		return
	}
	target := filepath.Join(dir, cascadeID+".json.gz")
	if info, err := os.Stat(target); err == nil && info.Size() > 0 {
		return
	}
	raw, err := json.Marshal(data)
	if err != nil || len(raw) == 0 {
		return
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	var buf bytes.Buffer
	gz, err := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
	if err != nil {
		return
	}
	if _, err := gz.Write(raw); err != nil {
		_ = gz.Close()
		return
	}
	if err := gz.Close(); err != nil {
		return
	}
	tmp := fmt.Sprintf("%s.%d.tmp", target, time.Now().UnixNano())
	if err := os.WriteFile(tmp, buf.Bytes(), 0o600); err == nil {
		if renameErr := os.Rename(tmp, target); renameErr != nil {
			_ = os.Remove(tmp)
		}
	}
}

// removeSealedSegmentDiskCaches deletes any persisted local sealed-segment snapshots for ids.
func removeSealedSegmentDiskCaches(ids []string) {
	dir := stitchSegmentCacheDir()
	if dir == "" {
		return
	}
	for _, cid := range ids {
		if cid != "" && cascadeIDRe.MatchString(cid) {
			_ = os.Remove(filepath.Join(dir, cid+".json.gz"))
		}
	}
}

func (s *cascadeStitchStore) ensureLoadedLocked() {
	if s.loaded {
		return
	}
	s.loaded = true
	p := stitchStateFilePath()
	if p == "" {
		return
	}
	raw, err := os.ReadFile(p)
	if err != nil || len(raw) == 0 {
		return
	}
	var doc struct {
		SwitchEpoch  uint64                         `json:"switchEpoch"`
		LastAccount  string                         `json:"lastAccount,omitempty"`
		ChainsByRoot map[string]*CascadeStitchChain `json:"chains"`
	}
	if json.Unmarshal(raw, &doc) != nil {
		return
	}
	if doc.SwitchEpoch > s.switchEpoch {
		s.switchEpoch = doc.SwitchEpoch
	}
	if doc.LastAccount != "" {
		s.lastAccount = doc.LastAccount
	}
	for rootID, ch := range doc.ChainsByRoot {
		if ch == nil || len(ch.Segments) == 0 || !cascadeIDRe.MatchString(rootID) {
			continue
		}
		ch.RootCascadeID = rootID
		s.chainsByRoot[rootID] = ch
		for _, seg := range ch.Segments {
			if seg.CascadeID != "" && seg.CascadeID != rootID {
				s.rootByChild[seg.CascadeID] = rootID
			}
		}
		// Restore active leaf's epoch: use persisted segment epoch when present so a gateway restart
		// between NotifyAccountSwitched and the next send still triggers handover rollover;
		// fall back to s.switchEpoch for legacy chains saved before Epoch was persisted.
		leafSeg := ch.Segments[len(ch.Segments)-1]
		leaf := leafSeg.CascadeID
		leafEpoch := leafSeg.Epoch
		if leafEpoch == 0 {
			leafEpoch = s.switchEpoch
		}
		s.cascadeEpoch[rootID] = leafEpoch
		s.cascadeEpoch[leaf] = leafEpoch
	}
}

func (s *cascadeStitchStore) saveLocked() {
	p := stitchStateFilePath()
	if p == "" {
		return
	}
	_ = os.MkdirAll(filepath.Dir(p), 0o700)
	doc := struct {
		SwitchEpoch  uint64                         `json:"switchEpoch"`
		LastAccount  string                         `json:"lastAccount,omitempty"`
		ChainsByRoot map[string]*CascadeStitchChain `json:"chains"`
	}{
		SwitchEpoch:  s.switchEpoch,
		LastAccount:  s.lastAccount,
		ChainsByRoot: s.chainsByRoot,
	}
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err == nil {
		_ = os.Rename(tmp, p)
	}
}

// NotifyAccountSwitched increments the account-switch generation epoch so any subsequent message
// sent to a pre-existing conversation automatically compacts its prior context and rolls over
// to a fresh underlying cascadeId under the new account while keeping the client view unified.
func (p *Proxy) NotifyAccountSwitched(accountID string) {
	defaultStitchStore.mu.Lock()
	defaultStitchStore.ensureLoadedLocked()
	defaultStitchStore.switchEpoch++
	defaultStitchStore.lastAccount = strings.TrimSpace(accountID)
	epoch := defaultStitchStore.switchEpoch
	defaultStitchStore.saveLocked()
	defaultStitchStore.mu.Unlock()
	slog.Info("[Stitch] Recorded account switch for compact handover", "account", accountID, "epoch", epoch)
}

// ResolveVisibleRootCascadeID maps any underlying child continuation cascade ID back to its
// client-facing root cascade ID. If cascadeID is not a stitched child, it returns cascadeID unchanged.
func ResolveVisibleRootCascadeID(cascadeID string) string {
	cascadeID = strings.TrimSpace(cascadeID)
	if cascadeID == "" {
		return ""
	}
	defaultStitchStore.mu.Lock()
	defaultStitchStore.ensureLoadedLocked()
	root, ok := defaultStitchStore.rootByChild[cascadeID]
	defaultStitchStore.mu.Unlock()
	if ok && root != "" {
		return root
	}
	return cascadeID
}

// IsStitchedChildCascade reports whether cascadeID is an internal continuation child cascade
// that should be hidden as a standalone row in GetAllCascadeTrajectories.
func IsStitchedChildCascade(cascadeID string) bool {
	cascadeID = strings.TrimSpace(cascadeID)
	if cascadeID == "" {
		return false
	}
	defaultStitchStore.mu.Lock()
	defaultStitchStore.ensureLoadedLocked()
	_, isChild := defaultStitchStore.rootByChild[cascadeID]
	defaultStitchStore.mu.Unlock()
	return isChild
}

// ResolveActiveCascadeID returns the currently active underlying leaf cascade ID for a visible root cascade ID.
func ResolveActiveCascadeID(cascadeID string) string {
	cascadeID = strings.TrimSpace(cascadeID)
	if cascadeID == "" {
		return ""
	}
	defaultStitchStore.mu.Lock()
	defaultStitchStore.ensureLoadedLocked()
	rootID := cascadeID
	if r, ok := defaultStitchStore.rootByChild[cascadeID]; ok && r != "" {
		rootID = r
	}
	if ch, ok := defaultStitchStore.chainsByRoot[rootID]; ok && ch != nil && len(ch.Segments) > 0 {
		leaf := ch.Segments[len(ch.Segments)-1].CascadeID
		defaultStitchStore.mu.Unlock()
		if leaf != "" {
			return leaf
		}
	}
	defaultStitchStore.mu.Unlock()
	return cascadeID
}

// GetStitchChainSegments returns all segment cascade IDs [root, child1, child2...] for rootCascadeID.
// Returns nil if rootCascadeID has no stitched continuations.
func GetStitchChainSegments(cascadeID string) []CascadeStitchSegment {
	cascadeID = strings.TrimSpace(cascadeID)
	if cascadeID == "" {
		return nil
	}
	defaultStitchStore.mu.Lock()
	defaultStitchStore.ensureLoadedLocked()
	rootID := cascadeID
	if r, ok := defaultStitchStore.rootByChild[cascadeID]; ok && r != "" {
		rootID = r
	}
	ch := defaultStitchStore.chainsByRoot[rootID]
	if ch == nil || len(ch.Segments) <= 1 {
		defaultStitchStore.mu.Unlock()
		return nil
	}
	out := make([]CascadeStitchSegment, len(ch.Segments))
	copy(out, ch.Segments)
	defaultStitchStore.mu.Unlock()
	return out
}

// GetStitchChainSegmentIDs returns all segment cascade IDs in the stitch chain containing cascadeID
// without modifying the store. Returns nil if cascadeID is not part of a stitched chain.
func GetStitchChainSegmentIDs(cascadeID string) []string {
	segs := GetStitchChainSegments(cascadeID)
	if len(segs) == 0 {
		return nil
	}
	ids := make([]string, 0, len(segs))
	for _, seg := range segs {
		if seg.CascadeID != "" {
			ids = append(ids, seg.CascadeID)
		}
	}
	return ids
}

// RecordCascadeCreatedAtCurrentEpoch marks a newly started cascade as already belonging to the current account epoch.
func RecordCascadeCreatedAtCurrentEpoch(cascadeID string) {
	cascadeID = strings.TrimSpace(cascadeID)
	if cascadeID == "" {
		return
	}
	defaultStitchStore.mu.Lock()
	defaultStitchStore.ensureLoadedLocked()
	defaultStitchStore.cascadeEpoch[cascadeID] = defaultStitchStore.switchEpoch
	defaultStitchStore.mu.Unlock()
}

// RemoveStitchChain removes any stitch mappings associated with rootOrChildID (e.g. on conversation delete).
func RemoveStitchChain(cascadeID string) []string {
	cascadeID = strings.TrimSpace(cascadeID)
	if cascadeID == "" {
		return nil
	}
	defaultStitchStore.mu.Lock()
	defaultStitchStore.ensureLoadedLocked()
	rootID := cascadeID
	if r, ok := defaultStitchStore.rootByChild[cascadeID]; ok && r != "" {
		rootID = r
	}
	ch := defaultStitchStore.chainsByRoot[rootID]
	if ch == nil {
		delete(defaultStitchStore.cascadeEpoch, cascadeID)
		defaultStitchStore.mu.Unlock()
		return nil
	}
	var allIDs []string
	for _, seg := range ch.Segments {
		if seg.CascadeID != "" {
			allIDs = append(allIDs, seg.CascadeID)
			delete(defaultStitchStore.rootByChild, seg.CascadeID)
			delete(defaultStitchStore.cascadeEpoch, seg.CascadeID)
		}
	}
	delete(defaultStitchStore.chainsByRoot, rootID)
	delete(defaultStitchStore.cascadeEpoch, rootID)
	defaultStitchStore.saveLocked()
	defaultStitchStore.mu.Unlock()
	return allIDs
}

func (s *cascadeStitchStore) needsHandoverRollover(rootID string) (bool, string, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()
	if s.switchEpoch == 0 {
		return false, rootID, ""
	}
	if r, ok := s.rootByChild[rootID]; ok && r != "" {
		rootID = r
	}
	activeID := rootID
	if ch, ok := s.chainsByRoot[rootID]; ok && ch != nil && len(ch.Segments) > 0 {
		activeID = ch.Segments[len(ch.Segments)-1].CascadeID
	}
	if ep, ok := s.cascadeEpoch[activeID]; ok && ep >= s.switchEpoch {
		return false, activeID, s.lastAccount
	}
	if ep, ok := s.cascadeEpoch[rootID]; ok && ep >= s.switchEpoch && activeID == rootID {
		return false, activeID, s.lastAccount
	}
	return true, activeID, s.lastAccount
}

func (s *cascadeStitchStore) appendSegment(rootID, prevActiveID, newChildID, accountID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()
	nowStr := time.Now().UTC().Format(time.RFC3339)
	ch, ok := s.chainsByRoot[rootID]
	if !ok || ch == nil || len(ch.Segments) == 0 {
		ch = &CascadeStitchChain{
			RootCascadeID: rootID,
			Segments: []CascadeStitchSegment{
				{CascadeID: rootID},
			},
		}
		s.chainsByRoot[rootID] = ch
	}
	ch.Segments = append(ch.Segments, CascadeStitchSegment{
		CascadeID: newChildID,
		AccountID: accountID,
		CreatedAt: nowStr,
		Epoch:     s.switchEpoch,
	})
	ch.UpdatedAt = nowStr
	s.rootByChild[newChildID] = rootID
	s.cascadeEpoch[rootID] = s.switchEpoch
	s.cascadeEpoch[prevActiveID] = s.switchEpoch
	s.cascadeEpoch[newChildID] = s.switchEpoch
	s.saveLocked()
}

// BuildOfflineCompactSummary constructs a concise, zero-LLM-token Markdown handover summary
// from the existing steps of a conversation (across all prior segments or on-disk transcript.jsonl).
func (p *Proxy) BuildOfflineCompactSummary(ctx context.Context, rootCascadeID string, port int, token string) (string, []string) {
	summary, wsURIs, _ := p.buildOfflineCompactSummaryDetailed(ctx, rootCascadeID, port, token)
	return summary, wsURIs
}

func (p *Proxy) buildOfflineCompactSummaryDetailed(ctx context.Context, rootCascadeID string, port int, token string) (string, []string, bool) {
	var allSteps []TrajectoryStep
	var workspaceURIs []string
	hadFetchErr := false

	// 1. Gather steps from existing trajectory segments (cross-port cache first, then upstream/LoadTrajectory)
	segments := GetStitchChainSegments(rootCascadeID)
	if len(segments) == 0 {
		segments = []CascadeStitchSegment{{CascadeID: rootCascadeID}}
	}
	for _, seg := range segments {
		if port > 0 {
			raw, err := p.fetchHistoricalSegmentTrajectory(ctx, seg.CascadeID, port, token)
			if err == nil && raw != nil {
				if len(workspaceURIs) == 0 && len(raw.Trajectory.WorkspaceUris) > 0 {
					workspaceURIs = append([]string(nil), raw.Trajectory.WorkspaceUris...)
				}
				allSteps = append(allSteps, raw.Trajectory.Steps...)
			} else {
				hadFetchErr = true
			}
		}
	}

	// 2. If upstream steps were unavailable, fall back to on-disk brain/<id>/.system_generated/logs/transcript.jsonl
	if len(allSteps) == 0 {
		if diskSummary := buildCompactSummaryFromTranscriptJSONLs(segments); diskSummary != "" {
			return diskSummary, workspaceURIs, false
		}
		return "", workspaceURIs, hadFetchErr
	}

	var userPrompts []string
	var agentReplies []string
	var touchedFiles []string
	seenFiles := make(map[string]bool)
	var latestPlan *StepPlanItem

	for idx, st := range allSteps {
		switch st.Type {
		case "CORTEX_STEP_TYPE_USER_INPUT":
			txt, _ := st.UserInput.ExtractTextAndSlashCommands()
			txt = StripCompactHandoverPrefix(strings.TrimSpace(txt))
			if txt != "" {
				userPrompts = append(userPrompts, truncateRunes(txt, 360))
			}
		case "CORTEX_STEP_TYPE_PLANNER_RESPONSE":
			if st.PlannerResponse != nil {
				resp := strings.TrimSpace(CleanInternalSystemMessage(st.PlannerResponse.Response))
				if resp != "" {
					agentReplies = append(agentReplies, truncateRunes(resp, 500))
				}
			}
		}
		if isUpdateStepsStep(st) {
			if plan := extractStepPlanFromStep(st, idx); plan != nil && len(plan.Items) > 0 {
				latestPlan = plan
			}
		}
		if st.CodeAction != nil {
			for _, uri := range extractCodeActionURIs(st) {
				clean := strings.TrimPrefix(strings.TrimSpace(uri), "file://")
				if clean != "" && !seenFiles[clean] {
					seenFiles[clean] = true
					touchedFiles = append(touchedFiles, clean)
				}
			}
		}
	}

	if len(userPrompts) == 0 && len(agentReplies) == 0 {
		if diskSummary := buildCompactSummaryFromTranscriptJSONLs(segments); diskSummary != "" {
			return diskSummary, workspaceURIs, false
		}
		return "", workspaceURIs, hadFetchErr
	}

	var sb strings.Builder
	sb.WriteString("[Context Handover Summary (Compacted from prior turns)]\n")
	if len(userPrompts) > 0 {
		sb.WriteString("\n### User Goals & Recent Requests\n")
		sb.WriteString("- Initial Goal: " + strings.ReplaceAll(userPrompts[0], "\n", " ") + "\n")
		start := len(userPrompts) - 3
		if start < 1 {
			start = 1
		}
		for i := start; i < len(userPrompts); i++ {
			sb.WriteString(fmt.Sprintf("- Follow-up #%d: %s\n", i, strings.ReplaceAll(userPrompts[i], "\n", " ")))
		}
	}
	if latestPlan != nil && len(latestPlan.Items) > 0 {
		sb.WriteString("\n### Current Step Checklist\n")
		for _, it := range latestPlan.Items {
			sb.WriteString(fmt.Sprintf("- [%s] %s\n", it.Status, it.Title))
		}
	}
	if len(touchedFiles) > 0 {
		sb.WriteString("\n### Modified / Referenced Files\n")
		start := 0
		if len(touchedFiles) > 12 {
			start = len(touchedFiles) - 12
		}
		for _, f := range touchedFiles[start:] {
			sb.WriteString("- " + f + "\n")
		}
	}
	if len(agentReplies) > 0 {
		sb.WriteString("\n### Latest Progress & State\n")
		start := len(agentReplies) - 2
		if start < 0 {
			start = 0
		}
		for _, r := range agentReplies[start:] {
			sb.WriteString("- " + strings.ReplaceAll(r, "\n", " ") + "\n")
		}
	}
	sb.WriteString("\nPlease continue seamlessly from the state above and respond to the user's new message below.")
	return sb.String(), workspaceURIs, false
}

func extractCodeActionURIs(s TrajectoryStep) []string {
	if s.CodeAction == nil {
		return nil
	}
	var out []string
	if ar := s.CodeAction.ActionResult; ar != nil {
		if ar.AbsoluteURI != "" {
			out = append(out, ar.AbsoluteURI)
		}
		if ar.Edit != nil && ar.Edit.AbsoluteURI != "" {
			out = append(out, ar.Edit.AbsoluteURI)
		}
	}
	if as := s.CodeAction.ActionSpec; as != nil {
		if as.CreateFile != nil && as.CreateFile.Path != nil && as.CreateFile.Path.AbsoluteURI != "" {
			out = append(out, as.CreateFile.Path.AbsoluteURI)
		}
		if as.EditFile != nil && as.EditFile.Path != nil && as.EditFile.Path.AbsoluteURI != "" {
			out = append(out, as.EditFile.Path.AbsoluteURI)
		}
	}
	return out
}

func buildCompactSummaryFromTranscriptJSONLs(segments []CascadeStitchSegment) string {
	var userInputs []string
	var agentReplies []string
	for _, seg := range segments {
		cascadeID := seg.CascadeID
		if cascadeID == "" || !cascadeIDRe.MatchString(cascadeID) {
			continue
		}
		p := resolveAntigravitySubPath("brain", cascadeID, ".system_generated", "logs", "transcript.jsonl")
		if p == "" {
			continue
		}
		raw, err := os.ReadFile(p)
		if err != nil || len(raw) == 0 {
			continue
		}
		for _, line := range bytes.Split(raw, []byte("\n")) {
			line = bytes.TrimSpace(line)
			if len(line) == 0 {
				continue
			}
			var entry struct {
				Type    string `json:"type"`
				Content string `json:"content"`
			}
			if json.Unmarshal(line, &entry) != nil {
				continue
			}
			c := strings.TrimSpace(CleanInternalSystemMessage(entry.Content))
			c = StripCompactHandoverPrefix(c)
			if c == "" {
				continue
			}
			switch entry.Type {
			case "USER_INPUT":
				userInputs = append(userInputs, truncateRunes(c, 360))
			case "PLANNER_RESPONSE":
				agentReplies = append(agentReplies, truncateRunes(c, 500))
			}
		}
	}
	if len(userInputs) == 0 && len(agentReplies) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("[Context Handover Summary (Compacted from prior turns)]\n")
	if len(userInputs) > 0 {
		sb.WriteString("\n### User Goals & Recent Requests\n")
		sb.WriteString("- Initial Goal: " + strings.ReplaceAll(userInputs[0], "\n", " ") + "\n")
		start := len(userInputs) - 3
		if start < 1 {
			start = 1
		}
		for i := start; i < len(userInputs); i++ {
			sb.WriteString(fmt.Sprintf("- Follow-up #%d: %s\n", i, strings.ReplaceAll(userInputs[i], "\n", " ")))
		}
	}
	if len(agentReplies) > 0 {
		sb.WriteString("\n### Latest Progress & State\n")
		start := len(agentReplies) - 2
		if start < 0 {
			start = 0
		}
		for _, r := range agentReplies[start:] {
			sb.WriteString("- " + strings.ReplaceAll(r, "\n", " ") + "\n")
		}
	}
	sb.WriteString("\nPlease continue seamlessly from the state above and respond to the user's new message below.")
	return sb.String()
}

func buildCompactSummaryFromTranscriptJSONL(cascadeID string) string {
	return buildCompactSummaryFromTranscriptJSONLs([]CascadeStitchSegment{{CascadeID: cascadeID}})
}

// WrapUserPromptWithCompactHandover embeds the offline compact handover summary inside markers
// so the model receives the full context while the gateway parser can strip the prefix and display
// only the user's actual message in the unified conversation UI.
func WrapUserPromptWithCompactHandover(summary, userText string) string {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return userText
	}
	return fmt.Sprintf("%s\n%s\n%s\n\n%s", stitchPromptHeaderMarker, summary, stitchPromptEndMarker, userText)
}

// StripCompactHandoverPrefix removes the internal compact handover block from a step's user input text
// so mobile and web clients only render the user's real prompt text.
func StripCompactHandoverPrefix(text string) string {
	for _, pair := range [][2]string{
		{stitchPromptHeaderMarker, stitchPromptEndMarker},
		{legacyStitchPromptHeaderMarker, legacyStitchPromptEndMarker},
	} {
		if !strings.Contains(text, pair[0]) {
			continue
		}
		startIdx := strings.Index(text, pair[0])
		endIdx := strings.Index(text, pair[1])
		if startIdx >= 0 && endIdx > startIdx {
			after := text[endIdx+len(pair[1]):]
			text = strings.TrimLeft(after, "\r\n ")
		}
	}
	return text
}

// EnsureCascadeTargetForSend resolves the active underlying cascade ID for a user send request,
// transparently rolling over to a newly started continuation cascade with an offline compact summary
// if an account switch occurred since the conversation last ran.
// If handover rollover is required but StartCascade fails after retries, an error is returned
// so the caller aborts instead of falling back to the prior account's cascade ID.
func (p *Proxy) EnsureCascadeTargetForSend(ctx context.Context, clientCascadeID string, rawMap map[string]interface{}, port int, token string) (targetCascadeID string, rolledOver bool, err error) {
	rootID := ResolveVisibleRootCascadeID(clientCascadeID)
	if !IsSwitchCompactHandoverEnabled() || port <= 0 || rootID == "" {
		return ResolveActiveCascadeID(rootID), false, nil
	}

	needsRollover, prevActiveID, accountID := defaultStitchStore.needsHandoverRollover(rootID)
	if !needsRollover {
		return prevActiveID, false, nil
	}

	defaultStitchStore.creatingMu.Lock()
	defer defaultStitchStore.creatingMu.Unlock()

	// Double-check after acquiring creation lock
	needsRollover, prevActiveID, accountID = defaultStitchStore.needsHandoverRollover(rootID)
	if !needsRollover {
		return prevActiveID, false, nil
	}

	summary, wsURIs, hadFetchErr := p.buildOfflineCompactSummaryDetailed(ctx, rootID, port, token)
	if summary == "" {
		if !hadFetchErr && len(GetStitchChainSegments(rootID)) <= 1 {
			// Conversation genuinely has no prior history at all (e.g. brand new empty draft).
			RecordCascadeCreatedAtCurrentEpoch(prevActiveID)
			return prevActiveID, false, nil
		}
		// Prior segments exist or could not be read transiently; still roll over onto a fresh
		// cascade under the current account rather than sending cross-account on prevActiveID.
		summary = "[Context Handover Summary]\nContinuing the previous conversation after an account switch. Please respond to the user's request below."
	}

	startPayload := map[string]interface{}{
		"source": "CORTEX_TRAJECTORY_SOURCE_INTERACTIVE_CASCADE",
	}
	if len(wsURIs) > 0 {
		startPayload["workspaceUris"] = wsURIs
	}

	retryDelays := []time.Duration{300 * time.Millisecond, 600 * time.Millisecond}
	var (
		cBody   []byte
		cStatus int
		cErr    error
	)
	for attempt := 0; attempt <= len(retryDelays); attempt++ {
		if attempt > 0 {
			if ctx != nil {
				timer := time.NewTimer(retryDelays[attempt-1])
				select {
				case <-timer.C:
				case <-ctx.Done():
					timer.Stop()
					return "", false, ctx.Err()
				}
			} else {
				time.Sleep(retryDelays[attempt-1])
			}
		}
		cBody, cStatus, cErr = p.callUpstreamRPC(ctx, "StartCascade", startPayload)
		if cErr == nil && cStatus == http.StatusOK {
			break
		}
		if ctx != nil && ctx.Err() != nil {
			break
		}
	}
	if cErr != nil || cStatus != http.StatusOK {
		slog.Warn("[Stitch] StartCascade for compact handover failed after retries; refusing cross-account fallback",
			"root", shortCascadeID(rootID), "err", cErr, "status", cStatus)
		if cErr != nil {
			return "", false, fmt.Errorf("StartCascade handover failed: %w", cErr)
		}
		return "", false, fmt.Errorf("StartCascade handover returned HTTP %d", cStatus)
	}
	var started struct {
		CascadeID string `json:"cascadeId"`
	}
	if json.Unmarshal(cBody, &started) != nil || strings.TrimSpace(started.CascadeID) == "" {
		return "", false, fmt.Errorf("StartCascade handover returned empty cascadeId")
	}
	newChildID := strings.TrimSpace(started.CascadeID)

	// Carry over model and planExecute state from the previous active cascade
	if model, cfg := GetCascadeModel(prevActiveID); model != "" || len(cfg) > 0 {
		SetCascadeModel(newChildID, model, cfg)
	} else if model, cfg := GetCascadeModel(rootID); model != "" || len(cfg) > 0 {
		SetCascadeModel(newChildID, model, cfg)
	}
	if IsCascadePlanExecute(prevActiveID) || IsCascadePlanExecute(rootID) {
		SetCascadePlanExecute(newChildID, true)
	}

	// Prepend the offline compact summary to the first text item in rawMap
	injectCompactSummaryIntoRawSendMap(rawMap, summary)
	rawMap["cascadeId"] = newChildID

	defaultStitchStore.appendSegment(rootID, prevActiveID, newChildID, accountID)
	p.agentStateHub.OnCascadeTargetRolledOver(p, rootID)
	if stitchPostStartDelay > 0 {
		if ctx != nil {
			timer := time.NewTimer(stitchPostStartDelay)
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
			}
		} else {
			time.Sleep(stitchPostStartDelay)
		}
	}
	slog.Info("[Stitch] 🔄 Compact handover created continuation cascade",
		"root", shortCascadeID(rootID), "prev", shortCascadeID(prevActiveID), "new", shortCascadeID(newChildID), "account", accountID)
	return newChildID, true, nil
}

func injectCompactSummaryIntoRawSendMap(rawMap map[string]interface{}, summary string) {
	if rawMap == nil || strings.TrimSpace(summary) == "" {
		return
	}
	if items, ok := rawMap["items"].([]interface{}); ok && len(items) > 0 {
		for _, it := range items {
			if itemMap, ok := it.(map[string]interface{}); ok {
				if t, ok := itemMap["text"].(string); ok {
					itemMap["text"] = WrapUserPromptWithCompactHandover(summary, t)
					return
				}
			}
		}
		firstItem := map[string]interface{}{"text": WrapUserPromptWithCompactHandover(summary, "")}
		rawMap["items"] = append([]interface{}{firstItem}, items...)
		return
	}
	if txt, ok := rawMap["text"].(string); ok {
		wrapped := WrapUserPromptWithCompactHandover(summary, txt)
		rawMap["text"] = wrapped
		rawMap["items"] = []interface{}{map[string]interface{}{"text": wrapped}}
		return
	}
	rawMap["items"] = []interface{}{map[string]interface{}{"text": WrapUserPromptWithCompactHandover(summary, "Continue")}}
}

func makeAccountSwitchMarkerStep(accountID, createdAt string) TrajectoryStep {
	label := "🔄 已切换账号并自动压缩历史上下文 (Compact Handover)"
	if acc := strings.TrimSpace(accountID); acc != "" {
		label = fmt.Sprintf("🔄 已切换至账号 %s 并自动压缩历史上下文 (Compact Handover)", acc)
	}
	if createdAt == "" {
		createdAt = time.Now().UTC().Format(time.RFC3339)
	}
	var s TrajectoryStep
	s.Type = "CORTEX_STEP_TYPE_PLANNER_RESPONSE"
	s.Status = "CORTEX_STEP_STATUS_DONE"
	s.Metadata.CreatedAt = createdAt
	s.Metadata.CompletedAt = createdAt
	s.PlannerResponse = &TrajectoryPlannerResponse{
		Response: ">" + " " + label,
	}
	return s
}

// ResolveGlobalStepToSegment maps a stitched global stepIndex on rootCascadeID to the owning
// underlying (segmentCascadeID, localStepIndex).
func (p *Proxy) ResolveGlobalStepToSegment(ctx context.Context, cascadeID string, globalStepIndex int, port int, token string) (string, int) {
	segments := GetStitchChainSegments(cascadeID)
	if len(segments) <= 1 || globalStepIndex < 0 {
		return ResolveActiveCascadeID(cascadeID), globalStepIndex
	}
	offset := 0
	for i, seg := range segments {
		if i > 0 {
			// Account for the 1 synthetic divider step inserted before each child segment
			if globalStepIndex == offset {
				return seg.CascadeID, 0
			}
			offset++
		}
		raw, err := p.fetchSingleSegmentTrajectory(ctx, seg.CascadeID, port, token, 5*time.Second)
		if err != nil || raw == nil {
			continue
		}
		n := len(raw.Trajectory.Steps)
		if globalStepIndex < offset+n || i == len(segments)-1 {
			local := globalStepIndex - offset
			if local < 0 {
				local = 0
			}
			return seg.CascadeID, local
		}
		offset += n
	}
	return ResolveActiveCascadeID(cascadeID), globalStepIndex
}
