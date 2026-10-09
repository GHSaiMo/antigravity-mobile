package proxy

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
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

type allTrajectoriesFlight struct {
	done chan struct{}
	body []byte
	err  error
}

// allTrajectoriesSnapshot caches the raw (already gunzipped) GetAllCascadeTrajectories
// response body for the active upstream, with single-flight de-duplication.
// The zero value is ready to use. Cached bytes are shared and must be treated as read-only.
type allTrajectoriesSnapshot struct {
	mu        sync.Mutex
	port      int
	token     string
	fetchedAt time.Time
	body      []byte
	flight    *allTrajectoriesFlight
	gen       uint64 // bumped by invalidate so an in-flight result fetched before a mutation is not cached
}

// fetchAllTrajectoriesRaw returns the raw `{}`-request GetAllCascadeTrajectories body,
// served from a shared snapshot no older than allTrajectoriesTTL.
func (p *Proxy) fetchAllTrajectoriesRaw(ctx context.Context, port int, token string) ([]byte, error) {
	return p.fetchAllTrajectoriesRawMaxAge(ctx, port, token, allTrajectoriesTTL)
}

// fetchAllTrajectoriesRawMaxAge is fetchAllTrajectoriesRaw with a caller-chosen freshness bound.
// maxAge <= 0 forces a refresh (joining one already in flight) and re-populates the shared snapshot,
// so a periodic poller such as the Watcher keeps it warm for everyone else.
func (p *Proxy) fetchAllTrajectoriesRawMaxAge(ctx context.Context, port int, token string, maxAge time.Duration) ([]byte, error) {
	if port == 0 {
		return nil, fmt.Errorf("antigravity upstream not connected")
	}
	s := &p.allTrajectories

	s.mu.Lock()
	if s.body != nil && s.port == port && s.token == token && time.Since(s.fetchedAt) < maxAge {
		b := s.body
		s.mu.Unlock()
		return b, nil
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
			s.mu.Lock()
			f.body, f.err = body, err
			if err == nil && s.gen == gen {
				s.port, s.token, s.body, s.fetchedAt = port, token, body, time.Now()
			}
			s.flight = nil
			s.mu.Unlock()
			close(f.done)
		}()
	}
	s.mu.Unlock()

	select {
	case <-f.done:
		return f.body, f.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// invalidateAllTrajectories drops the cached snapshot; call after RPCs that mutate the list.
func (p *Proxy) invalidateAllTrajectories() {
	s := &p.allTrajectories
	s.mu.Lock()
	s.body = nil
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

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(io.LimitReader(reader, maxAllTrajectoriesBody)); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
