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
	Name           string `json:"name"`
	ConversationID string `json:"conversationId"`
	Model          string `json:"model"`
	Role           string `json:"role"`
	WakeMode       string `json:"wakeMode"`
	WaitSince      string `json:"waitSince,omitempty"`
	LastReadSeq    int    `json:"lastReadSeq"`
	LastNudgedSeq  int    `json:"lastNudgedSeq"`
	Killed         bool   `json:"killed"`
	Status         string `json:"status"` // "working", "waiting", "killed", "error", "stuck"
	LastActivity   string `json:"lastActivity,omitempty"`
	ActivityKind   string `json:"activityKind,omitempty"` // "tool", "thinking", ""
	Steps          int    `json:"steps,omitempty"`
	Posts          int    `json:"posts,omitempty"`
	LastPostSeq    int    `json:"lastPostSeq,omitempty"`
	Workspace      string `json:"workspace,omitempty"`
	WorkspaceMode  string `json:"workspaceMode,omitempty"`
	RoleWorker     bool   `json:"roleWorker,omitempty"`
	UnreadCount    int    `json:"unreadCount"`
	IsCurrent      bool   `json:"isCurrent"`
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

// SwarmPlanGoal represents goal in plan.json or view.json.
type SwarmPlanGoal struct {
	Title string `json:"title"`
	Line  string `json:"line,omitempty"`
}

// SwarmPlanThread represents a thread entry in plan.json.
type SwarmPlanThread struct {
	ID    string   `json:"id"`
	Title string   `json:"title"`
	Goal  string   `json:"goal,omitempty"`
	Tasks []string `json:"tasks"`
}

// SwarmPlan mirrors plan.json written by the coordinator (swarm_pane skill).
type SwarmPlan struct {
	Goal    *SwarmPlanGoal    `json:"goal,omitempty"`
	Tasks   map[string]string `json:"tasks,omitempty"`
	Threads []SwarmPlanThread `json:"threads,omitempty"`
}

// SwarmViewThread represents a thread in view.json.
type SwarmViewThread struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Goal        string   `json:"goal,omitempty"`
	Now         string   `json:"now,omitempty"`
	State       string   `json:"state,omitempty"`
	Workstreams []string `json:"workstreams,omitempty"`
	Attention   []string `json:"attention,omitempty"`
}

// SwarmView mirrors the structured summary in view.json written by the swarm narrator.
type SwarmView struct {
	Version        flexString     `json:"version,omitempty"`
	AsOfSeq        int            `json:"as_of_seq,omitempty"`
	UpdatedAt      string         `json:"updated_at,omitempty"`
	Headline       string         `json:"headline,omitempty"`
	Goal           *SwarmPlanGoal `json:"goal,omitempty"`
	RunningSummary string         `json:"running_summary,omitempty"`
	Phase          *struct {
		Label string `json:"label"`
		Index int    `json:"index"`
		Total int    `json:"total"`
	} `json:"phase,omitempty"`
	Attention   []SwarmViewAttention  `json:"attention,omitempty"`
	Workstreams []SwarmViewWorkstream `json:"workstreams,omitempty"`
	Threads     []SwarmViewThread     `json:"threads,omitempty"`
	Now         map[string]string     `json:"now,omitempty"`
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
	TotalMessages int                 `json:"totalMessages,omitempty"`
	HasMoreBefore bool                `json:"hasMoreBefore,omitempty"`
	OldestSeq     int                 `json:"oldestSeq,omitempty"`
	View          *SwarmView          `json:"view,omitempty"`
	Plan          *SwarmPlan          `json:"plan,omitempty"`
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
	roleWorkerNameRegex = regexp.MustCompile(`^(narrator|monitor)(-\d+)?$`)
	thinkingHeadingRe   = regexp.MustCompile(`(?:^|\n)\s*\*\*([^*\n]{3,96})\*\*\s*(?:\n|$)`)
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
	if !cascadeIDRe.MatchString(coordID) {
		return
	}
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
		if m.ConversationID != "" && cascadeIDRe.MatchString(m.ConversationID) {
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
	if cascadeID == "" || !cascadeIDRe.MatchString(cascadeID) {
		return "", ""
	}
	for _, agDir := range antigravityHomeCandidates() {
		if dir, coordID := resolveSwarmDirInBase(filepath.Join(agDir, "brain"), cascadeID); dir != "" {
			return dir, coordID
		}
	}
	return "", ""
}

func resolveSwarmDirInBase(brainBase, cascadeID string) (string, string) {
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
	Workspace      string     `json:"workspace"`
	WorkspaceMode  string     `json:"workspace_mode"`
}

func isSwarmRoleWorker(name, role string) bool {
	if role != "" && role != "worker" && role != "coordinator" {
		return true
	}
	return roleWorkerNameRegex.MatchString(name)
}

func readSwarmPlan(swarmDir string) (*SwarmPlan, int64, int64) {
	pPath := filepath.Join(swarmDir, "plan.json")
	fi, err := os.Stat(pPath)
	if err != nil || fi.IsDir() {
		return nil, 0, 0
	}
	const maxPlanBytes = 64 * 1024
	if fi.Size() <= 0 || fi.Size() > maxPlanBytes {
		return nil, fi.ModTime().UnixNano(), fi.Size()
	}
	data, err := os.ReadFile(pPath)
	if err != nil || len(data) == 0 {
		return nil, fi.ModTime().UnixNano(), fi.Size()
	}
	var plan SwarmPlan
	if err := json.Unmarshal(data, &plan); err != nil {
		return nil, fi.ModTime().UnixNano(), fi.Size()
	}
	return &plan, fi.ModTime().UnixNano(), fi.Size()
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
		if m.ConversationID == "" || !cascadeIDRe.MatchString(m.ConversationID) {
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
		if !m.Killed && m.ConversationID != "" && cascadeIDRe.MatchString(m.ConversationID) {
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
	var mMod, mSize, bMod, bSize, vMod, pMod, pSize int64
	membersPath := filepath.Join(swarmDir, "members.json")
	if fi, err := os.Stat(membersPath); err == nil {
		mMod = fi.ModTime().UnixNano()
		mSize = fi.Size()
	}
	if fi, err := os.Stat(filepath.Join(swarmDir, "board.jsonl")); err == nil {
		bMod = fi.ModTime().UnixNano()
		bSize = fi.Size()
	}
	if fi, err := os.Stat(filepath.Join(swarmDir, "plan.json")); err == nil && !fi.IsDir() {
		pMod = fi.ModTime().UnixNano()
		pSize = fi.Size()
	}

	brainBase := swarmBrainBaseFromSwarmDir(swarmDir)

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
	return fmt.Sprintf("%s:%d-%d:%d-%d:%d:%d-%d:%x", coordID, mMod, mSize, bMod, bSize, vMod, pMod, pSize, tDigest)
}

type cachedSwarmBriefEntry struct {
	sig   string
	brief SwarmBrief
}

var (
	swarmBriefCacheMu   sync.RWMutex
	swarmBriefCache     = make(map[string]cachedSwarmBriefEntry)
	swarmMembersCacheMu sync.RWMutex
	swarmMembersCache   = make(map[string]cachedSwarmMembersEntry)
	swarmWorkerNameRe   = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,31}$`)
)

type cachedSwarmMembersEntry struct {
	modTime int64
	size    int64
	members map[string]rawSwarmMemberEntry
}

// SwarmListSummary is lightweight swarm metadata attached to conversation list entries.
type SwarmListSummary struct {
	HasSwarm           bool
	WorkingCount       int
	TotalMembers       int
	LatestActivityTime time.Time
}

func loadCachedSwarmMembers(cacheKey, membersPath string, fi os.FileInfo) (map[string]rawSwarmMemberEntry, error) {
	modTime := fi.ModTime().UnixNano()
	size := fi.Size()

	swarmMembersCacheMu.RLock()
	if entry, ok := swarmMembersCache[cacheKey]; ok && entry.modTime == modTime && entry.size == size {
		members := entry.members
		swarmMembersCacheMu.RUnlock()
		return members, nil
	}
	swarmMembersCacheMu.RUnlock()

	data, err := os.ReadFile(membersPath)
	if err != nil {
		return nil, err
	}
	var raw struct {
		Members map[string]rawSwarmMemberEntry `json:"members"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	if raw.Members == nil {
		raw.Members = make(map[string]rawSwarmMemberEntry)
	}

	swarmMembersCacheMu.Lock()
	if len(swarmMembersCache) > 128 {
		swarmMembersCache = make(map[string]cachedSwarmMembersEntry)
	}
	swarmMembersCache[cacheKey] = cachedSwarmMembersEntry{
		modTime: modTime,
		size:    size,
		members: raw.Members,
	}
	swarmMembersCacheMu.Unlock()
	return raw.Members, nil
}

func swarmBrainBaseFromSwarmDir(swarmDir string) string {
	if swarmDir != "" {
		// swarmDir is <brainBase>/<coordID>/.system_generated/swarm
		if base := filepath.Dir(filepath.Dir(filepath.Dir(swarmDir))); base != "" && base != "." {
			return base
		}
	}
	if agDir := resolveAntigravityHomeDir(); agDir != "" {
		return filepath.Join(agDir, "brain")
	}
	return ""
}

// InspectSwarmCoordinatorForList inspects whether coordID owns a swarm session and summarizes
// its active worker count and latest activity time for the conversation list.
// Non-swarm conversations incur at most two stat syscalls and return nil.
func InspectSwarmCoordinatorForList(
	coordID string,
	rawMemberStatuses map[string]string,
	rawMemberModTimes map[string]time.Time,
) *SwarmListSummary {
	if coordID == "" || !cascadeIDRe.MatchString(coordID) {
		return nil
	}

	var brainBase, swarmDir, membersPath string
	var fiMembers os.FileInfo
	hasInitOnlySwarm := false
	for _, agDir := range antigravityHomeCandidates() {
		candBase := filepath.Join(agDir, "brain")
		candSwarm := filepath.Join(candBase, coordID, ".system_generated", "swarm")
		candMembers := filepath.Join(candSwarm, "members.json")
		if fi, err := os.Stat(candMembers); err == nil && !fi.IsDir() {
			brainBase = candBase
			swarmDir = candSwarm
			membersPath = candMembers
			fiMembers = fi
			break
		}
		if fiSwarm, sErr := os.Stat(filepath.Join(candSwarm, "swarm.json")); sErr == nil && !fiSwarm.IsDir() {
			hasInitOnlySwarm = true
		}
	}
	if fiMembers == nil {
		if hasInitOnlySwarm {
			return &SwarmListSummary{HasSwarm: true}
		}
		return nil
	}

	cacheKey := brainBase + "\x00" + coordID
	members, err := loadCachedSwarmMembers(cacheKey, membersPath, fiMembers)
	if err != nil {
		return &SwarmListSummary{HasSwarm: true}
	}

	totalMembers := len(members)
	workingCount := 0
	var latestMod time.Time
	recordMod := func(t time.Time) {
		if !t.IsZero() && t.After(latestMod) {
			latestMod = t
		}
	}

	for _, m := range members {
		cid := strings.TrimSpace(m.ConversationID)
		if cid != "" {
			if mt, ok := rawMemberModTimes[cid]; ok {
				recordMod(mt)
			}
		}
		if m.Killed {
			continue
		}
		if rawSt, hasUpstream := rawMemberStatuses[cid]; hasUpstream && rawSt != "" {
			if rawSt == "CASCADE_RUN_STATUS_RUNNING" {
				workingCount++
			}
			continue
		}
		wakeMode := strings.TrimSpace(m.WakeMode)
		if strings.EqualFold(wakeMode, "working") {
			workingCount++
		} else if wakeMode == "" && cid != "" && cascadeIDRe.MatchString(cid) {
			tPath := filepath.Join(brainBase, cid, ".system_generated", "logs", "transcript.jsonl")
			if st, _ := readMemberTranscriptActivity(tPath); st == "working" {
				workingCount++
			}
		}
	}

	// While the swarm is actively executing, fold in live filesystem mtimes so the coordinator
	// reflects real-time progress even before upstream flushes worker summaries.
	if workingCount > 0 {
		recordMod(fiMembers.ModTime())
		if fiBoard, bErr := os.Stat(filepath.Join(swarmDir, "board.jsonl")); bErr == nil && !fiBoard.IsDir() {
			recordMod(fiBoard.ModTime())
		}
		if _, vMod := resolveSwarmViewPath(brainBase, swarmDir, members); vMod > 0 {
			recordMod(time.Unix(0, vMod))
		}
		for _, m := range members {
			if m.Killed {
				continue
			}
			cid := strings.TrimSpace(m.ConversationID)
			if cid == "" || !cascadeIDRe.MatchString(cid) {
				continue
			}
			tPath := filepath.Join(brainBase, cid, ".system_generated", "logs", "transcript.jsonl")
			if fi, tErr := os.Stat(tPath); tErr == nil && !fi.IsDir() {
				recordMod(fi.ModTime())
			}
		}
	}

	return &SwarmListSummary{
		HasSwarm:           true,
		WorkingCount:       workingCount,
		TotalMembers:       totalMembers,
		LatestActivityTime: latestMod,
	}
}

// readBoardLatestSeqTail reads the highest sequence number near the end of board.jsonl
// using a bounded reverse-tail seek instead of scanning the entire file from byte 0.
func readBoardLatestSeqTail(boardPath string) int {
	f, err := os.Open(boardPath)
	if err != nil {
		return 0
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil || fi.Size() <= 0 {
		return 0
	}

	for _, tailWindow := range []int64{64 * 1024, 512 * 1024} {
		offset := fi.Size() - tailWindow
		if offset < 0 {
			offset = 0
		}
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			return 0
		}
		buf, err := io.ReadAll(io.LimitReader(f, tailWindow))
		if err != nil || len(buf) == 0 {
			return 0
		}
		if offset > 0 {
			if nl := bytes.IndexByte(buf, '\n'); nl >= 0 {
				buf = buf[nl+1:]
			}
		}
		latestSeq := 0
		lines := bytes.Split(buf, []byte{'\n'})
		for i := len(lines) - 1; i >= 0; i-- {
			line := bytes.TrimSpace(lines[i])
			if len(line) == 0 {
				continue
			}
			var m struct {
				Seq int `json:"seq"`
			}
			if err := json.Unmarshal(line, &m); err == nil && m.Seq > latestSeq {
				latestSeq = m.Seq
				break
			}
		}
		if latestSeq > 0 || offset == 0 {
			return latestSeq
		}
	}
	return 0
}

// GetSwarmBrief extracts a lightweight brief of the swarm for streaming.
func GetSwarmBrief(cascadeID string) *SwarmBrief {
	swarmDir, coordID := resolveSwarmDir(cascadeID)
	if swarmDir == "" {
		return nil
	}

	sig := GetSwarmSignature(cascadeID)
	if sig != "" {
		swarmBriefCacheMu.RLock()
		if entry, ok := swarmBriefCache[coordID]; ok && entry.sig == sig {
			cp := entry.brief
			swarmBriefCacheMu.RUnlock()
			return &cp
		}
		swarmBriefCacheMu.RUnlock()
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

	brainBase := swarmBrainBaseFromSwarmDir(swarmDir)

	total := len(rawMembers.Members)
	working := 0
	for _, m := range rawMembers.Members {
		if m.Killed {
			continue
		}
		status := "working"
		if brainBase != "" && m.ConversationID != "" && cascadeIDRe.MatchString(m.ConversationID) {
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
			if brief.Headline == "" && v.Goal != nil {
				brief.Headline = v.Goal.Title
			}
			if v.Phase != nil {
				brief.Phase = v.Phase.Label
			}
			brief.AttentionCount = len(v.Attention)
		}
	}
	if brief.Headline == "" {
		if plan, _, _ := readSwarmPlan(swarmDir); plan != nil && plan.Goal != nil {
			if plan.Goal.Title != "" {
				brief.Headline = plan.Goal.Title
			} else {
				brief.Headline = plan.Goal.Line
			}
		}
	}

	// Read tail seq of board.jsonl via reverse seek without scanning the entire file.
	brief.LatestSeq = readBoardLatestSeqTail(filepath.Join(swarmDir, "board.jsonl"))

	if sig != "" {
		swarmBriefCacheMu.Lock()
		if len(swarmBriefCache) > 64 {
			swarmBriefCache = make(map[string]cachedSwarmBriefEntry)
		}
		swarmBriefCache[coordID] = cachedSwarmBriefEntry{sig: sig, brief: *brief}
		swarmBriefCacheMu.Unlock()
	}

	return brief
}

type memberTranscriptSummary struct {
	status       string
	activity     string
	activityKind string
	steps        int
}

func unquoteSwarmArg(raw json.RawMessage) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return ""
	}
	var s string
	if err := json.Unmarshal(trimmed, &s); err == nil {
		s = strings.TrimSpace(s)
		if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
			var inner string
			if json.Unmarshal([]byte(s), &inner) == nil {
				return inner
			}
			return s[1 : len(s)-1]
		}
		return s
	}
	return string(trimmed)
}

func oneLineSwarmLabel(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "`", "")
	s = strings.ReplaceAll(s, "**", "")
	s = strings.Join(strings.Fields(s), " ")
	if maxLen > 0 && len(s) > maxLen {
		return strings.TrimSpace(s[:maxLen-1]) + "…"
	}
	return s
}

func describeSwarmToolCall(name string, args map[string]json.RawMessage) string {
	if name == "" {
		return ""
	}
	switch name {
	case "swarm_wait":
		return "Waiting for posts"
	case "read_board":
		return "Reading the board"
	case "post_message":
		return "Posting to the board"
	}
	if summary := unquoteSwarmArg(args["toolSummary"]); summary != "" {
		return oneLineSwarmLabel(summary, 96)
	}
	if action := unquoteSwarmArg(args["toolAction"]); action != "" {
		return oneLineSwarmLabel(action, 96)
	}
	p := unquoteSwarmArg(args["Path"])
	if p == "" {
		p = unquoteSwarmArg(args["AbsolutePath"])
	}
	if p == "" {
		p = unquoteSwarmArg(args["TargetFile"])
	}
	if p != "" {
		base := filepath.Base(strings.ReplaceAll(p, "\\", "/"))
		switch name {
		case "read_file":
			return "Read " + base
		case "write_file":
			return "Write " + base
		case "edit_file":
			return "Edit " + base
		}
	}
	if (name == "code_search" || name == "grep_search") && len(args["Query"]) > 0 {
		if q := unquoteSwarmArg(args["Query"]); q != "" {
			return oneLineSwarmLabel("Search "+q, 96)
		}
	}
	if name == "swarm" && len(args["action"]) > 0 {
		act := unquoteSwarmArg(args["action"])
		target := unquoteSwarmArg(args["name"])
		return oneLineSwarmLabel(strings.TrimSpace("swarm "+act+" "+target), 96)
	}
	return name
}

func extractThinkingHeading(thinking string) string {
	if thinking == "" {
		return ""
	}
	matches := thinkingHeadingRe.FindAllStringSubmatch(thinking, -1)
	if len(matches) == 0 {
		return ""
	}
	last := matches[len(matches)-1]
	if len(last) > 1 {
		return oneLineSwarmLabel(last[1], 96)
	}
	return ""
}

// readMemberTranscriptSummary inspects the tail of a member's transcript.jsonl
// to extract live runtime status ("working", "waiting", "error"), crisp tool/thinking
// activity (matching upstream swarm_board/main.mjs), and step count.
func readMemberTranscriptSummary(transcriptPath string) memberTranscriptSummary {
	tFile, err := os.Open(transcriptPath)
	if err != nil {
		return memberTranscriptSummary{}
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
	var lines [][]byte
	for scanner.Scan() {
		b := bytes.TrimSpace(scanner.Bytes())
		if len(b) > 0 {
			cp := make([]byte, len(b))
			copy(cp, b)
			lines = append(lines, cp)
		}
	}
	if len(lines) == 0 {
		return memberTranscriptSummary{}
	}

	type transcriptToolCall struct {
		Name string                     `json:"name"`
		Args map[string]json.RawMessage `json:"args"`
	}
	type transcriptStep struct {
		StepIndex int                  `json:"step_index"`
		Type      string               `json:"type"`
		Status    string               `json:"status"`
		Content   string               `json:"content"`
		Thinking  string               `json:"thinking"`
		Error     string               `json:"error"`
		ToolCalls []transcriptToolCall `json:"tool_calls"`
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

	var newest transcriptStep
	if err := json.Unmarshal(lines[len(lines)-1], &newest); err != nil {
		return memberTranscriptSummary{}
	}
	steps := len(lines)
	if newest.StepIndex+1 > steps {
		steps = newest.StepIndex + 1
	}

	if newest.Status == "ERROR" || newest.Type == "ERROR_MESSAGE" || (newest.ErrorMessage != nil && newest.ErrorMessage.Error.ShortError != "") {
		msg := strings.TrimSpace(newest.Content)
		if msg == "" && newest.Error != "" {
			msg = strings.TrimSpace(newest.Error)
		}
		if msg == "" && newest.ErrorMessage != nil {
			msg = newest.ErrorMessage.Error.ShortError
		}
		if len(msg) > 120 {
			msg = msg[:120] + "..."
		}
		return memberTranscriptSummary{status: "error", activity: msg, steps: steps}
	}

	// 1. Determine whether the member's turn has ended (turnEnded in main.mjs).
	idle := false
	var trailingGeneric []transcriptStep
	for i := len(lines) - 1; i >= 0; i-- {
		var st transcriptStep
		if err := json.Unmarshal(lines[i], &st); err != nil {
			break
		}
		if st.Type == "GENERIC" {
			trailingGeneric = append([]transcriptStep{st}, trailingGeneric...)
			continue
		}
		if st.Type == "PLANNER_RESPONSE" {
			if len(st.ToolCalls) == 0 && st.ToolCall == nil {
				idle = len(trailingGeneric) == 0 && (st.Status == "DONE" || st.Status == "")
			} else {
				waitIdx := -1
				for idx, tc := range st.ToolCalls {
					if tc.Name == "swarm_wait" {
						waitIdx = idx
						break
					}
				}
				if waitIdx >= 0 {
					if len(trailingGeneric) >= len(st.ToolCalls) && trailingGeneric[waitIdx].Status == "DONE" {
						idle = true
					} else if len(trailingGeneric) == 0 && len(st.ToolCalls) == 1 && st.Status == "DONE" {
						idle = true
					}
				}
			}
		}
		break
	}
	if !idle && newest.Type == "GENERIC" && newest.Status == "DONE" && strings.Contains(newest.Content, "Waiting for a message") {
		idle = true
	}

	// 2. Walk back to the newest planner step of the current turn for live activity.
	var activity, activityKind string
	for i := len(lines) - 1; i >= 0; i-- {
		var st transcriptStep
		if err := json.Unmarshal(lines[i], &st); err != nil {
			continue
		}
		if st.Type == "USER_INPUT" {
			break
		}
		if st.ToolCall != nil {
			if st.ToolCall.ToolSummary != "" {
				activity = oneLineSwarmLabel(st.ToolCall.ToolSummary, 96)
			} else {
				activity = describeSwarmToolCall(st.ToolCall.Name, nil)
			}
			activityKind = "tool"
			break
		}
		if st.Type != "PLANNER_RESPONSE" {
			if st.PlannerResponse != nil && st.PlannerResponse.Response != "" && activity == "" {
				activity = "Replying…"
				activityKind = "thinking"
				break
			}
			continue
		}
		hasWait := false
		for _, tc := range st.ToolCalls {
			if tc.Name == "swarm_wait" {
				hasWait = true
				break
			}
		}
		if hasWait {
			break
		}
		for j := len(st.ToolCalls) - 1; j >= 0; j-- {
			if lbl := describeSwarmToolCall(st.ToolCalls[j].Name, st.ToolCalls[j].Args); lbl != "" {
				activity = lbl
				activityKind = "tool"
				break
			}
		}
		if activity != "" {
			break
		}
		if h := extractThinkingHeading(st.Thinking); h != "" {
			activity = h
			activityKind = "thinking"
			break
		}
	}

	status := "working"
	if idle {
		status = "waiting"
	}
	return memberTranscriptSummary{
		status:       status,
		activity:     activity,
		activityKind: activityKind,
		steps:        steps,
	}
}

func readMemberTranscriptActivity(transcriptPath string) (statusOverride string, lastActivity string) {
	sum := readMemberTranscriptSummary(transcriptPath)
	return sum.status, sum.activity
}

// GetSwarmSnapshot returns full details of the swarm, checking ETag.
func GetSwarmSnapshot(cascadeID string, afterSeq int, clientETag string) (*SwarmSnapshot, bool, error) {
	return GetSwarmSnapshotPaged(cascadeID, afterSeq, 0, 0, clientETag)
}

// GetSwarmSnapshotPaged returns swarm state with optional tail-window pagination (limit),
// backward paging (beforeSeq), or forward incremental polling (afterSeq), plus ETag short-circuit.
func GetSwarmSnapshotPaged(cascadeID string, afterSeq, beforeSeq, limit int, clientETag string) (*SwarmSnapshot, bool, error) {
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

	brainBase := swarmBrainBaseFromSwarmDir(swarmDir)

	viewPath, viewMod := resolveSwarmViewPath(brainBase, swarmDir, rawMembers.Members)
	plan, planMod, planSize := readSwarmPlan(swarmDir)
	tDigest := computeMembersTranscriptDigest(brainBase, rawMembers.Members)

	currentETag := fmt.Sprintf(`"%d-%d-%d-%d-%d-%d-%d-%x-%d-%d-%d"`,
		fiMembers.ModTime().UnixNano(), fiMembers.Size(), boardMod, boardSize,
		viewMod, planMod, planSize, tDigest, afterSeq, beforeSeq, limit)
	if clientETag != "" && clientETag == currentETag {
		return nil, true, nil
	}

	// Read board.jsonl once: always aggregate member post stats & total count,
	// while bounding retained messages by afterSeq / beforeSeq / limit.
	messages := make([]SwarmBoardMessage, 0)
	latestSeq := 0
	totalMessages := 0
	hasMoreBefore := false
	postCounts := make(map[string]int)
	lastPostSeqs := make(map[string]int)
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
				totalMessages++
				if msg.Seq > latestSeq {
					latestSeq = msg.Seq
				}
				if msg.Agent != "" && !strings.HasPrefix(msg.Text, "[channel]") {
					postCounts[msg.Agent]++
					if msg.Seq > lastPostSeqs[msg.Agent] {
						lastPostSeqs[msg.Agent] = msg.Seq
					}
				}
				if beforeSeq > 0 {
					if msg.Seq < beforeSeq {
						messages = append(messages, msg)
					}
				} else if msg.Seq > afterSeq {
					messages = append(messages, msg)
				}
			}
		}
	}

	if limit > 0 && len(messages) > limit {
		if beforeSeq > 0 || afterSeq == 0 {
			// Keep newest `limit` messages in the window (tail window for initial load or beforeSeq page).
			messages = messages[len(messages)-limit:]
			hasMoreBefore = true
		} else {
			messages = messages[:limit]
		}
	}
	for i := range messages {
		found := mentionRegex.FindAllStringSubmatch(messages[i].Text, -1)
		mentions := make([]string, 0, len(found))
		for _, m := range found {
			if len(m) > 1 {
				mentions = append(mentions, m[1])
			}
		}
		messages[i].Mentions = mentions
	}
	oldestSeq := 0
	if len(messages) > 0 {
		oldestSeq = messages[0].Seq
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

		var trSum memberTranscriptSummary
		if !rm.Killed && rm.ConversationID != "" && cascadeIDRe.MatchString(rm.ConversationID) {
			tPath := filepath.Join(brainBase, rm.ConversationID, ".system_generated", "logs", "transcript.jsonl")
			trSum = readMemberTranscriptSummary(tPath)
			if trSum.status != "" {
				status = trSum.status
			}
		}
		if status == "working" {
			workingCount++
		}

		unread := latestSeq - rm.LastReadSeq
		if unread < 0 {
			unread = 0
		}

		memberModel := strings.TrimSpace(rm.Model)
		if rm.ConversationID != "" && cascadeIDRe.MatchString(rm.ConversationID) {
			if actual := strings.TrimSpace(resolveSubagentRuntimeModel(rm.ConversationID)); actual != "" {
				memberModel = actual
			}
		}

		members = append(members, SwarmMemberInfo{
			Name:           name,
			ConversationID: rm.ConversationID,
			Model:          memberModel,
			Role:           rm.Role,
			WakeMode:       rm.WakeMode,
			WaitSince:      string(rm.WaitSince),
			LastReadSeq:    rm.LastReadSeq,
			LastNudgedSeq:  rm.LastNudgedSeq,
			Killed:         rm.Killed,
			Status:         status,
			LastActivity:   trSum.activity,
			ActivityKind:   trSum.activityKind,
			Steps:          trSum.steps,
			Posts:          postCounts[name],
			LastPostSeq:    lastPostSeqs[name],
			Workspace:      strings.TrimSpace(rm.Workspace),
			WorkspaceMode:  strings.TrimSpace(rm.WorkspaceMode),
			RoleWorker:     isSwarmRoleWorker(name, rm.Role),
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
		if brief.Headline == "" && view.Goal != nil {
			brief.Headline = view.Goal.Title
		}
		if view.Phase != nil {
			brief.Phase = view.Phase.Label
		}
		brief.AttentionCount = len(view.Attention)
	}
	if brief.Headline == "" && plan != nil && plan.Goal != nil {
		if plan.Goal.Title != "" {
			brief.Headline = plan.Goal.Title
		} else {
			brief.Headline = plan.Goal.Line
		}
	}

	snap := &SwarmSnapshot{
		CascadeID:     cascadeID,
		CoordinatorID: coordID,
		Title:         title,
		ETag:          currentETag,
		Brief:         brief,
		Members:       members,
		Messages:      messages,
		TotalMessages: totalMessages,
		HasMoreBefore: hasMoreBefore,
		OldestSeq:     oldestSeq,
		View:          view,
		Plan:          plan,
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
	if !cascadeIDRe.MatchString(cascadeID) {
		writeJSONError(w, "invalid cascadeId", http.StatusBadRequest)
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
	beforeSeq := 0
	if s := r.URL.Query().Get("beforeSeq"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			beforeSeq = n
		}
	} else if s := r.URL.Query().Get("before_seq"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			beforeSeq = n
		}
	}
	limit := 0
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			if n > 500 {
				n = 500
			}
			limit = n
		}
	}
	clientETag := r.Header.Get("If-None-Match")

	snap, notModified, err := GetSwarmSnapshotPaged(cascadeID, afterSeq, beforeSeq, limit, clientETag)
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
	if err := json.NewDecoder(io.LimitReader(r.Body, 64*1024)).Decode(&req); err != nil {
		writeJSONError(w, "invalid request: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.CascadeID == "" {
		req.CascadeID = req.CascadeIDAlt
	}
	if req.CascadeID != "" && !cascadeIDRe.MatchString(req.CascadeID) {
		writeJSONError(w, "invalid cascadeId", http.StatusBadRequest)
		return
	}
	if req.CoordinatorID != "" && !cascadeIDRe.MatchString(req.CoordinatorID) {
		writeJSONError(w, "invalid coordinatorId", http.StatusBadRequest)
		return
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
	if !cascadeIDRe.MatchString(targetConv) {
		writeJSONError(w, "invalid target conversation", http.StatusBadRequest)
		return
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
	if err := json.NewDecoder(io.LimitReader(r.Body, 64*1024)).Decode(&req); err != nil {
		writeJSONError(w, "invalid request: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.CascadeID == "" {
		req.CascadeID = req.CascadeIDAlt
	}
	if req.CascadeID != "" && !cascadeIDRe.MatchString(req.CascadeID) {
		writeJSONError(w, "invalid cascadeId", http.StatusBadRequest)
		return
	}
	if req.CoordinatorID != "" && !cascadeIDRe.MatchString(req.CoordinatorID) {
		writeJSONError(w, "invalid coordinatorId", http.StatusBadRequest)
		return
	}
	worker := strings.TrimSpace(req.WorkerName)
	if worker == "" {
		worker = strings.TrimSpace(req.Member)
	}
	if worker == "" {
		writeJSONError(w, "workerName cannot be empty", http.StatusBadRequest)
		return
	}
	if !swarmWorkerNameRe.MatchString(worker) {
		writeJSONError(w, "invalid workerName", http.StatusBadRequest)
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
	if !cascadeIDRe.MatchString(targetConv) {
		writeJSONError(w, "invalid target conversation", http.StatusBadRequest)
		return
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
