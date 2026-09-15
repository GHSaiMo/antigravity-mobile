package auth

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// wsTicketTTL is how long a WS ticket is valid after issuance.
const wsTicketTTL = 30 * time.Second

// wsTicket represents a short-lived, one-time-use token for WebSocket authentication.
// S9: replaces the long-lived auth_token in WebSocket URLs so the real token
// never appears in query strings (which leak via logs, Referer, browser history).
type wsTicket struct {
	deviceID  string
	createdAt time.Time
}

// WSTicketStore manages short-lived one-time tickets for WebSocket auth.
type WSTicketStore struct {
	mu      sync.Mutex
	tickets map[string]*wsTicket
}

// NewWSTicketStore creates a new ticket store and starts a background cleanup goroutine.
func NewWSTicketStore() *WSTicketStore {
	s := &WSTicketStore{
		tickets: make(map[string]*wsTicket),
	}
	go s.gc()
	return s
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
	s.tickets[ticket] = &wsTicket{
		deviceID:  deviceID,
		createdAt: time.Now(),
	}
	s.mu.Unlock()

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
	for range ticker.C {
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
