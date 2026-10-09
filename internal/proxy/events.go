package proxy

import (
	"context"
	"encoding/json"
	"hash/fnv"
	"log/slog"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Conversation-list change events (GET /gateway/events, WebSocket).
//
// Clients used to poll GetAllCascadeTrajectories every 4-10s to notice status changes. This channel
// only tells them *when* to refetch: frames carry a revision number, never list data, so the list
// endpoint (filtering, title enrichment, tombstones) stays the single source of truth.
//
//	server -> client: {"type":"hello","rev":N}    on connect (client should fetch once)
//	                  {"type":"changed","rev":N}  the list changed since the last frame
//
// Change detection runs only while at least one client is subscribed, so an idle gateway does no
// extra work, and all subscribers share one detector.
// eventsPollInterval and eventsNudgeSpacing are variables only so tests can shorten them.
var (
	eventsPollInterval = 1500 * time.Millisecond
	// eventsNudgeSpacing bounds how often mutating RPCs (the desktop UI fires bursts of them) can force a re-check.
	eventsNudgeSpacing = 400 * time.Millisecond
)

const (
	eventsPingInterval = 20 * time.Second // below typical tunnel/proxy idle timeouts
	eventsReadTimeout  = 45 * time.Second
	eventsWriteTimeout = 5 * time.Second
)

type eventsFrame struct {
	Type string `json:"type"`
	Rev  uint64 `json:"rev"`
}

type eventSub struct {
	wake chan struct{} // capacity 1: bursts of changes coalesce into one frame
}

// eventsHub fans change notifications out to subscribers. The zero value is ready to use.
type eventsHub struct {
	mu     sync.Mutex
	subs   map[*eventSub]struct{}
	rev    uint64
	sig    uint64
	hasSig bool
	cancel context.CancelFunc
	nudge  chan struct{}
}

func (h *eventsHub) currentRev() uint64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.rev
}

// subscribe registers a subscriber; the first one starts the detector via start.
func (h *eventsHub) subscribe(start func(ctx context.Context, nudge <-chan struct{})) (*eventSub, func()) {
	s := &eventSub{wake: make(chan struct{}, 1)}
	h.mu.Lock()
	if h.subs == nil {
		h.subs = make(map[*eventSub]struct{})
	}
	h.subs[s] = struct{}{}
	if len(h.subs) == 1 {
		ctx, cancel := context.WithCancel(context.Background())
		h.cancel = cancel
		h.nudge = make(chan struct{}, 1)
		h.hasSig = false // re-baseline: do not report changes that happened while nobody listened
		go start(ctx, h.nudge)
	}
	h.mu.Unlock()

	return s, func() {
		h.mu.Lock()
		delete(h.subs, s)
		if len(h.subs) == 0 && h.cancel != nil {
			h.cancel()
			h.cancel = nil
			h.nudge = nil
		}
		h.mu.Unlock()
	}
}

// observe records a new list signature and notifies subscribers when it differs from the last one.
// The first observation after the detector starts only sets the baseline.
func (h *eventsHub) observe(sig uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.hasSig {
		h.sig, h.hasSig = sig, true
		return
	}
	if sig == h.sig {
		return
	}
	h.sig = sig
	h.rev++
	for s := range h.subs {
		select {
		case s.wake <- struct{}{}:
		default:
		}
	}
}

// requestCheck asks a running detector to re-check immediately (e.g. right after a mutating RPC).
func (h *eventsHub) requestCheck() {
	h.mu.Lock()
	n := h.nudge
	h.mu.Unlock()
	if n == nil {
		return
	}
	select {
	case n <- struct{}{}:
	default:
	}
}

// listSignature hashes the visible-relevant state of every non-deleted cascade in a raw
// GetAllCascadeTrajectories body. Any change to an entry (status, step count, timestamps,
// title, ...) changes the signature; over-notifying is harmless, missing a change is not.
func listSignature(raw []byte) (uint64, bool) {
	var env struct {
		TrajectorySummaries map[string]json.RawMessage `json:"trajectorySummaries"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return 0, false
	}
	ids := make([]string, 0, len(env.TrajectorySummaries))
	for id := range env.TrajectorySummaries {
		if IsDeletedCascade(id) {
			continue
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	h := fnv.New64a()
	for _, id := range ids {
		_, _ = h.Write([]byte(id))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write(env.TrajectorySummaries[id])
		_, _ = h.Write([]byte{1})
	}
	return h.Sum64(), true
}

// cascadeSummarySignature hashes one cascade's entry in the shared GetAllCascadeTrajectories snapshot.
// ok is false when the snapshot is unavailable or does not list the cascade.
func (p *Proxy) cascadeSummarySignature(cascadeID string, port int, token string) (uint64, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	raw, err := p.fetchAllTrajectoriesRaw(ctx, port, token)
	if err != nil {
		return 0, false
	}
	var env struct {
		TrajectorySummaries map[string]json.RawMessage `json:"trajectorySummaries"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return 0, false
	}
	entry, ok := env.TrajectorySummaries[cascadeID]
	if !ok {
		return 0, false
	}
	h := fnv.New64a()
	_, _ = h.Write(entry)
	return h.Sum64(), true
}

// runEventsDetector polls the shared list snapshot while subscribers exist.
func (p *Proxy) runEventsDetector(ctx context.Context, nudge <-chan struct{}) {
	ticker := time.NewTicker(eventsPollInterval)
	defer ticker.Stop()
	check := func() {
		port, token := p.ActiveUpstream()
		if port == 0 {
			return
		}
		cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		// The snapshot's own TTL keeps this from adding upstream load beyond what list polls already cause.
		raw, err := p.fetchAllTrajectoriesRaw(cctx, port, token)
		if err != nil {
			return
		}
		if sig, ok := listSignature(raw); ok {
			p.events.observe(sig)
		}
	}
	lastCheck := time.Time{}
	run := func() {
		check()
		lastCheck = time.Now()
	}
	run()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		case <-nudge:
			if wait := eventsNudgeSpacing - time.Since(lastCheck); wait > 0 {
				select {
				case <-ctx.Done():
					return
				case <-time.After(wait):
				}
			}
			run()
		}
	}
}

// HandleEvents serves GET /gateway/events.
func (p *Proxy) HandleEvents(w http.ResponseWriter, r *http.Request) {
	sanitizeWebSocketHeaders(r)
	conn, err := streamUpgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Warn("[Events] WS upgrade failed", "err", err)
		return
	}
	defer conn.Close()
	defer p.trackWSConn(conn)()

	sub, unsubscribe := p.events.subscribe(p.runEventsDetector)
	defer unsubscribe()

	var writeMu sync.Mutex
	write := func(f eventsFrame) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		_ = conn.SetWriteDeadline(time.Now().Add(eventsWriteTimeout))
		return conn.WriteJSON(f)
	}

	closed := make(chan struct{})
	_ = conn.SetReadDeadline(time.Now().Add(eventsReadTimeout))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(eventsReadTimeout))
	})
	go func() {
		defer close(closed)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
			_ = conn.SetReadDeadline(time.Now().Add(eventsReadTimeout))
		}
	}()

	if err := write(eventsFrame{Type: "hello", Rev: p.events.currentRev()}); err != nil {
		return
	}

	ping := time.NewTicker(eventsPingInterval)
	defer ping.Stop()
	for {
		select {
		case <-closed:
			return
		case <-sub.wake:
			if err := write(eventsFrame{Type: "changed", Rev: p.events.currentRev()}); err != nil {
				return
			}
		case <-ping.C:
			writeMu.Lock()
			_ = conn.SetWriteDeadline(time.Now().Add(eventsWriteTimeout))
			err := conn.WriteMessage(websocket.PingMessage, nil)
			writeMu.Unlock()
			if err != nil {
				return
			}
		}
	}
}
