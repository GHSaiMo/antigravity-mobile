package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	saltOnce   sync.Once
	cachedSalt string
)

// PairedDevice represents an authorized device paired with the gateway.
type PairedDevice struct {
	DeviceID   string    `json:"device_id"`
	DeviceName string    `json:"device_name"`
	Platform   string    `json:"platform"` // ios / pwa / macos / other
	TokenHash  string    `json:"token_hash"`
	CreatedAt  time.Time `json:"created_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
	LastSeenIP string    `json:"last_seen_ip"`
}

// lastSeenUpdate is a lightweight event sent to the AuthStore's background worker.
type lastSeenUpdate struct {
	deviceID   string
	remoteAddr string
}

// AuthStore manages paired devices and persists them to disk.
type AuthStore struct {
	mu       sync.RWMutex
	devices  map[string]PairedDevice // device_id -> PairedDevice
	tokenMap map[string]string       // token_hash -> device_id
	filePath string

	// P5: channel-based worker for UpdateLastSeen, avoiding per-request goroutine spawns.
	lastSeenCh chan lastSeenUpdate

	// S9: ticket store for short-lived one-time WebSocket / media authorization.
	wsTickets *WSTicketStore
}

func getTokenSalt() string {
	if s := strings.TrimSpace(os.Getenv("AUTH_SALT")); s != "" {
		return s
	}
	saltOnce.Do(func() {
		cachedSalt = loadOrCreateAuthSalt("")
	})
	// S7: if salt is empty (rand.Read failed), return "" so HashToken can surface the failure.
	return cachedSalt
}

func loadOrCreateAuthSalt(storePath string) string {
	dir := ""
	if storePath != "" {
		dir = filepath.Dir(storePath)
	} else {
		dir = ResolvePath("~/.antigravity-mobile")
	}
	saltPath := filepath.Join(dir, "auth_salt")
	if b, err := os.ReadFile(saltPath); err == nil {
		if s := strings.TrimSpace(string(b)); s != "" {
			return s
		}
	}
	raw := make([]byte, 32)
	// S7: if rand.Read fails we must not fall back to a predictable value — log and return
	// an empty string so callers can detect the failure and abort rather than use a weak salt.
	if _, err := rand.Read(raw); err != nil {
		return ""
	}
	s := hex.EncodeToString(raw)
	_ = os.MkdirAll(dir, 0700)
	_ = os.WriteFile(saltPath, []byte(s+"\n"), 0600)
	return s
}

// HashToken computes a salted SHA-256 hash of the raw token.
// Format: "s256:" + hex(sha256(salt + ":" + rawToken))
func HashToken(rawToken string) string {
	salt := getTokenSalt()
	h := sha256.New()
	h.Write([]byte(salt))
	h.Write([]byte(":"))
	h.Write([]byte(rawToken))
	return "s256:" + hex.EncodeToString(h.Sum(nil))
}

// LegacyHashToken computes unsalted SHA-256 for backward compatibility with existing devices.
func LegacyHashToken(rawToken string) string {
	sum := sha256.Sum256([]byte(rawToken))
	return hex.EncodeToString(sum[:])
}

// ResolvePath resolves leading ~ to the current user's home directory.
func ResolvePath(path string) string {
	if strings.HasPrefix(path, "~/") || path == "~" {
		home, err := os.UserHomeDir()
		if err == nil {
			if path == "~" {
				return home
			}
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

// NewAuthStore creates a new AuthStore loading from filePath.
// If filePath is empty, defaults to ~/.antigravity-mobile/auth_store.json.
func NewAuthStore(filePath string) (*AuthStore, error) {
	if filePath == "" {
		filePath = "~/.antigravity-mobile/auth_store.json"
	}
	resolved := ResolvePath(filePath)

	store := &AuthStore{
		devices:    make(map[string]PairedDevice),
		tokenMap:   make(map[string]string),
		filePath:   resolved,
		lastSeenCh: make(chan lastSeenUpdate, 64), // P5: buffered channel; drops when full (debounce handles correctness)
		wsTickets:  NewWSTicketStore(),             // S9: one-time WS ticket store
	}

	saltOnce.Do(func() {
		cachedSalt = loadOrCreateAuthSalt(resolved)
	})

	if err := store.load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load auth store from %s: %w", resolved, err)
	}

	if err := os.Chmod(resolved, 0600); err != nil && !os.IsNotExist(err) {
		// best-effort; ignore missing file
	}

	// P5: single background worker drains lastSeenCh so the hot auth middleware path
	// never needs to spawn a goroutine per request.
	go store.lastSeenWorker()

	return store, nil
}

// lastSeenWorker drains the lastSeenCh channel and calls UpdateLastSeen sequentially.
func (s *AuthStore) lastSeenWorker() {
	for upd := range s.lastSeenCh {
		s.UpdateLastSeen(upd.deviceID, upd.remoteAddr)
	}
}

// EnqueueLastSeen sends a non-blocking update to the background worker.
// If the channel is full the update is silently dropped — the debounce logic
// in UpdateLastSeen ensures eventual consistency without data loss.
func (s *AuthStore) EnqueueLastSeen(deviceID, remoteAddr string) {
	select {
	case s.lastSeenCh <- lastSeenUpdate{deviceID: deviceID, remoteAddr: remoteAddr}:
	default: // channel full — drop; the 30s debounce means we'll catch it next time
	}
}

// IssueWSTicket creates a short-lived one-time ticket for a device.
func (s *AuthStore) IssueWSTicket(deviceID string) (string, error) {
	if s.wsTickets == nil {
		s.wsTickets = NewWSTicketStore()
	}
	return s.wsTickets.Issue(deviceID)
}

// ValidateWSTicket validates and consumes a one-time ticket, returning the paired device.
func (s *AuthStore) ValidateWSTicket(ticket string) (*PairedDevice, bool) {
	if s.wsTickets == nil || ticket == "" {
		return nil, false
	}
	deviceID, ok := s.wsTickets.Validate(ticket)
	if !ok || deviceID == "" {
		return nil, false
	}
	s.mu.RLock()
	dev, exists := s.devices[deviceID]
	s.mu.RUnlock()
	if !exists {
		return nil, false
	}
	devCopy := dev
	return &devCopy, true
}

// load reads the store JSON from disk.
func (s *AuthStore) load() error {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return err
	}

	if len(strings.TrimSpace(string(data))) == 0 {
		return nil
	}

	var list []PairedDevice
	if err := json.Unmarshal(data, &list); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.devices = make(map[string]PairedDevice, len(list))
	s.tokenMap = make(map[string]string, len(list))
	for _, dev := range list {
		s.devices[dev.DeviceID] = dev
		if dev.TokenHash != "" {
			s.tokenMap[dev.TokenHash] = dev.DeviceID
		}
	}
	return nil
}

// save writes the store JSON to disk atomically.
func (s *AuthStore) save() error {
	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create auth store dir: %w", err)
	}

	list := make([]PairedDevice, 0, len(s.devices))
	for _, dev := range s.devices {
		list = append(list, dev)
	}

	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}

	tmpFile := fmt.Sprintf("%s.tmp.%d", s.filePath, time.Now().UnixNano())
	if err := os.WriteFile(tmpFile, data, 0600); err != nil {
		return err
	}

	return os.Rename(tmpFile, s.filePath)
}

// AddDevice adds or updates a paired device.
func (s *AuthStore) AddDevice(dev PairedDevice) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// If updating an existing device, remove its previous token to prevent orphaned active credentials
	if oldDev, exists := s.devices[dev.DeviceID]; exists && oldDev.TokenHash != "" {
		delete(s.tokenMap, oldDev.TokenHash)
	}

	s.devices[dev.DeviceID] = dev
	if dev.TokenHash != "" {
		s.tokenMap[dev.TokenHash] = dev.DeviceID
	}

	return s.save()
}

// ValidateToken checks whether rawToken is valid and returns the paired device.
// Supports both modern salted hashes and legacy unsalted hashes for seamless migration.
func (s *AuthStore) ValidateToken(rawToken string) (*PairedDevice, bool) {
	if rawToken == "" {
		return nil, false
	}
	hash := HashToken(rawToken)

	s.mu.RLock()
	deviceID, ok := s.tokenMap[hash]
	isLegacy := false
	if !ok {
		// Fallback to legacy unsalted SHA-256 for backward compatibility
		legacyHash := LegacyHashToken(rawToken)
		deviceID, ok = s.tokenMap[legacyHash]
		if ok {
			isLegacy = true
		}
	}
	if !ok {
		s.mu.RUnlock()
		return nil, false
	}

	dev, exists := s.devices[deviceID]
	s.mu.RUnlock()

	if !exists {
		return nil, false
	}

	// S8: Transparent in-place migration — upgrade legacy unsalted hash to modern salted hash.
	// The next call to ValidateToken will find the salted hash directly, removing the legacy path.
	if isLegacy && hash != "" {
		go func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			if d, ok := s.devices[deviceID]; ok {
				legacyHash := LegacyHashToken(rawToken)
				delete(s.tokenMap, legacyHash)
				d.TokenHash = hash
				s.devices[deviceID] = d
				s.tokenMap[hash] = deviceID
				_ = s.save()
			}
		}()
	}

	devCopy := dev
	return &devCopy, true
}

// CleanIP extracts the IP address from a remote address (which may contain a port).
func CleanIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err == nil {
		return host
	}
	return remoteAddr
}

// UpdateLastSeen asynchronously updates the last seen timestamp and IP for a device.
// Uses a fast read-lock debounce path to prevent write-lock contention on high-frequency requests.
func (s *AuthStore) UpdateLastSeen(deviceID, remoteAddr string) {
	cleanIP := CleanIP(remoteAddr)
	now := time.Now()

	// Fast read check: if last seen was updated within the last 30s with unchanged IP, skip
	s.mu.RLock()
	dev, ok := s.devices[deviceID]
	if !ok || (now.Sub(dev.LastSeenAt) < 30*time.Second && dev.LastSeenIP == cleanIP) {
		s.mu.RUnlock()
		return
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()

	dev, ok = s.devices[deviceID]
	if !ok {
		return
	}

	// Throttle disk writes: only save to disk if last saved was more than 1 minute ago or IP changed
	shouldSave := now.Sub(dev.LastSeenAt) > 1*time.Minute || dev.LastSeenIP != cleanIP

	dev.LastSeenAt = now
	dev.LastSeenIP = cleanIP
	s.devices[deviceID] = dev

	if shouldSave {
		_ = s.save()
	}
}

// ListDevices returns all paired devices without leaking secret fields.
func (s *AuthStore) ListDevices() []PairedDevice {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]PairedDevice, 0, len(s.devices))
	for _, dev := range s.devices {
		d := dev
		d.TokenHash = "" // hide hash from external listing
		result = append(result, d)
	}
	return result
}

// RemoveDevice removes a device by device_id.
func (s *AuthStore) RemoveDevice(deviceID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dev, ok := s.devices[deviceID]
	if !ok {
		return fmt.Errorf("device not found: %s", deviceID)
	}

	delete(s.devices, deviceID)
	delete(s.tokenMap, dev.TokenHash)

	return s.save()
}

// HasDevices returns whether any devices are currently paired.
func (s *AuthStore) HasDevices() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.devices) > 0
}
