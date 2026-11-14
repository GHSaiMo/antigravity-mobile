package proxy

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// SwarmBrief is a lightweight snapshot of active swarm state embedded in StreamUpdatePayload.
type SwarmBrief struct {
	Active         bool   `json:"active"`
	CoordinatorID  string `json:"coordinatorId"`
	Title          string `json:"title"`
	Phase          string `json:"phase,omitempty"`
	Headline       string `json:"headline,omitempty"`
	WorkingCount   int    `json:"workingCount"`
	TotalMembers   int    `json:"totalMembers"`
	LatestSeq      int    `json:"latestSeq"`
	AttentionCount int    `json:"attentionCount"`
}

// SwarmMemberInfo represents a member in members.json enriched with runtime activity.
type SwarmMemberInfo struct {
	Name          string `json:"name"`
	ConversationID string `json:"conversationId"`
	Model         string `json:"model"`
	Role          string `json:"role"`
	WakeMode      string `json:"wakeMode"`
	WaitSince     string `json:"waitSince,omitempty"`
	LastReadSeq   int    `json:"lastReadSeq"`
	LastNudgedSeq int    `json:"lastNudgedSeq"`
	Killed        bool   `json:"killed"`
	Status        string `json:"status"` // "working", "waiting", "killed", "error", "stuck"
	LastActivity  string `json:"lastActivity,omitempty"`
	UnreadCount   int    `json:"unreadCount"`
	IsCurrent     bool   `json:"isCurrent"`
}

// SwarmBoardMessage is one line from board.jsonl.
type SwarmBoardMessage struct {
	Seq      int      `json:"seq"`
	Agent    string   `json:"agent"`
	Text     string   `json:"text"`
	Time     string   `json:"time"`
	Channel  string   `json:"channel,omitempty"`
	Mentions []string `json:"mentions,omitempty"`
}

// SwarmViewAttention represents an actionable item in view.json.
type SwarmViewAttention struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`     // decision, failure, review, stuck, quota, error, fyi
	Severity string `json:"severity"` // high, med, low
	Title    string `json:"title"`
	Why      string `json:"why,omitempty"`
}

// SwarmViewWorkstream represents a stream of work in view.json.
type SwarmViewWorkstream struct {
	ID     string   `json:"id"`
	Title  string   `json:"title"`
	Owners []string `json:"owners,omitempty"`
	State  string   `json:"state"` // queued, active, blocked, review, done, failed
	Note   string   `json:"note,omitempty"`
}

// SwarmView mirrors the structured summary in view.json written by the swarm narrator.
type SwarmView struct {
	Version   flexString `json:"version,omitempty"`
	AsOfSeq   int        `json:"as_of_seq,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
	Headline  string `json:"headline,omitempty"`
	Phase     *struct {
		Label string `json:"label"`
		Index int    `json:"index"`
		Total int    `json:"total"`
	} `json:"phase,omitempty"`
	Attention   []SwarmViewAttention   `json:"attention,omitempty"`
	Workstreams []SwarmViewWorkstream  `json:"workstreams,omitempty"`
	Milestones  []struct {
		ID   string `json:"id"`
		Text string `json:"text"`
	} `json:"milestones,omitempty"`
	Risks []struct {
		ID   string `json:"id"`
		Text string `json:"text"`
	} `json:"risks,omitempty"`
}

// SwarmSnapshot is returned by GET /gateway/swarm.
type SwarmSnapshot struct {
	CascadeID     string              `json:"cascadeId"`
	CoordinatorID string              `json:"coordinatorId"`
	Title         string              `json:"title"`
	ETag          string              `json:"etag"`
	Brief         SwarmBrief          `json:"brief"`
	Members       []SwarmMemberInfo   `json:"members"`
	Messages      []SwarmBoardMessage `json:"messages"`
	View          *SwarmView          `json:"view,omitempty"`
}

type swarmBrainTopology struct {
	brainModTime  int64
	scannedAt     time.Time
	knownCoords   map[string]int64  // coordID -> members.json mtime
	noSwarmDirs   map[string]int64  // non-swarm dirName -> dir mtime
	memberToCoord map[string]string // memberConvID -> coordID
}

const swarmTopologyScanTTL = 3 * time.Second

var (
	mentionRegex        = regexp.MustCompile(`@([a-zA-Z0-9_\-]+)`)
	narratorNameRegex   = regexp.MustCompile(`^narrator(-\d+)?$`)
	swarmCoordMu        sync.RWMutex
	memberToCoordCache  = make(map[string]string)
	swarmTopologyByBase = make(map[string]*swarmBrainTopology)
)

// flexString unmarshals either a JSON string, number, or null into a string.
// Antigravity 2.31+ writes wait_since as an integer sequence number in members.json,
// while older/mock fixtures may use a string.
type flexString string

func (f *flexString) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		*f = ""
		return nil
	}
	if trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return err
		}
		*f = flexString(s)
		return nil
	}
	*f = flexString(string(trimmed))
	return nil
}

func indexCoordinatorMembersLocked(topo *swarmBrainTopology, brainBase, coordID string, mtime int64) {
	membersPath := filepath.Join(brainBase, coordID, ".system_generated", "swarm", "members.json")
	data, err := os.ReadFile(membersPath)
	if err != nil {
		delete(topo.knownCoords, coordID)
		return
	}
	var raw struct {
		Members map[string]struct {
			ConversationID string `json:"conversation_id"`
		} `json:"members"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return
	}
	topo.knownCoords[coordID] = mtime
	for _, m := range raw.Members {
		if m.ConversationID != "" {
			topo.memberToCoord[m.ConversationID] = coordID
			if len(memberToCoordCache) > 2048 {
				memberToCoordCache = make(map[string]string)
			}
			memberToCoordCache[m.ConversationID] = coordID
		}
	}
}

// resolveSwarmDir locates the .system_generated/swarm directory for a conversation ID.
// If the conversation is itself the coordinator, returns its own swarm dir.
// If it is a worker member, finds the coordinator owning this member using a bounded topology cache
// so non-swarm conversations incur at most two stat syscalls instead of scanning brain/.
func resolveSwarmDir(cascadeID string) (string, string) {
	if cascadeID == "" {
		return "", ""
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", ""
	}
	brainBase := filepath.Join(home, ".gemini", "antigravity", "brain")

	// 1. Direct check: is cascadeID the coordinator?
	directDir := filepath.Join(brainBase, cascadeID, ".system_generated", "swarm")
	if fi, err := os.Stat(filepath.Join(directDir, "members.json")); err == nil && !fi.IsDir() {
		return directDir, cascadeID
	}

	// 2. Positive cache check
	swarmCoordMu.RLock()
	topo := swarmTopologyByBase[brainBase]
	cachedCoord := ""
	if topo != nil {
		cachedCoord = topo.memberToCoord[cascadeID]
	}
	if cachedCoord == "" {
		cachedCoord = memberToCoordCache[cascadeID]
	}
	swarmCoordMu.RUnlock()
	if cachedCoord != "" {
		cachedDir := filepath.Join(brainBase, cachedCoord, ".system_generated", "swarm")
		if fi, err := os.Stat(filepath.Join(cachedDir, "members.json")); err == nil && !fi.IsDir() {
			return cachedDir, cachedCoord
		}
	}

	// 3. Topology & negative cache check
	brainFi, err := os.Stat(brainBase)
	if err != nil || !brainFi.IsDir() {
		return "", ""
	}
	brainMtime := brainFi.ModTime().UnixNano()

	swarmCoordMu.Lock()
	defer swarmCoordMu.Unlock()

	if len(swarmTopologyByBase) > 8 {
		swarmTopologyByBase = make(map[string]*swarmBrainTopology)
	}
	topo = swarmTopologyByBase[brainBase]
	if topo == nil {
		topo = &swarmBrainTopology{
			knownCoords:   make(map[string]int64),
			noSwarmDirs:   make(map[string]int64),
			memberToCoord: make(map[string]string),
		}
		swarmTopologyByBase[brainBase] = topo
	}

	// Fast negative hit when brain/ mtime and known coordinator members.json mtimes haven't changed within TTL.
	if !topo.scannedAt.IsZero() && time.Since(topo.scannedAt) < swarmTopologyScanTTL && topo.brainModTime == brainMtime {
		for coordID, prevMod := range topo.knownCoords {
			mPath := filepath.Join(brainBase, coordID, ".system_generated", "swarm", "members.json")
			if fi, sErr := os.Stat(mPath); sErr == nil && !fi.IsDir() {
				if fi.ModTime().UnixNano() != prevMod {
					indexCoordinatorMembersLocked(topo, brainBase, coordID, fi.ModTime().UnixNano())
				}
			} else {
				delete(topo.knownCoords, coordID)
			}
		}
		if coordID := topo.memberToCoord[cascadeID]; coordID != "" {
			return filepath.Join(brainBase, coordID, ".system_generated", "swarm"), coordID
		}
		return "", ""
	}

	entries, err := os.ReadDir(brainBase)
	if err != nil {
		return "", ""
	}
	if len(topo.noSwarmDirs) > 4096 {
		topo.noSwarmDirs = make(map[string]int64)
	}
	if len(topo.memberToCoord) > 2048 {
		topo.memberToCoord = make(map[string]string)
	}

	for _, e := range entries {
		if !e.IsDir() || e.Name() == cascadeID {
			continue
		}
		dirName := e.Name()
		var dirMod int64
		if info, iErr := e.Info(); iErr == nil {
			dirMod = info.ModTime().UnixNano()
		}
		if prevNoMod, isNoSwarm := topo.noSwarmDirs[dirName]; isNoSwarm && prevNoMod == dirMod && dirMod != 0 {
			continue
		}
		mPath := filepath.Join(brainBase, dirName, ".system_generated", "swarm", "members.json")
		fi, sErr := os.Stat(mPath)
		if sErr != nil || fi.IsDir() {
			topo.noSwarmDirs[dirName] = dirMod
			delete(topo.knownCoords, dirName)
			continue
		}
		delete(topo.noSwarmDirs, dirName)
		mMod := fi.ModTime().UnixNano()
		if prevMod, ok := topo.knownCoords[dirName]; !ok || prevMod != mMod {
			indexCoordinatorMembersLocked(topo, brainBase, dirName, mMod)
		}
	}

	topo.brainModTime = brainMtime
	topo.scannedAt = time.Now()

	if coordID := topo.memberToCoord[cascadeID]; coordID != "" {
		return filepath.Join(brainBase, coordID, ".system_generated", "swarm"), coordID
	}
	return "", ""
}

type rawSwarmMemberEntry struct {
	ConversationID string     `json:"conversation_id"`
	Model          string     `json:"model"`
	Role           string     `json:"role"`
	WakeMode       string     `json:"wake_mode"`
	WaitSince      flexString `json:"wait_since"`
	LastReadSeq    int        `json:"last_read_seq"`
	LastNudgedSeq  int        `json:"last_nudged_seq"`
	Killed         bool       `json:"killed"`
}

// resolveSwarmViewPath resolves <swarmDir>/view.json first, falling back to
// <brainBase>/<narratorConvId>/swarm_view/view.json per Antigravity 2.32.1 swarm_board/main.mjs.
func resolveSwarmViewPath(brainBase, swarmDir string, members map[string]rawSwarmMemberEntry) (string, int64) {
	primary := filepath.Join(swarmDir, "view.json")
	if fi, err := os.Stat(primary); err == nil && !fi.IsDir() {
		return primary, fi.ModTime().UnixNano()
	}
	if brainBase == "" || len(members) == 0 {
		return primary, 0
	}
	names := make([]string, 0, len(members))
	for name := range members {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		m := members[name]
		if m.ConversationID == "" {
			continue
		}
		if m.Role == "narrator" || narratorNameRegex.MatchString(name) {
			fallback := filepath.Join(brainBase, m.ConversationID, "swarm_view", "view.json")
			if fi, err := os.Stat(fallback); err == nil && !fi.IsDir() {
				return fallback, fi.ModTime().UnixNano()
			}
		}
	}
	return primary, 0
}

// computeMembersTranscriptDigest computes a fast FNV-1a digest of active members' transcript.jsonl
// mtime and byte size so ETag and stream signatures reflect live worker progress immediately.
func computeMembersTranscriptDigest(brainBase string, members map[string]rawSwarmMemberEntry) uint64 {
	if brainBase == "" || len(members) == 0 {
		return 0
	}
	names := make([]string, 0, len(members))
	for name, m := range members {
		if !m.Killed && m.ConversationID != "" {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return 0
	}
	sort.Strings(names)
	h := fnv.New64a()
	var scratch [16]byte
	for _, name := range names {
		m := members[name]
		tPath := filepath.Join(brainBase, m.ConversationID, ".system_generated", "logs", "transcript.jsonl")
		if fi, err := os.Stat(tPath); err == nil && !fi.IsDir() {
			_, _ = h.Write([]byte(name))
			_, _ = h.Write(strconv.AppendInt(scratch[:0], fi.ModTime().UnixNano(), 16))
			_, _ = h.Write([]byte{':'})
			_, _ = h.Write(strconv.AppendInt(scratch[:0], fi.Size(), 16))
		}
	}
	return h.Sum64()
}

// GetSwarmSignature returns a lightweight mtime+size signature for the conversation's swarm files
// so stream fast-path polling can detect swarm state changes immediately.
func GetSwarmSignature(cascadeID string) string {
	swarmDir, coordID := resolveSwarmDir(cascadeID)
	if swarmDir == "" {
		return ""
	}
	var mMod, mSize, bMod, bSize, vMod int64
	membersPath := filepath.Join(swarmDir, "members.json")
	if fi, err := os.Stat(membersPath); err == nil {
		mMod = fi.ModTime().UnixNano()
		mSize = fi.Size()
	}
	if fi, err := os.Stat(filepath.Join(swarmDir, "board.jsonl")); err == nil {
		bMod = fi.ModTime().UnixNano()
		bSize = fi.Size()
	}

	home, _ := os.UserHomeDir()
	brainBase := ""
	if home != "" {
		brainBase = filepath.Join(home, ".gemini", "antigravity", "brain")
	}

	var tDigest uint64
	if membersData, err := os.ReadFile(membersPath); err == nil {
		var rawMembers struct {
			Members map[string]rawSwarmMemberEntry `json:"members"`
		}
		if json.Unmarshal(membersData, &rawMembers) == nil {
			_, vMod = resolveSwarmViewPath(brainBase, swarmDir, rawMembers.Members)
			tDigest = computeMembersTranscriptDigest(brainBase, rawMembers.Members)
		}
	} else if fi, err := os.Stat(filepath.Join(swarmDir, "view.json")); err == nil {
		vMod = fi.ModTime().UnixNano()
	}
	return fmt.Sprintf("%s:%d-%d:%d-%d:%d:%x", coordID, mMod, mSize, bMod, bSize, vMod, tDigest)
}

// GetSwarmBrief extracts a lightweight brief of the swarm for streaming.
func GetSwarmBrief(cascadeID string) *SwarmBrief {
	swarmDir, coordID := resolveSwarmDir(cascadeID)
	if swarmDir == "" {
		return nil
	}

	membersData, err := os.ReadFile(filepath.Join(swarmDir, "members.json"))
	if err != nil {
		return nil
	}

	var rawMembers struct {
		Title   string                         `json:"title"`
		Members map[string]rawSwarmMemberEntry `json:"members"`
	}
	if err := json.Unmarshal(membersData, &rawMembers); err != nil {
		return nil
	}

	home, _ := os.UserHomeDir()
	brainBase := ""
	if home != "" {
		brainBase = filepath.Join(home, ".gemini", "antigravity", "brain")
	}

	total := len(rawMembers.Members)
	working := 0
	for _, m := range rawMembers.Members {
		if m.Killed {
			continue
		}
		status := "working"
		if brainBase != "" && m.ConversationID != "" {
			tPath := filepath.Join(brainBase, m.ConversationID, ".system_generated", "logs", "transcript.jsonl")
			if st, _ := readMemberTranscriptActivity(tPath); st != "" {
				status = st
			}
		}
		if status == "working" {
			working++
		}
	}

	brief := &SwarmBrief{
		Active:        true,
		CoordinatorID: coordID,
		Title:         rawMembers.Title,
		TotalMembers:  total,
		WorkingCount:  working,
	}
	if brief.Title == "" {
		brief.Title = "蜂群协作"
	}

	// Read view.json (primary or narrator fallback) for headline/phase/attention count
	viewPath, _ := resolveSwarmViewPath(brainBase, swarmDir, rawMembers.Members)
	if viewData, err := os.ReadFile(viewPath); err == nil && len(viewData) > 0 {
		var v SwarmView
		if err := json.Unmarshal(viewData, &v); err == nil {
			brief.Headline = v.Headline
			if v.Phase != nil {
				brief.Phase = v.Phase.Label
			}
			brief.AttentionCount = len(v.Attention)
		}
	}

	// Read tail seq of board.jsonl
	if boardFile, err := os.Open(filepath.Join(swarmDir, "board.jsonl")); err == nil {
		defer boardFile.Close()
		scanner := bufio.NewScanner(boardFile)
		scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		latestSeq := 0
		for scanner.Scan() {
			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}
			var m struct {
				Seq int `json:"seq"`
			}
			if err := json.Unmarshal(line, &m); err == nil && m.Seq > latestSeq {
				latestSeq = m.Seq
			}
		}
		brief.LatestSeq = latestSeq
	}

	return brief
}

// readMemberTranscriptActivity inspects the last line(s) of a member's transcript.jsonl
// to extract its live runtime status ("working", "waiting", "error") and activity summary.
func readMemberTranscriptActivity(transcriptPath string) (statusOverride string, lastActivity string) {
	tFile, err := os.Open(transcriptPath)
	if err != nil {
		return "", ""
	}
	defer tFile.Close()

	const tailWindow = 256 * 1024
	discardFirstPartialLine := false
	if fi, sErr := tFile.Stat(); sErr == nil && fi.Size() > tailWindow {
		if _, seekErr := tFile.Seek(fi.Size()-tailWindow, io.SeekStart); seekErr == nil {
			discardFirstPartialLine = true
		}
	}

	scanner := bufio.NewScanner(tFile)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	if discardFirstPartialLine {
		_ = scanner.Scan()
	}
	var prevLine, lastLine []byte
	for scanner.Scan() {
		b := scanner.Bytes()
		if len(b) > 0 {
			prevLine = append(prevLine[:0], lastLine...)
			lastLine = append(lastLine[:0], b...)
		}
	}
	if len(lastLine) == 0 {
		return "", ""
	}

	type transcriptStep struct {
		Type      string `json:"type"`
		Status    string `json:"status"`
		Content   string `json:"content"`
		ToolCalls []struct {
			Name string `json:"name"`
		} `json:"tool_calls"`
		// Legacy camelCase fallbacks
		ToolCall *struct {
			Name        string `json:"name"`
			ToolSummary string `json:"toolSummary"`
		} `json:"toolCall"`
		PlannerResponse *struct {
			Response string `json:"response"`
		} `json:"plannerResponse"`
		ErrorMessage *struct {
			Error struct {
				ShortError string `json:"shortError"`
			} `json:"error"`
		} `json:"errorMessage"`
	}

	var step transcriptStep
	if err := json.Unmarshal(lastLine, &step); err != nil {
		return "", ""
	}

	if step.Status == "ERROR" || step.Type == "ERROR_MESSAGE" {
		msg := strings.TrimSpace(step.Content)
		if msg == "" && step.ErrorMessage != nil {
			msg = step.ErrorMessage.Error.ShortError
		}
		if len(msg) > 120 {
			msg = msg[:120] + "..."
		}
		return "error", msg
	}
	if step.ErrorMessage != nil && step.ErrorMessage.Error.ShortError != "" {
		return "error", step.ErrorMessage.Error.ShortError
	}

	if step.ToolCall != nil {
		if step.ToolCall.ToolSummary != "" {
			return "working", step.ToolCall.ToolSummary
		}
		return "working", "调用工具 " + step.ToolCall.Name
	}

	if len(step.ToolCalls) > 0 {
		names := make([]string, 0, len(step.ToolCalls))
		hasSwarmWait := false
		for _, tc := range step.ToolCalls {
			if tc.Name != "" {
				names = append(names, tc.Name)
			}
			if tc.Name == "swarm_wait" {
				hasSwarmWait = true
			}
		}
		if hasSwarmWait && step.Status == "DONE" && len(names) == 1 {
			return "waiting", "等待唤醒 (swarm_wait)"
		}
		if len(names) > 0 {
			return "working", "调用工具 " + strings.Join(names, ", ")
		}
	}

	if step.Type == "PLANNER_RESPONSE" {
		if step.Status == "DONE" && len(step.ToolCalls) == 0 {
			return "waiting", "等待新消息"
		}
		return "working", "思考与回复中..."
	}

	if step.Type == "GENERIC" && step.Status == "DONE" {
		if strings.Contains(step.Content, "Waiting for a message") {
			return "waiting", "等待唤醒 (swarm_wait)"
		}
		// Check if previous planner step called swarm_wait
		if len(prevLine) > 0 {
			var prevStep transcriptStep
			if json.Unmarshal(prevLine, &prevStep) == nil {
				for _, tc := range prevStep.ToolCalls {
					if tc.Name == "swarm_wait" {
						return "waiting", "等待唤醒 (swarm_wait)"
					}
				}
			}
		}
		return "working", "处理工具返回..."
	}

	if step.PlannerResponse != nil && step.PlannerResponse.Response != "" {
		return "working", "回复中..."
	}
	return "", ""
}

// GetSwarmSnapshot returns full details of the swarm, checking ETag.
func GetSwarmSnapshot(cascadeID string, afterSeq int, clientETag string) (*SwarmSnapshot, bool, error) {
	swarmDir, coordID := resolveSwarmDir(cascadeID)
	if swarmDir == "" {
		return nil, false, os.ErrNotExist
	}

	membersPath := filepath.Join(swarmDir, "members.json")
	boardPath := filepath.Join(swarmDir, "board.jsonl")

	fiMembers, err := os.Stat(membersPath)
	if err != nil {
		return nil, false, err
	}
	var boardMod int64
	var boardSize int64
	if fi, err := os.Stat(boardPath); err == nil {
		boardMod = fi.ModTime().UnixNano()
		boardSize = fi.Size()
	}

	membersData, err := os.ReadFile(membersPath)
	if err != nil {
		return nil, false, err
	}
	var rawMembers struct {
		Title   string                         `json:"title"`
		Members map[string]rawSwarmMemberEntry `json:"members"`
	}
	if err := json.Unmarshal(membersData, &rawMembers); err != nil {
		return nil, false, err
	}

	home, _ := os.UserHomeDir()
	brainBase := ""
	if home != "" {
		brainBase = filepath.Join(home, ".gemini", "antigravity", "brain")
	}

	viewPath, viewMod := resolveSwarmViewPath(brainBase, swarmDir, rawMembers.Members)
	tDigest := computeMembersTranscriptDigest(brainBase, rawMembers.Members)

	currentETag := fmt.Sprintf(`"%d-%d-%d-%d-%d-%x"`, fiMembers.ModTime().UnixNano(), fiMembers.Size(), boardMod, boardSize, viewMod, tDigest)
	if clientETag != "" && clientETag == currentETag {
		return nil, true, nil
	}

	// Read board.jsonl
	messages := make([]SwarmBoardMessage, 0)
	latestSeq := 0
	if boardFile, err := os.Open(boardPath); err == nil {
		defer boardFile.Close()
		scanner := bufio.NewScanner(boardFile)
		scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		for scanner.Scan() {
			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}
			var msg SwarmBoardMessage
			if err := json.Unmarshal(line, &msg); err == nil {
				if msg.Seq > latestSeq {
					latestSeq = msg.Seq
				}
				if msg.Seq > afterSeq {
					// extract mentions
					found := mentionRegex.FindAllStringSubmatch(msg.Text, -1)
					mentions := make([]string, 0, len(found))
					for _, m := range found {
						if len(m) > 1 {
							mentions = append(mentions, m[1])
						}
					}
					msg.Mentions = mentions
					messages = append(messages, msg)
				}
			}
		}
	}

	members := make([]SwarmMemberInfo, 0, len(rawMembers.Members))
	workingCount := 0
	for name, rm := range rawMembers.Members {
		status := "waiting"
		if rm.Killed {
			status = "killed"
		} else {
			status = "working"
		}

		lastActivity := ""
		if !rm.Killed && rm.ConversationID != "" {
			tPath := filepath.Join(brainBase, rm.ConversationID, ".system_generated", "logs", "transcript.jsonl")
			if st, act := readMemberTranscriptActivity(tPath); st != "" || act != "" {
				if st != "" {
					status = st
				}
				lastActivity = act
			}
		}
		if status == "working" {
			workingCount++
		}

		unread := latestSeq - rm.LastReadSeq
		if unread < 0 {
			unread = 0
		}

		members = append(members, SwarmMemberInfo{
			Name:           name,
			ConversationID: rm.ConversationID,
			Model:          rm.Model,
			Role:           rm.Role,
			WakeMode:       rm.WakeMode,
			WaitSince:      string(rm.WaitSince),
			LastReadSeq:    rm.LastReadSeq,
			LastNudgedSeq:  rm.LastNudgedSeq,
			Killed:         rm.Killed,
			Status:         status,
			LastActivity:   lastActivity,
			UnreadCount:    unread,
			IsCurrent:      rm.ConversationID == cascadeID,
		})
	}
	sort.Slice(members, func(i, j int) bool {
		if members[i].Role != members[j].Role {
			if members[i].Role == "coordinator" {
				return true
			}
			if members[j].Role == "coordinator" {
				return false
			}
		}
		return members[i].Name < members[j].Name
	})

	var view *SwarmView
	if viewData, err := os.ReadFile(viewPath); err == nil && len(viewData) > 0 {
		var v SwarmView
		if err := json.Unmarshal(viewData, &v); err == nil {
			view = &v
		}
	}

	title := rawMembers.Title
	if title == "" {
		title = "蜂群协作"
	}

	brief := SwarmBrief{
		Active:        true,
		CoordinatorID: coordID,
		Title:         title,
		WorkingCount:  workingCount,
		TotalMembers:  len(members),
		LatestSeq:     latestSeq,
	}
	if view != nil {
		brief.Headline = view.Headline
		if view.Phase != nil {
			brief.Phase = view.Phase.Label
		}
		brief.AttentionCount = len(view.Attention)
	}

	snap := &SwarmSnapshot{
		CascadeID:     cascadeID,
		CoordinatorID: coordID,
		Title:         title,
		ETag:          currentETag,
		Brief:         brief,
		Members:       members,
		Messages:      messages,
		View:          view,
	}

	return snap, false, nil
}

// HandleGetSwarm serves GET /gateway/swarm.
func (p *Proxy) HandleGetSwarm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	cascadeID := r.URL.Query().Get("cascadeId")
	if cascadeID == "" {
		cascadeID = r.URL.Query().Get("cascade_id")
	}
	if cascadeID == "" {
		writeJSONError(w, "missing cascadeId", http.StatusBadRequest)
		return
	}

	afterSeq := 0
	if s := r.URL.Query().Get("afterSeq"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n >= 0 {
			afterSeq = n
		}
	} else if s := r.URL.Query().Get("after_seq"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n >= 0 {
			afterSeq = n
		}
	}
	clientETag := r.Header.Get("If-None-Match")

	snap, notModified, err := GetSwarmSnapshot(cascadeID, afterSeq, clientETag)
	if err != nil {
		if os.IsNotExist(err) {
			writeJSONError(w, "no active swarm found for this conversation", http.StatusNotFound)
			return
		}
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if notModified {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("ETag", snap.ETag)
	_ = json.NewEncoder(w).Encode(snap)
}

// HandleSwarmFeedback serves POST /gateway/swarm/feedback.
func (p *Proxy) HandleSwarmFeedback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		CascadeID     string `json:"cascadeId"`
		CascadeIDAlt  string `json:"cascade_id"`
		CoordinatorID string `json:"coordinatorId"`
		Feedback      string `json:"feedback"`
		Content       string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "invalid request: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.CascadeID == "" {
		req.CascadeID = req.CascadeIDAlt
	}
	feedback := strings.TrimSpace(req.Feedback)
	if feedback == "" {
		feedback = strings.TrimSpace(req.Content)
	}
	if feedback == "" {
		writeJSONError(w, "feedback text cannot be empty", http.StatusBadRequest)
		return
	}

	targetConv := req.CoordinatorID
	if targetConv == "" {
		_, coord := resolveSwarmDir(req.CascadeID)
		targetConv = coord
	}
	if targetConv == "" {
		targetConv = req.CascadeID
	}

	msg := fmt.Sprintf("@narrator user feedback: %s", feedback)
	if err := p.sendCascadeTextMessage(targetConv, msg); err != nil {
		slog.Warn("[Swarm] Failed to send feedback to coordinator", "conv", targetConv, "err", err)
		writeJSONError(w, "failed to dispatch feedback: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok", "dispatchedTo": targetConv})
}

// HandleSwarmNudge serves POST /gateway/swarm/nudge.
func (p *Proxy) HandleSwarmNudge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		CascadeID     string `json:"cascadeId"`
		CascadeIDAlt  string `json:"cascade_id"`
		CoordinatorID string `json:"coordinatorId"`
		WorkerName    string `json:"workerName"`
		Member        string `json:"member"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "invalid request: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.CascadeID == "" {
		req.CascadeID = req.CascadeIDAlt
	}
	worker := strings.TrimSpace(req.WorkerName)
	if worker == "" {
		worker = strings.TrimSpace(req.Member)
	}
	if worker == "" {
		writeJSONError(w, "workerName cannot be empty", http.StatusBadRequest)
		return
	}

	targetConv := req.CoordinatorID
	if targetConv == "" {
		_, coord := resolveSwarmDir(req.CascadeID)
		targetConv = coord
	}
	if targetConv == "" {
		targetConv = req.CascadeID
	}

	msg := fmt.Sprintf("@%s 请汇报当前进展与卡点", worker)
	if err := p.sendCascadeTextMessage(targetConv, msg); err != nil {
		slog.Warn("[Swarm] Failed to send nudge to worker", "worker", worker, "conv", targetConv, "err", err)
		writeJSONError(w, "failed to dispatch nudge: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok", "worker": worker, "dispatchedTo": targetConv})
}

// sendCascadeTextMessage sends a single text prompt to the upstream language_server for a cascade.
func (p *Proxy) sendCascadeTextMessage(cascadeID, text string) error {
	port, token := p.ActiveUpstream()
	if p.insp != nil {
		if cur := p.insp.Current(); cur != nil && cur.IsHealthy && cur.Port > 0 {
			port = cur.Port
			token = cur.CSRFToken
		}
	}
	if port <= 0 {
		return fmt.Errorf("upstream language_server unavailable")
	}

	msgPayload := map[string]interface{}{
		"cascadeId": cascadeID,
		"items": []map[string]interface{}{
			{"text": text},
		},
	}
	msgBytes, err := json.Marshal(msgPayload)
	if err != nil {
		return err
	}

	msgURL := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/SendUserCascadeMessage", port)
	msgReq, err := http.NewRequest(http.MethodPost, msgURL, strings.NewReader(string(msgBytes)))
	if err != nil {
		return err
	}
	msgReq.Header.Set("Content-Type", "application/json")
	msgReq.Header.Set("Connect-Protocol-Version", "1")
	if token != "" {
		msgReq.Header.Set("x-codeium-csrf-token", token)
	}

	resp, err := p.mediumClient.Do(msgReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("upstream returned status %d", resp.StatusCode)
	}

	ClearTrajectoryCache(cascadeID)
	p.notifyStreamTouch(cascadeID)
	return nil
}
