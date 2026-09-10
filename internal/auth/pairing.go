package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"
)

// DefaultPairingTTL is the default validity duration for a pairing session (5 minutes).
const DefaultPairingTTL = 5 * time.Minute

// PairingSession represents an active in-memory pairing code session.
type PairingSession struct {
	Code      string    `json:"code"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// IsExpired reports whether the session has passed its expiration time.
func (s *PairingSession) IsExpired() bool {
	return time.Now().After(s.ExpiresAt)
}

// PairingManager coordinates one-time pairing sessions and credential issuance.
type PairingManager struct {
	mu            sync.Mutex
	sessions      map[string]*PairingSession
	latestSession *PairingSession
}

// NewPairingManager creates a new PairingManager.
func NewPairingManager() *PairingManager {
	pm := &PairingManager{
		sessions: make(map[string]*PairingSession),
	}
	return pm
}

// randomHex generates n random bytes and returns them as a lowercase hex string.
func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// GenerateSession generates a new 32-byte (64 hex characters) pairing code valid for ttl duration.
func (pm *PairingManager) GenerateSession(ttl time.Duration) (*PairingSession, error) {
	if ttl == 0 {
		ttl = DefaultPairingTTL
	}

	code, err := randomHex(32)
	if err != nil {
		return nil, fmt.Errorf("failed to generate random pairing code: %w", err)
	}

	now := time.Now()
	session := &PairingSession{
		Code:      code,
		CreatedAt: now,
		ExpiresAt: now.Add(ttl),
	}

	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Clear old expired sessions when generating a new one
	nowTime := time.Now()
	for k, s := range pm.sessions {
		if nowTime.After(s.ExpiresAt) {
			delete(pm.sessions, k)
		}
	}

	pm.sessions[code] = session
	pm.latestSession = session

	return session, nil
}

// LatestSession returns the most recently generated unexpired session, if any.
func (pm *PairingManager) LatestSession() *PairingSession {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.latestSession != nil && !pm.latestSession.IsExpired() {
		return pm.latestSession
	}
	return nil
}

// ValidateAndConsume validates the pairing code and immediately consumes/deletes it if valid.
func (pm *PairingManager) ValidateAndConsume(code string) bool {
	code = strings.TrimSpace(code)
	if code == "" {
		return false
	}

	pm.mu.Lock()
	defer pm.mu.Unlock()

	session, ok := pm.sessions[code]
	if !ok {
		return false
	}

	delete(pm.sessions, code)
	if pm.latestSession != nil && pm.latestSession.Code == code {
		pm.latestSession = nil
	}

	if session.IsExpired() {
		return false
	}

	return true
}

// CleanupExpired purges all expired sessions from memory.
func (pm *PairingManager) CleanupExpired() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	now := time.Now()
	for k, s := range pm.sessions {
		if now.After(s.ExpiresAt) {
			delete(pm.sessions, k)
		}
	}
	if pm.latestSession != nil && pm.latestSession.IsExpired() {
		pm.latestSession = nil
	}
}

// GenerateDeviceCredentials produces a new device_id and device_token.
// device_id: "dev_" + 12 hex chars
// device_token: "tok_" + 64 hex chars (32 random bytes)
func GenerateDeviceCredentials() (deviceID, deviceToken string, err error) {
	idHex, err := randomHex(6)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate device id: %w", err)
	}
	tokenHex, err := randomHex(32)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate device token: %w", err)
	}

	return "dev_" + idHex, "tok_" + tokenHex, nil
}
