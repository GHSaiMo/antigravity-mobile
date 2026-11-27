package proxy

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	agentStateStreamLinger = 2 * time.Second
	reactiveRetryBaseDelay = 1500 * time.Millisecond
	reactiveRetryMaxDelay  = 20 * time.Second
)

type liveContextUsageSnapshot struct {
	usedTokens int64
	maxTokens  int64
	updatedAt  time.Time
}

type agentStateStreamSlot struct {
	cascadeID   string
	refs        int
	live        bool
	cancel      context.CancelFunc
	lingerTimer *time.Timer
	usage       *liveContextUsageSnapshot
	port        int
}

// reactiveAgentStateHub maintains at most one long-lived upstream StreamAgentStateUpdates
// HTTP/2 Connect stream per active cascadeId, shared across all attached WebSocket clients.
// Zero value is ready to use.
type reactiveAgentStateHub struct {
	mu    sync.Mutex
	slots map[string]*agentStateStreamSlot
}

// Acquire increments the subscriber count for cascadeID and ensures a shared upstream
// StreamAgentStateUpdates stream is running. Call the returned release func on disconnect.
func (h *reactiveAgentStateHub) Acquire(p *Proxy, cascadeID string) func() {
	if cascadeID == "" || p == nil {
		return func() {}
	}
	h.mu.Lock()
	if h.slots == nil {
		h.slots = make(map[string]*agentStateStreamSlot)
	}
	slot, ok := h.slots[cascadeID]
	if !ok {
		slot = &agentStateStreamSlot{cascadeID: cascadeID}
		h.slots[cascadeID] = slot
	}
	slot.refs++
	if slot.lingerTimer != nil {
		slot.lingerTimer.Stop()
		slot.lingerTimer = nil
	}
	port, _ := p.ActiveUpstream()
	if slot.cancel == nil && port > 0 {
		h.startSlotLocked(p, slot, port)
	}
	h.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			h.release(cascadeID)
		})
	}
}

func (h *reactiveAgentStateHub) startSlotLocked(p *Proxy, slot *agentStateStreamSlot, port int) {
	ctx, cancel := context.WithCancel(context.Background())
	slot.cancel = cancel
	slot.port = port
	slot.live = false
	cid := slot.cascadeID
	go h.runSlot(ctx, p, slot, cid)
}

func (h *reactiveAgentStateHub) release(cascadeID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	slot, ok := h.slots[cascadeID]
	if !ok {
		return
	}
	if slot.refs > 0 {
		slot.refs--
	}
	if slot.refs > 0 {
		return
	}
	if slot.lingerTimer != nil {
		slot.lingerTimer.Stop()
		slot.lingerTimer = nil
	}
	if !slot.live || agentStateStreamLinger <= 0 {
		if slot.cancel != nil {
			slot.cancel()
			slot.cancel = nil
		}
		slot.live = false
		delete(h.slots, cascadeID)
		return
	}
	slot.lingerTimer = time.AfterFunc(agentStateStreamLinger, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		cur, exists := h.slots[cascadeID]
		if !exists || cur != slot || cur.refs > 0 {
			return
		}
		if cur.cancel != nil {
			cur.cancel()
			cur.cancel = nil
		}
		cur.live = false
		delete(h.slots, cascadeID)
	})
}

// StopAll immediately cancels all active upstream StreamAgentStateUpdates streams.
func (h *reactiveAgentStateHub) StopAll() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for id, slot := range h.slots {
		if slot.lingerTimer != nil {
			slot.lingerTimer.Stop()
			slot.lingerTimer = nil
		}
		if slot.cancel != nil {
			slot.cancel()
			slot.cancel = nil
		}
		slot.live = false
		if slot.refs == 0 {
			delete(h.slots, id)
		}
	}
}

// OnUpstreamChanged restarts active streams when language_server port changes.
func (h *reactiveAgentStateHub) OnUpstreamChanged(p *Proxy, newPort int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for id, slot := range h.slots {
		if slot.port == newPort && slot.cancel != nil {
			continue
		}
		if slot.cancel != nil {
			slot.cancel()
			slot.cancel = nil
		}
		slot.live = false
		if slot.refs > 0 && newPort > 0 {
			h.startSlotLocked(p, slot, newPort)
		} else if slot.refs == 0 {
			if slot.lingerTimer != nil {
				slot.lingerTimer.Stop()
			}
			delete(h.slots, id)
		}
	}
}

// IsLive reports whether an active Connect stream is currently established for cascadeID.
func (h *reactiveAgentStateHub) IsLive(cascadeID string) bool {
	if cascadeID == "" {
		return false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if slot, ok := h.slots[cascadeID]; ok {
		return slot.live
	}
	return false
}

// LiveContextUsage returns the latest contextWindowUsage pushed by StreamAgentStateUpdates, if fresh.
func (h *reactiveAgentStateHub) LiveContextUsage(cascadeID string, maxAge time.Duration) (usedTokens int64, maxTokens int64, ok bool) {
	if cascadeID == "" {
		return 0, 0, false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	slot, exists := h.slots[cascadeID]
	if !exists || slot.usage == nil {
		return 0, 0, false
	}
	if maxAge > 0 && time.Since(slot.usage.updatedAt) > maxAge {
		return 0, 0, false
	}
	return slot.usage.usedTokens, slot.usage.maxTokens, slot.usage.usedTokens > 0 || slot.usage.maxTokens > 0
}

func (h *reactiveAgentStateHub) setSlotLive(slot *agentStateStreamSlot, live bool) {
	h.mu.Lock()
	if cur, ok := h.slots[slot.cascadeID]; ok && cur == slot {
		slot.live = live
	}
	h.mu.Unlock()
}

func (h *reactiveAgentStateHub) setSlotUsage(slot *agentStateStreamSlot, used, max int64) {
	if used <= 0 && max <= 0 {
		return
	}
	h.mu.Lock()
	if cur, ok := h.slots[slot.cascadeID]; ok && cur == slot {
		slot.usage = &liveContextUsageSnapshot{
			usedTokens: used,
			maxTokens:  max,
			updatedAt:  time.Now(),
		}
	}
	h.mu.Unlock()
}

func (h *reactiveAgentStateHub) runSlot(ctx context.Context, p *Proxy, slot *agentStateStreamSlot, cascadeID string) {
	defer h.setSlotLive(slot, false)

	backoff := reactiveRetryBaseDelay
	for {
		if ctx.Err() != nil {
			return
		}
		port, token := p.ActiveUpstream()
		if port <= 0 {
			if !sleepContext(ctx, backoff) {
				return
			}
			continue
		}

		framesRead, supported := h.streamOnce(ctx, p, slot, cascadeID, port, token)
		if ctx.Err() != nil {
			return
		}
		if !supported {
			// Non-Connect mock server or unsupported endpoint: back off quietly without spamming.
			if !sleepContext(ctx, reactiveRetryMaxDelay) {
				return
			}
			continue
		}
		if framesRead > 0 {
			backoff = reactiveRetryBaseDelay
		} else {
			backoff *= 2
			if backoff > reactiveRetryMaxDelay {
				backoff = reactiveRetryMaxDelay
			}
		}
		if !sleepContext(ctx, backoff) {
			return
		}
	}
}

func (h *reactiveAgentStateHub) streamOnce(
	ctx context.Context,
	p *Proxy,
	slot *agentStateStreamSlot,
	cascadeID string,
	port int,
	token string,
) (frames int, supported bool) {
	upstreamCascadeID := ResolveActiveCascadeID(cascadeID)
	if upstreamCascadeID == "" {
		upstreamCascadeID = cascadeID
	}
	reqPayload := map[string]interface{}{
		"conversationId":                      upstreamCascadeID,
		"subscriberId":                        fmt.Sprintf("mgy-live-%s-%d", shortCascadeID(upstreamCascadeID), time.Now().UnixNano()),
		"initialStepsPageBounds":              map[string]int{"startIndex": 0, "endIndexExclusive": 0},
		"initialGeneratorMetadatasPageBounds": map[string]int{"startIndex": 0, "endIndexExclusive": 0},
		"initialExecutorMetadatasPageBounds":  map[string]int{"startIndex": 0, "endIndexExclusive": 0},
	}

	buf := GetSmallBuffer()
	var header [5]byte
	buf.Write(header[:])
	if err := json.NewEncoder(buf).Encode(reqPayload); err != nil {
		PutSmallBuffer(buf)
		return 0, true
	}
	binary.BigEndian.PutUint32(buf.Bytes()[1:5], uint32(buf.Len()-5))
	reqBytes := append([]byte(nil), buf.Bytes()...)
	PutSmallBuffer(buf)

	url := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/StreamAgentStateUpdates", port)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBytes))
	if err != nil {
		return 0, true
	}
	req.Header.Set("Content-Type", "application/connect+json")
	req.Header.Set("Connect-Protocol-Version", "1")
	if token != "" {
		req.Header.Set("x-codeium-csrf-token", token)
	}

	client := p.streamingHTTPClient()
	if client == nil {
		return 0, false
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, true
	}
	defer resp.Body.Close()

	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	if resp.StatusCode != http.StatusOK || !strings.Contains(ct, "application/connect") {
		return 0, false
	}

	for {
		flag, body, err := readConnectEnvelope(resp.Body)
		if err != nil {
			return frames, frames > 0
		}
		if flag != 0 {
			// End-of-stream trailer frame (flag == 2); check if upstream returned an error JSON.
			if bytes.Contains(body, []byte(`"unimplemented"`)) || bytes.Contains(body, []byte(`"deprecated"`)) {
				return frames, false
			}
			return frames, frames > 0
		}

		var state upstreamAgentStateUpdate
		if err := json.Unmarshal(body, &state); err != nil {
			continue
		}
		frames++
		if frames == 1 {
			h.setSlotLive(slot, true)
		}

		// 1. Update live pending/queued messages cache directly from stream frame.
		if state.Update.PendingAgentMessagesUpdate != nil || len(state.Update.PendingAgentMessages) > 0 || frames == 1 {
			items := parseAgentStateQueuedMessages(&state)
			setLivePendingMessagesCache(cascadeID, items)
		}

		// 2. Capture live contextWindowUsage tokens if pushed by Antigravity 2.32+.
		if cwu := state.Update.ContextWindowUsage; cwu != nil {
			h.setSlotUsage(slot, int64(cwu.EstimatedTokensUsed), int64(cwu.MaxContextTokens))
		}

		// 3. On live delta frames (frames > 1), expire trajectory cache freshness and wake all WS stream listeners immediately.
		if frames > 1 {
			expireTrajectoryCacheFreshness(cascadeID)
			p.notifyStreamTouch(cascadeID)
		}
	}
}

// expireTrajectoryCacheFreshness marks a cached trajectory entry stale so the next
// fetchUpstreamTrajectoryWithMaxAge call refetches without breaking in-flight singleflight coalescing
// or invalidating unrelated cascades' in-flight fetches.
func expireTrajectoryCacheFreshness(cascadeID string) {
	if cascadeID == "" {
		return
	}
	rootID := ResolveVisibleRootCascadeID(cascadeID)
	activeID := ResolveActiveCascadeID(rootID)
	defaultTrajCache.trajCacheMu.Lock()
	if cached, ok := defaultTrajCache.trajCache[cascadeID]; ok {
		cached.fetchedAt = time.Time{}
	}
	if rootID != "" && rootID != cascadeID {
		if cached, ok := defaultTrajCache.trajCache[rootID]; ok {
			cached.fetchedAt = time.Time{}
		}
	}
	if activeID != "" && activeID != cascadeID {
		if cached, ok := defaultTrajCache.trajCache[activeID]; ok {
			cached.fetchedAt = time.Time{}
		}
	}
	defaultTrajCache.trajCacheMu.Unlock()
}

// liveStreamAuxSignature summarizes live context-window usage and queued messages pushed via
// StreamAgentStateUpdates so HandleCascadeStream can preserve lastRawSig across touchCh wakeups
// while still pushing immediately when only queued messages or token usage change.
func (p *Proxy) liveStreamAuxSignature(cascadeID string) string {
	if cascadeID == "" {
		return ""
	}
	used, maxTok, _ := p.agentStateHub.LiveContextUsage(cascadeID, 0)
	pendingCacheMu.RLock()
	pe, hasPending := pendingCache[cascadeID]
	var qCount int
	var firstID, lastID string
	if hasPending && pe != nil {
		qCount = len(pe.messages)
		if qCount > 0 {
			firstID = pe.messages[0].ID
			lastID = pe.messages[qCount-1].ID
		}
	}
	pendingCacheMu.RUnlock()
	if used == 0 && maxTok == 0 && qCount == 0 {
		return ""
	}
	return fmt.Sprintf("%d/%d:%d:%s:%s", used, maxTok, qCount, firstID, lastID)
}

func setLivePendingMessagesCache(cascadeID string, items []QueuedMessageItem) {
	if cascadeID == "" {
		return
	}
	now := time.Now()
	pendingCacheMu.Lock()
	if len(pendingCache) >= maxPendingCacheEntries {
		for k, v := range pendingCache {
			if now.Sub(v.fetchedAt) >= pendingTTL {
				delete(pendingCache, k)
			}
		}
	}
	pendingCache[cascadeID] = &pendingMessagesCacheEntry{
		fetchedAt: now,
		messages:  items,
	}
	pendingCacheMu.Unlock()
}

// ---------------------------------------------------------------------------
// Global Shared JetboxSubscribeToSummaries Reactive Hub
// ---------------------------------------------------------------------------

type summarySub struct {
	wake chan struct{}
}

// reactiveSummariesHub maintains a single upstream JetboxSubscribeToSummaries Connect stream
// while at least one consumer (eventsHub or notifier.Watcher) is subscribed.
// Incoming summary deltas patch Proxy.allTrajectories in-place and wake listeners in <5ms.
type reactiveSummariesHub struct {
	mu      sync.Mutex
	subs    map[*summarySub]struct{}
	live    bool
	port    int
	cancel  context.CancelFunc
	entries map[string]json.RawMessage
}

// SubscribeSummaryUpdates registers a listener for real-time summary updates via JetboxSubscribeToSummaries.
func (p *Proxy) SubscribeSummaryUpdates() (<-chan struct{}, func()) {
	return p.summariesHub.Acquire(p)
}

// IsReactiveSummariesLive reports whether JetboxSubscribeToSummaries is actively connected.
func (p *Proxy) IsReactiveSummariesLive() bool {
	return p.summariesHub.IsLive()
}

func (h *reactiveSummariesHub) IsLive() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.live
}

func (h *reactiveSummariesHub) Acquire(p *Proxy) (<-chan struct{}, func()) {
	sub := &summarySub{wake: make(chan struct{}, 1)}
	h.mu.Lock()
	if h.subs == nil {
		h.subs = make(map[*summarySub]struct{})
	}
	h.subs[sub] = struct{}{}
	port, _ := p.ActiveUpstream()
	if h.cancel == nil && len(h.subs) > 0 {
		h.startLocked(p, port)
	}
	h.mu.Unlock()

	var once sync.Once
	return sub.wake, func() {
		once.Do(func() {
			h.mu.Lock()
			delete(h.subs, sub)
			if len(h.subs) == 0 {
				if h.cancel != nil {
					h.cancel()
					h.cancel = nil
				}
				h.live = false
				h.entries = nil
			}
			h.mu.Unlock()
		})
	}
}

func (h *reactiveSummariesHub) startLocked(p *Proxy, port int) {
	ctx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel
	h.port = port
	h.live = false
	go h.run(ctx, p)
}

func (h *reactiveSummariesHub) StopAll() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.cancel != nil {
		h.cancel()
		h.cancel = nil
	}
	h.live = false
	h.entries = nil
}

func (h *reactiveSummariesHub) OnUpstreamChanged(p *Proxy, newPort int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.port == newPort && h.cancel != nil {
		return
	}
	if h.cancel != nil {
		h.cancel()
		h.cancel = nil
	}
	h.live = false
	h.entries = nil
	if len(h.subs) > 0 {
		h.startLocked(p, newPort)
	}
}

func (h *reactiveSummariesHub) setLive(live bool) {
	h.mu.Lock()
	h.live = live
	h.mu.Unlock()
}

func (h *reactiveSummariesHub) broadcast() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for sub := range h.subs {
		select {
		case sub.wake <- struct{}{}:
		default:
		}
	}
}

func (h *reactiveSummariesHub) run(ctx context.Context, p *Proxy) {
	defer h.setLive(false)

	backoff := reactiveRetryBaseDelay
	for {
		if ctx.Err() != nil {
			return
		}
		port, token := p.ActiveUpstream()
		if port <= 0 {
			if !sleepContext(ctx, backoff) {
				return
			}
			continue
		}

		framesRead, supported := h.streamSummariesOnce(ctx, p, port, token)
		if ctx.Err() != nil {
			return
		}
		if !supported {
			if !sleepContext(ctx, reactiveRetryMaxDelay) {
				return
			}
			continue
		}
		if framesRead > 0 {
			backoff = reactiveRetryBaseDelay
		} else {
			backoff *= 2
			if backoff > reactiveRetryMaxDelay {
				backoff = reactiveRetryMaxDelay
			}
		}
		if !sleepContext(ctx, backoff) {
			return
		}
	}
}

func (h *reactiveSummariesHub) streamSummariesOnce(ctx context.Context, p *Proxy, port int, token string) (frames int, supported bool) {
	reqBytes := encodeConnectEnvelope([]byte("{}"))
	url := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/JetboxSubscribeToSummaries", port)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBytes))
	if err != nil {
		return 0, true
	}
	req.Header.Set("Content-Type", "application/connect+json")
	req.Header.Set("Connect-Protocol-Version", "1")
	if token != "" {
		req.Header.Set("x-codeium-csrf-token", token)
	}

	client := p.streamingHTTPClient()
	if client == nil {
		return 0, false
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, true
	}
	defer resp.Body.Close()

	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	if resp.StatusCode != http.StatusOK || !strings.Contains(ct, "application/connect") {
		return 0, false
	}

	for {
		flag, body, err := readConnectEnvelope(resp.Body)
		if err != nil {
			return frames, frames > 0
		}
		if flag != 0 {
			if bytes.Contains(body, []byte(`"unimplemented"`)) || bytes.Contains(body, []byte(`"deprecated"`)) {
				return frames, false
			}
			return frames, frames > 0
		}

		var env struct {
			Updates map[string]json.RawMessage `json:"updates"`
		}
		if err := json.Unmarshal(body, &env); err != nil {
			continue
		}
		frames++
		if frames == 1 {
			h.setLive(true)
		}
		if len(env.Updates) == 0 {
			continue
		}

		p.applyReactiveSummaryUpdates(port, token, env.Updates, frames == 1)
		h.broadcast()
	}
}

// applyReactiveSummaryUpdates merges live JetboxSubscribeToSummaries deltas into p.allTrajectories
// in O(|delta|) JSON work (without unmarshaling/re-marshaling the entire conversation map on every frame)
// and marks s.body dirty for lazy serialization only when raw JSON bytes are requested.
func (p *Proxy) applyReactiveSummaryUpdates(port int, token string, updates map[string]json.RawMessage, isInitial bool) {
	// Pre-parse ONLY the |delta| entries outside any snapshot lock.
	deltaRaw := make(map[string]json.RawMessage, len(updates))
	deltaSigs := make(map[string]uint64, len(updates))
	deltaLive := make(map[string]subagentLive, len(updates))
	hasLive := make(map[string]bool, len(updates))
	var removed []string

	for id, raw := range updates {
		trimmed := bytes.TrimSpace(raw)
		if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
			removed = append(removed, id)
			continue
		}
		cp := make(json.RawMessage, len(trimmed))
		copy(cp, trimmed)
		deltaRaw[id] = cp
		sig, live, ok := parseSingleSummaryEntry(cp)
		deltaSigs[id] = sig
		if ok {
			deltaLive[id] = live
			hasLive[id] = true
		}
	}

	h := &p.summariesHub
	var initialSnapshot map[string]json.RawMessage
	h.mu.Lock()
	if h.entries == nil || isInitial {
		h.entries = make(map[string]json.RawMessage, len(deltaRaw))
	}
	for _, id := range removed {
		delete(h.entries, id)
	}
	for id, cp := range deltaRaw {
		h.entries[id] = cp
	}
	if isInitial && len(h.entries) > 0 {
		initialSnapshot = make(map[string]json.RawMessage, len(h.entries))
		for k, v := range h.entries {
			initialSnapshot[k] = v
		}
	}
	h.mu.Unlock()

	s := &p.allTrajectories
	s.mu.Lock()
	var observedSig uint64
	hasSig := false

	if (s.entries != nil || s.body != nil) && s.port == port && s.token == token && !isInitial {
		if s.entries == nil && s.body != nil {
			s.entries, s.parsed = parseAllTrajectoriesWithEntries(s.body)
		}
		if s.entries != nil {
			for _, id := range removed {
				delete(s.entries, id)
			}
			for id, cp := range deltaRaw {
				s.entries[id] = cp
			}
			s.parsed = s.parsed.cloneWithDelta(deltaSigs, deltaLive, hasLive, removed)
			s.body = nil // lazily serialized on next fetchAllTrajectoriesRaw
			s.fetchedAt = time.Now()
			s.gen++
			if sig, ok := s.parsed.computeListSignature(); ok {
				observedSig = sig
				hasSig = true
			}
		} else {
			s.fetchedAt = time.Time{}
			s.gen++
		}
	} else if isInitial && len(initialSnapshot) > 0 {
		parsed := buildTrajectoriesIndexFromMap(initialSnapshot)
		s.port = port
		s.token = token
		s.entries = initialSnapshot
		s.body = nil // lazily serialized on next fetchAllTrajectoriesRaw
		s.parsed = parsed
		s.fetchedAt = time.Now()
		s.gen++
		if sig, ok := parsed.computeListSignature(); ok {
			observedSig = sig
			hasSig = true
		}
	} else {
		// Force next snapshot read to incorporate fresh upstream state immediately.
		s.fetchedAt = time.Time{}
		s.gen++
	}
	s.mu.Unlock()

	if hasSig {
		p.events.observe(observedSig)
	} else {
		p.events.requestCheck()
	}
}

func buildTrajectoriesIndexFromMap(summaries map[string]json.RawMessage) *allTrajectoriesParsed {
	n := len(summaries)
	idx := &allTrajectoriesParsed{
		sortedIDs: make([]string, 0, n),
		sigByID:   make(map[string]uint64, n),
		liveByID:  make(map[string]subagentLive, n),
	}
	for id, entry := range summaries {
		idx.sortedIDs = append(idx.sortedIDs, id)
		sig, live, ok := parseSingleSummaryEntry(entry)
		idx.sigByID[id] = sig
		if ok {
			idx.liveByID[id] = live
		}
	}
	sort.Strings(idx.sortedIDs)
	return idx
}

func sleepContext(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
