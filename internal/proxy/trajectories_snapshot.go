package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// allTrajectoriesTTL bounds how stale a shared GetAllCascadeTrajectories snapshot may be.
// The Watcher (1.5-3.5s), every client list poll (4-10s per device) and several one-off
// lookups all need the same list; sharing one upstream call per TTL window keeps the
// language_server (and our JSON decoding) from being hit once per consumer.
const allTrajectoriesTTL = time.Second

// maxAllTrajectoriesBody caps the upstream list response we are willing to buffer.
const maxAllTrajectoriesBody = 64 * 1024 * 1024

// allTrajectoriesParsed holds pre-decoded lightweight indices over a GetAllCascadeTrajectories
// response so stream change detection, subagent live lookups, and events list hashing never
// repeatedly json.Unmarshal megabytes of raw list JSON on hot paths.
type allTrajectoriesParsed struct {
	sortedIDs []string
	sigByID   map[string]uint64
	liveByID  map[string]subagentLive
}

func parseSingleSummaryEntry(entry json.RawMessage) (uint64, subagentLive, bool) {
	h := fnv.New64a()
	_, _ = h.Write(entry)
	sig := h.Sum64()

	var s struct {
		Status             string `json:"status"`
		StepCount          int    `json:"stepCount"`
		Summary            string `json:"summary"`
		TrajectoryMetadata struct {
			ParentConversationID string `json:"parentConversationId"`
		} `json:"trajectoryMetadata"`
	}
	if json.Unmarshal(entry, &s) == nil {
		return sig, subagentLive{
			Status:    s.Status,
			StepCount: s.StepCount,
			Title:     s.Summary,
			Parent:    strings.TrimSpace(s.TrajectoryMetadata.ParentConversationID),
		}, true
	}
	return sig, subagentLive{}, false
}

func parseAllTrajectoriesWithEntries(raw []byte) (map[string]json.RawMessage, *allTrajectoriesParsed) {
	var env struct {
		TrajectorySummaries map[string]json.RawMessage `json:"trajectorySummaries"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, nil
	}
	entries := env.TrajectorySummaries
	if entries == nil {
		entries = make(map[string]json.RawMessage)
	}
	return entries, buildTrajectoriesIndexFromMap(entries)
}

func parseAllTrajectoriesIndex(raw []byte) *allTrajectoriesParsed {
	_, parsed := parseAllTrajectoriesWithEntries(raw)
	return parsed
}

// cloneWithDelta produces a new immutable *allTrajectoriesParsed by applying O(|delta|) pre-parsed
// updates and removals without re-hashing or re-unmarshaling unchanged entries.
func (idx *allTrajectoriesParsed) cloneWithDelta(
	deltaSigs map[string]uint64,
	deltaLive map[string]subagentLive,
	hasLive map[string]bool,
	removed []string,
) *allTrajectoriesParsed {
	if idx == nil {
		return &allTrajectoriesParsed{
			sortedIDs: []string{},
			sigByID:   make(map[string]uint64),
			liveByID:  make(map[string]subagentLive),
		}
	}
	newCap := len(idx.sigByID) + len(deltaSigs)
	sigByID := make(map[string]uint64, newCap)
	for k, v := range idx.sigByID {
		sigByID[k] = v
	}
	liveByID := make(map[string]subagentLive, len(idx.liveByID)+len(deltaLive))
	for k, v := range idx.liveByID {
		liveByID[k] = v
	}

	idsChanged := false
	for _, id := range removed {
		if _, existed := sigByID[id]; existed {
			delete(sigByID, id)
			delete(liveByID, id)
			idsChanged = true
		}
	}
	for id, sig := range deltaSigs {
		if _, existed := sigByID[id]; !existed {
			idsChanged = true
		}
		sigByID[id] = sig
		if hasLive[id] {
			liveByID[id] = deltaLive[id]
		} else {
			delete(liveByID, id)
		}
	}

	var sortedIDs []string
	if !idsChanged {
		sortedIDs = idx.sortedIDs
	} else {
		sortedIDs = make([]string, 0, len(sigByID))
		for id := range sigByID {
			sortedIDs = append(sortedIDs, id)
		}
		sort.Strings(sortedIDs)
	}
	return &allTrajectoriesParsed{
		sortedIDs: sortedIDs,
		sigByID:   sigByID,
		liveByID:  liveByID,
	}
}

func marshalTrajectorySummariesMap(entries map[string]json.RawMessage, sortedIDs []string) []byte {
	total := 32
	for id, raw := range entries {
		total += len(id) + len(raw) + 6
	}
	var buf bytes.Buffer
	buf.Grow(total)
	buf.WriteString(`{"trajectorySummaries":{`)
	first := true
	if len(sortedIDs) > 0 {
		for _, id := range sortedIDs {
			raw, ok := entries[id]
			if !ok || len(raw) == 0 {
				continue
			}
			if !first {
				buf.WriteByte(',')
			}
			first = false
			buf.WriteString(strconv.Quote(id))
			buf.WriteByte(':')
			buf.Write(raw)
		}
	} else {
		for id, raw := range entries {
			if len(raw) == 0 {
				continue
			}
			if !first {
				buf.WriteByte(',')
			}
			first = false
			buf.WriteString(strconv.Quote(id))
			buf.WriteByte(':')
			buf.Write(raw)
		}
	}
	buf.WriteString(`}}`)
	out := make([]byte, buf.Len())
	copy(out, buf.Bytes())
	return out
}

func (idx *allTrajectoriesParsed) computeListSignature() (uint64, bool) {
	if idx == nil {
		return 0, false
	}
	var buf [8]byte
	h := fnv.New64a()
	for _, id := range idx.sortedIDs {
		if IsDeletedCascade(id) {
			continue
		}
		_, _ = h.Write([]byte(id))
		_, _ = h.Write([]byte{0})
		sig := idx.sigByID[id]
		buf[0] = byte(sig >> 56)
		buf[1] = byte(sig >> 48)
		buf[2] = byte(sig >> 40)
		buf[3] = byte(sig >> 32)
		buf[4] = byte(sig >> 24)
		buf[5] = byte(sig >> 16)
		buf[6] = byte(sig >> 8)
		buf[7] = byte(sig)
		_, _ = h.Write(buf[:])
		_, _ = h.Write([]byte{1})
	}
	return h.Sum64(), true
}

type allTrajectoriesFlight struct {
	done   chan struct{}
	body   []byte
	parsed *allTrajectoriesParsed
	err    error
}

// allTrajectoriesSnapshot caches the raw (already gunzipped) GetAllCascadeTrajectories
// response body for the active upstream, with single-flight de-duplication.
// The zero value is ready to use. Cached bytes are shared and must be treated as read-only.
type allTrajectoriesSnapshot struct {
	mu        sync.Mutex
	port      int
	token     string
	fetchedAt time.Time
	entries   map[string]json.RawMessage
	body      []byte // lazily encoded when nil and entries != nil
	parsed    *allTrajectoriesParsed
	flight    *allTrajectoriesFlight
	gen       uint64 // bumped by invalidate so an in-flight result fetched before a mutation is not cached
}

// fetchAllTrajectoriesRaw returns the raw `{}`-request GetAllCascadeTrajectories body,
// served from a shared snapshot no older than allTrajectoriesTTL.
func (p *Proxy) fetchAllTrajectoriesRaw(ctx context.Context, port int, token string) ([]byte, error) {
	return p.fetchAllTrajectoriesRawMaxAge(ctx, port, token, allTrajectoriesTTL)
}

func (p *Proxy) fetchAllTrajectoriesIndex(ctx context.Context, port int, token string) (*allTrajectoriesParsed, error) {
	_, idx, err := p.fetchAllTrajectoriesMaxAgeOpts(ctx, port, token, allTrajectoriesTTL, false)
	return idx, err
}

// fetchAllTrajectoriesRawMaxAge is fetchAllTrajectoriesRaw with a caller-chosen freshness bound.
// maxAge <= 0 forces a refresh (joining one already in flight) and re-populates the shared snapshot,
// so a periodic poller such as the Watcher keeps it warm for everyone else.
func (p *Proxy) fetchAllTrajectoriesRawMaxAge(ctx context.Context, port int, token string, maxAge time.Duration) ([]byte, error) {
	body, _, err := p.fetchAllTrajectoriesMaxAgeOpts(ctx, port, token, maxAge, true)
	return body, err
}

func (p *Proxy) fetchAllTrajectoriesMaxAge(ctx context.Context, port int, token string, maxAge time.Duration) ([]byte, *allTrajectoriesParsed, error) {
	return p.fetchAllTrajectoriesMaxAgeOpts(ctx, port, token, maxAge, true)
}

func (p *Proxy) fetchAllTrajectoriesMaxAgeOpts(ctx context.Context, port int, token string, maxAge time.Duration, needBody bool) ([]byte, *allTrajectoriesParsed, error) {
	if port == 0 {
		return nil, nil, fmt.Errorf("antigravity upstream not connected")
	}
	s := &p.allTrajectories

	s.mu.Lock()
	if (s.body != nil || s.entries != nil) && s.port == port && s.token == token && time.Since(s.fetchedAt) < maxAge {
		if needBody && s.body == nil && s.entries != nil {
			var sorted []string
			if s.parsed != nil {
				sorted = s.parsed.sortedIDs
			}
			s.body = marshalTrajectorySummariesMap(s.entries, sorted)
		}
		b, idx := s.body, s.parsed
		s.mu.Unlock()
		return b, idx, nil
	}
	f := s.flight
	if f == nil {
		f = &allTrajectoriesFlight{done: make(chan struct{})}
		s.flight = f
		gen := s.gen
		go func() {
			// Detached from any single caller's context so one cancelled waiter cannot fail the others;
			// mediumClient still enforces its own timeout.
			body, err := p.fetchAllTrajectoriesUpstream(context.Background(), port, token)
			var (
				entries map[string]json.RawMessage
				parsed  *allTrajectoriesParsed
			)
			if err == nil {
				entries, parsed = parseAllTrajectoriesWithEntries(body)
			}
			s.mu.Lock()
			if err == nil {
				if s.gen == gen {
					s.port, s.token, s.entries, s.body, s.parsed, s.fetchedAt = port, token, entries, body, parsed, time.Now()
				} else if (s.body != nil || s.entries != nil) && s.port == port && s.token == token {
					if s.body == nil && s.entries != nil {
						var sorted []string
						if s.parsed != nil {
							sorted = s.parsed.sortedIDs
						}
						s.body = marshalTrajectorySummariesMap(s.entries, sorted)
					}
					body, parsed = s.body, s.parsed
				}
			}
			f.body, f.parsed, f.err = body, parsed, err
			s.flight = nil
			s.mu.Unlock()
			close(f.done)
		}()
	}
	s.mu.Unlock()

	select {
	case <-f.done:
		return f.body, f.parsed, f.err
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	}
}

// invalidateAllTrajectories drops the cached snapshot; call after RPCs that mutate the list.
func (p *Proxy) invalidateAllTrajectories() {
	s := &p.allTrajectories
	s.mu.Lock()
	s.entries = nil
	s.body = nil
	s.parsed = nil
	s.gen++
	s.mu.Unlock()
	p.events.requestCheck()
}

func (p *Proxy) fetchAllTrajectoriesUpstream(ctx context.Context, port int, token string) ([]byte, error) {
	url := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/GetAllCascadeTrajectories", port)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader("{}"))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connect-Protocol-Version", "1")
	if token != "" {
		req.Header.Set("x-codeium-csrf-token", token)
	}

	resp, err := p.mediumClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream status %d", resp.StatusCode)
	}

	var reader io.Reader = resp.Body
	if strings.EqualFold(resp.Header.Get("Content-Encoding"), "gzip") {
		gz, err := GetGzipReader(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("gzip reader failed: %w", err)
		}
		defer PutGzipReader(gz)
		reader = gz
	}

	buf := GetHugeBuffer()
	defer PutHugeBuffer(buf)
	if _, err := buf.ReadFrom(io.LimitReader(reader, maxAllTrajectoriesBody)); err != nil {
		return nil, err
	}
	out := make([]byte, buf.Len())
	copy(out, buf.Bytes())
	return out, nil
}
