package hub

import (
	"compress/flate"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	hubEventsBeatInterval  = 5 * time.Second
	hubEventsStateInterval = 1 * time.Second
	hubEventsPingInterval  = 20 * time.Second
	hubEventsReadTimeout   = 45 * time.Second
	hubEventsWriteTimeout  = 5 * time.Second
)

type hubEventsFrame struct {
	Type      string `json:"type"`
	Rev       uint64 `json:"rev"`
	TS        int64  `json:"ts,omitempty"`
	Desktop   string `json:"desktop,omitempty"`
	Unchanged bool   `json:"unchanged,omitempty"`
}

type hubEventSub struct {
	wake chan struct{}
}

// hubEventsBroadcaster fans out unified cluster revision updates to Hub /gateway/events clients.
type hubEventsBroadcaster struct {
	mu   sync.Mutex
	rev  uint64
	subs map[*hubEventSub]struct{}
}

func (b *hubEventsBroadcaster) currentRev() uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.rev
}

func (b *hubEventsBroadcaster) bump() uint64 {
	b.mu.Lock()
	b.rev++
	rev := b.rev
	for s := range b.subs {
		select {
		case s.wake <- struct{}{}:
		default:
		}
	}
	b.mu.Unlock()
	return rev
}

func (b *hubEventsBroadcaster) subscribe() (*hubEventSub, func()) {
	s := &hubEventSub{wake: make(chan struct{}, 1)}
	b.mu.Lock()
	if b.subs == nil {
		b.subs = make(map[*hubEventSub]struct{})
	}
	b.subs[s] = struct{}{}
	b.mu.Unlock()
	return s, func() {
		b.mu.Lock()
		delete(b.subs, s)
		b.mu.Unlock()
	}
}

// BumpEvents increments the Hub's unified conversation-list revision and notifies all connected clients.
func (h *Hub) BumpEvents() {
	h.events.bump()
}

// desktopState returns "ok" whenever either the local LS (if non-headless) or at least one remote node LS is healthy.
func (h *Hub) desktopState() string {
	if h.localLSHealthy() {
		return "ok"
	}
	h.stateMu.RLock()
	defer h.stateMu.RUnlock()
	for _, st := range h.nodeStates {
		if st.Enabled && st.Online && st.LSHealthy {
			return "ok"
		}
	}
	return "down"
}

// HandleEvents serves the unified Hub GET /gateway/events WebSocket stream.
// Wire format is 100% identical to single-host proxy.HandleEvents so mobile/web clients work unchanged.
func (h *Hub) HandleEvents(w http.ResponseWriter, r *http.Request) {
	hasSinceRev := r.URL.Query().Has("sinceRev")
	sinceRev, sinceRevErr := strconv.ParseUint(r.URL.Query().Get("sinceRev"), 10, 64)

	conn, err := hubWSUpgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Warn("[HubEvents] WS upgrade failed", "err", err)
		return
	}
	_ = conn.SetCompressionLevel(flate.BestSpeed)
	defer conn.Close()
	defer h.trackWS(conn)()

	sub, unsub := h.events.subscribe()
	defer unsub()

	var writeMu sync.Mutex
	writeFrame := func(f hubEventsFrame) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		_ = conn.SetWriteDeadline(time.Now().Add(hubEventsWriteTimeout))
		return conn.WriteJSON(f)
	}

	closed := make(chan struct{})
	conn.SetReadLimit(64 * 1024)
	_ = conn.SetReadDeadline(time.Now().Add(hubEventsReadTimeout))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(hubEventsReadTimeout))
	})
	go func() {
		defer close(closed)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
			_ = conn.SetReadDeadline(time.Now().Add(hubEventsReadTimeout))
		}
	}()

	curRev := h.events.currentRev()
	helloUnchanged := hasSinceRev && sinceRevErr == nil && curRev == sinceRev
	desktop := h.desktopState()
	sendBeat := func(typ string, unchanged bool) error {
		return writeFrame(hubEventsFrame{
			Type:      typ,
			Rev:       h.events.currentRev(),
			TS:        time.Now().UnixMilli(),
			Desktop:   desktop,
			Unchanged: unchanged,
		})
	}
	if err := sendBeat("hello", helloUnchanged); err != nil {
		return
	}

	beatTick := time.NewTicker(hubEventsBeatInterval)
	defer beatTick.Stop()
	stateTick := time.NewTicker(hubEventsStateInterval)
	defer stateTick.Stop()
	pingTick := time.NewTicker(hubEventsPingInterval)
	defer pingTick.Stop()

	for {
		select {
		case <-closed:
			return
		case <-sub.wake:
			if err := writeFrame(hubEventsFrame{Type: "changed", Rev: h.events.currentRev()}); err != nil {
				return
			}
		case <-beatTick.C:
			desktop = h.desktopState()
			if err := sendBeat("beat", false); err != nil {
				return
			}
		case <-stateTick.C:
			if now := h.desktopState(); now != desktop {
				desktop = now
				if err := sendBeat("beat", false); err != nil {
					return
				}
			}
		case <-pingTick.C:
			writeMu.Lock()
			_ = conn.SetWriteDeadline(time.Now().Add(hubEventsWriteTimeout))
			err := conn.WriteMessage(websocket.PingMessage, nil)
			writeMu.Unlock()
			if err != nil {
				return
			}
		}
	}
}

// ensureRemoteEventWatcher ensures a background WS subscriber is listening to a remote node's /gateway/events.
func (h *Hub) ensureRemoteEventWatcher(node NodeConfig) {
	if !node.Enabled || node.BaseURL == "" {
		return
	}
	h.watchMu.Lock()
	if h.nodeWatchers == nil {
		h.nodeWatchers = make(map[string]nodeWatcherEntry)
	}
	if existing, ok := h.nodeWatchers[node.ID]; ok && existing.baseURL == node.BaseURL && existing.token == node.Token {
		h.watchMu.Unlock()
		return
	} else if ok && existing.cancel != nil {
		existing.cancel()
	}

	ctx, cancel := context.WithCancel(h.rootCtx)
	h.nodeWatchers[node.ID] = nodeWatcherEntry{
		baseURL: node.BaseURL,
		token:   node.Token,
		cancel:  cancel,
	}
	h.watchMu.Unlock()

	go h.runRemoteEventWatcher(ctx, node)
}

func (h *Hub) stopRemoteEventWatcher(nodeID string) {
	h.watchMu.Lock()
	if existing, ok := h.nodeWatchers[nodeID]; ok {
		if existing.cancel != nil {
			existing.cancel()
		}
		delete(h.nodeWatchers, nodeID)
	}
	h.watchMu.Unlock()
}

func (h *Hub) runRemoteEventWatcher(ctx context.Context, node NodeConfig) {
	backoff := 1 * time.Second
	const maxBackoff = 15 * time.Second

	for {
		if ctx.Err() != nil {
			return
		}
		wsURL, err := buildNodeWSURL(node.BaseURL, "/gateway/events", "")
		if err != nil {
			return
		}
		header := make(http.Header)
		header.Set(HeaderHubForwarded, "1")
		if node.Token != "" {
			header.Set(HeaderHubToken, node.Token)
			header.Set("Authorization", "Bearer "+node.Token)
		}

		dialer := websocket.Dialer{
			Proxy:            nil,
			HandshakeTimeout: 3 * time.Second,
		}
		conn, resp, err := dialer.DialContext(ctx, wsURL, header)
		if err != nil {
			if resp != nil && resp.Body != nil {
				_ = resp.Body.Close()
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}

		backoff = 1 * time.Second
		doneCh := make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
				_ = conn.Close()
			case <-doneCh:
			}
		}()

		var lastRev uint64
		hasRev := false
		for {
			_ = conn.SetReadDeadline(time.Now().Add(30 * time.Second))
			_, msg, rErr := conn.ReadMessage()
			if rErr != nil {
				break
			}
			var frame hubEventsFrame
			if json.Unmarshal(msg, &frame) != nil {
				continue
			}
			switch frame.Type {
			case "hello":
				if hasRev && frame.Rev != lastRev {
					h.BumpEvents()
				}
				lastRev = frame.Rev
				hasRev = true
			case "changed":
				lastRev = frame.Rev
				hasRev = true
				h.BumpEvents()
			case "beat":
				if hasRev && frame.Rev != lastRev {
					lastRev = frame.Rev
					h.BumpEvents()
				}
				if frame.Desktop != "" {
					h.updateNodeDesktopState(node.ID, frame.Desktop == "ok")
				}
			}
		}
		close(doneCh)
		_ = conn.Close()

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
	}
}
