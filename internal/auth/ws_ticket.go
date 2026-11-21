package auth

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// wsTicketTTL is how long a WS ticket is valid after issuance.
const wsTicketTTL = 30 * time.Second

// maxTicketsPerDevice limits concurrent active tickets per device, preventing stockpiling.
const maxTicketsPerDevice = 5

// maxTotalWSTickets caps the overall size of the in-memory ticket map.
const maxTotalWSTickets = 1000

// wsTicket represents a short-lived, one-time-use token for WebSocket authentication.
// S9: replaces the long-lived auth_token in WebSocket URLs so the real token
// never appears in query strings (which leak via logs, Referer, browser history).
type wsTicket struct {
	deviceID  string
	createdAt time.Time
}

// WSTicketStore manages short-lived one-time tickets for WebSocket auth.
type WSTicketStore struct {
	mu        sync.Mutex
	tickets   map[string]*wsTicket
	stopCh    chan struct{}
	closeOnce sync.Once
}

// NewWSTicketStore creates a new ticket store and starts a background cleanup goroutine.
func NewWSTicketStore() *WSTicketStore {
	s := &WSTicketStore{
		tickets: make(map[string]*wsTicket),
		stopCh:  make(chan struct{}),
	}
	go s.gc()
	return s
}

// Close stops the background cleanup goroutine.
func (s *WSTicketStore) Close() {
	if s == nil || s.stopCh == nil {
		return
	}
	s.closeOnce.Do(func() {
		close(s.stopCh)
	})
}

// Issue creates a new one-time ticket bound to a device ID.
// The ticket is a random hex string, valid for wsTicketTTL.
func (s *WSTicketStore) Issue(deviceID string) (string, error) {
	raw := make([]byte, 20)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	ticket := hex.EncodeToString(raw)

	s.mu.Lock()
	defer s.mu.Unlock()

	// 1. Evict oldest ticket for this device if exceeding maxTicketsPerDevice
	var oldestDeviceTicketKey string
	var oldestDeviceTicketTime time.Time
	deviceCount := 0

	for k, t := range s.tickets {
		if t.deviceID == deviceID {
			deviceCount++
			if oldestDeviceTicketTime.IsZero() || t.createdAt.Before(oldestDeviceTicketTime) {
				oldestDeviceTicketTime = t.createdAt
				oldestDeviceTicketKey = k
			}
		}
	}
	if deviceCount >= maxTicketsPerDevice && oldestDeviceTicketKey != "" {
		delete(s.tickets, oldestDeviceTicketKey)
	}

	// 2. Global capacity safety net: prune expired or oldest if exceeding maxTotalWSTickets
	if len(s.tickets) >= maxTotalWSTickets {
		cutoff := time.Now().Add(-wsTicketTTL)
		for k, t := range s.tickets {
			if t.createdAt.Before(cutoff) {
				delete(s.tickets, k)
			}
		}
		if len(s.tickets) >= maxTotalWSTickets {
			var oldestKey string
			var oldestTime time.Time
			for k, t := range s.tickets {
				if oldestTime.IsZero() || t.createdAt.Before(oldestTime) {
					oldestTime = t.createdAt
					oldestKey = k
				}
			}
			if oldestKey != "" {
				delete(s.tickets, oldestKey)
			}
		}
	}

	s.tickets[ticket] = &wsTicket{
		deviceID:  deviceID,
		createdAt: time.Now(),
	}

	return ticket, nil
}

// Validate checks and consumes a ticket. Returns the device ID if valid.
// The ticket is deleted after first use (one-time).
func (s *WSTicketStore) Validate(ticket string) (deviceID string, ok bool) {
	if ticket == "" {
		return "", false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	t, exists := s.tickets[ticket]
	if !exists {
		return "", false
	}

	// Always delete — one-time use
	delete(s.tickets, ticket)

	if time.Since(t.createdAt) > wsTicketTTL {
		return "", false // expired
	}

	return t.deviceID, true
}

// gc cleans up expired tickets every 30 seconds.
func (s *WSTicketStore) gc() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			cutoff := time.Now().Add(-wsTicketTTL)
			s.mu.Lock()
			for k, t := range s.tickets {
				if t.createdAt.Before(cutoff) {
					delete(s.tickets, k)
				}
			}
			s.mu.Unlock()
		}
	}
}

// SidecarSessionTTL bounds how long a scoped sidecar-ui browser session cookie stays valid
// so static sub-resources (CSS/JS/API) and refreshes inside SFSafariViewController / Custom Tabs
// continue working after the one-time ?ticket= is consumed, without exposing the permanent device token.
const SidecarSessionTTL = 30 * time.Minute

const (
	maxSidecarSessionsPerDevice = 4
	maxTotalSidecarSessions     = 500
)

type sidecarSession struct {
	deviceID  string
	createdAt time.Time
}

// SidecarSessionStore manages short-lived, path-scoped session tokens for /gateway/sidecar-ui/.
type SidecarSessionStore struct {
	mu        sync.Mutex
	sessions  map[string]*sidecarSession
	stopCh    chan struct{}
	closeOnce sync.Once
}

// NewSidecarSessionStore creates a new sidecar session store with periodic GC.
func NewSidecarSessionStore() *SidecarSessionStore {
	s := &SidecarSessionStore{
		sessions: make(map[string]*sidecarSession),
		stopCh:   make(chan struct{}),
	}
	go s.gc()
	return s
}

// Close stops the background cleanup goroutine.
func (s *SidecarSessionStore) Close() {
	if s == nil || s.stopCh == nil {
		return
	}
	s.closeOnce.Do(func() {
		close(s.stopCh)
	})
}

// Issue creates a short-lived multi-read session token bound to deviceID.
func (s *SidecarSessionStore) Issue(deviceID string) (string, error) {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := hex.EncodeToString(raw)

	s.mu.Lock()
	defer s.mu.Unlock()

	var oldestDeviceKey string
	var oldestDeviceTime time.Time
	deviceCount := 0
	for k, sess := range s.sessions {
		if sess.deviceID == deviceID {
			deviceCount++
			if oldestDeviceTime.IsZero() || sess.createdAt.Before(oldestDeviceTime) {
				oldestDeviceTime = sess.createdAt
				oldestDeviceKey = k
			}
		}
	}
	if deviceCount >= maxSidecarSessionsPerDevice && oldestDeviceKey != "" {
		delete(s.sessions, oldestDeviceKey)
	}

	if len(s.sessions) >= maxTotalSidecarSessions {
		cutoff := time.Now().Add(-SidecarSessionTTL)
		for k, sess := range s.sessions {
			if sess.createdAt.Before(cutoff) {
				delete(s.sessions, k)
			}
		}
		if len(s.sessions) >= maxTotalSidecarSessions {
			var oldestKey string
			var oldestTime time.Time
			for k, sess := range s.sessions {
				if oldestTime.IsZero() || sess.createdAt.Before(oldestTime) {
					oldestTime = sess.createdAt
					oldestKey = k
				}
			}
			if oldestKey != "" {
				delete(s.sessions, oldestKey)
			}
		}
	}

	s.sessions[token] = &sidecarSession{
		deviceID:  deviceID,
		createdAt: time.Now(),
	}
	return token, nil
}

// Validate checks a sidecar session token without consuming it until SidecarSessionTTL expires.
func (s *SidecarSessionStore) Validate(token string) (deviceID string, ok bool) {
	if token == "" {
		return "", false
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	sess, exists := s.sessions[token]
	if !exists {
		return "", false
	}
	if time.Since(sess.createdAt) > SidecarSessionTTL {
		delete(s.sessions, token)
		return "", false
	}
	return sess.deviceID, true
}

func (s *SidecarSessionStore) gc() {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			cutoff := time.Now().Add(-SidecarSessionTTL)
			s.mu.Lock()
			for k, sess := range s.sessions {
				if sess.createdAt.Before(cutoff) {
					delete(s.sessions, k)
				}
			}
			s.mu.Unlock()
		}
	}
}
