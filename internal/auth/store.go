package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
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
	TokenHash  string    `json:"token_hash,omitempty"` // SEC-AUDIT L-4: omitempty hides empty hash from API listings
	CreatedAt  time.Time `json:"created_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
	LastSeenIP string    `json:"last_seen_ip"`
	// DeviceKey is a client-generated stable identifier (browser localStorage / app keychain).
	// Re-pairing with the same key replaces the existing device instead of adding a duplicate.
	DeviceKey string `json:"device_key,omitempty"`
	// E2EEKeyB64 holds the base64url-encoded 32-byte per-device AES-256-GCM key for zero-knowledge relay transport.
	// Redacted by ListDevices() before any external API response.
	E2EEKeyB64 string `json:"e2ee_key_b64,omitempty"`
	// E2EEReady indicates this device has negotiated an E2EE session key and can safely use relay_v1.
	E2EEReady bool `json:"e2ee_ready,omitempty"`
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
	saveSeq  uint64                  // monotonic snapshot counter assigned under mu
	filePath string

	// saveMu serializes disk writes outside mu so slow filesystem I/O never blocks ValidateToken RLock.
	saveMu       sync.Mutex
	lastSavedSeq uint64

	// P5: channel-based worker for UpdateLastSeen, avoiding per-request goroutine spawns.
	lastSeenCh chan lastSeenUpdate
	stopCh     chan struct{}
	workerDone chan struct{}
	bgWG       sync.WaitGroup
	closeOnce  sync.Once

	// S9: ticket store for short-lived one-time WebSocket / media authorization.
	wsTickets *WSTicketStore

	// Scoped short-lived session store for /gateway/sidecar-ui/ sub-resources.
	sidecarSessions *SidecarSessionStore

	// Optional lifecycle hooks (e.g. purging APNs push tokens when a device is unpaired/cleared).
	onDeviceRemoved func(deviceID string)
	onClearAll      func()
}

// SetOnDeviceRemoved registers a callback invoked after RemoveDevice succeeds.
func (s *AuthStore) SetOnDeviceRemoved(fn func(deviceID string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onDeviceRemoved = fn
}

// SetOnClearAll registers a callback invoked after ClearAll succeeds.
func (s *AuthStore) SetOnClearAll(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onClearAll = fn
}

func getTokenSalt() string {
	saltOnce.Do(func() {
		if s := strings.TrimSpace(os.Getenv("AUTH_SALT")); s != "" {
			cachedSalt = s
			return
		}
		cachedSalt = loadOrCreateAuthSalt("")
	})
	// S7: if salt is empty (rand.Read failed), return "" so HashToken can surface the failure.
	return cachedSalt
}

// DefaultDataDir returns the active configuration and data directory (~/.multigravity or MULTIGRAVITY_DATA_DIR).
func DefaultDataDir() string {
	if custom := strings.TrimSpace(os.Getenv("MULTIGRAVITY_DATA_DIR")); custom != "" {
		return ResolvePath(custom)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".multigravity"
	}
	return filepath.Join(home, ".multigravity")
}

// DefaultAuthStorePath returns the path to the auth store file.
func DefaultAuthStorePath() string {
	if custom := strings.TrimSpace(os.Getenv("MULTIGRAVITY_AUTH_STORE_PATH")); custom != "" {
		return ResolvePath(custom)
	}
	if custom := strings.TrimSpace(os.Getenv("AUTH_STORE_PATH")); custom != "" {
		return ResolvePath(custom)
	}
	dir := DefaultDataDir()
	return filepath.Join(dir, "auth_store.json")
}

func loadOrCreateAuthSalt(storePath string) string {
	dir := ""
	if storePath != "" {
		dir = filepath.Dir(storePath)
	} else {
		dir = DefaultDataDir()
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
		slog.Error("❌ [AuthStore] crypto/rand.Read failed to generate salt", "err", err)
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
// If filePath is empty, defaults to DefaultAuthStorePath().
func NewAuthStore(filePath string) (*AuthStore, error) {
	if filePath == "" {
		filePath = DefaultAuthStorePath()
	}
	resolved := ResolvePath(filePath)

	store := &AuthStore{
		devices:         make(map[string]PairedDevice),
		tokenMap:        make(map[string]string),
		filePath:        resolved,
		lastSeenCh:      make(chan lastSeenUpdate, 64), // P5: buffered channel; drops when full (debounce handles correctness)
		stopCh:          make(chan struct{}),
		workerDone:      make(chan struct{}),
		wsTickets:       NewWSTicketStore(), // S9: one-time WS ticket store
		sidecarSessions: NewSidecarSessionStore(),
	}

	saltOnce.Do(func() {
		if s := strings.TrimSpace(os.Getenv("AUTH_SALT")); s != "" {
			cachedSalt = s
			return
		}
		cachedSalt = loadOrCreateAuthSalt(resolved)
	})
	if cachedSalt == "" {
		return nil, fmt.Errorf("cryptographic failure: failed to initialize secure auth salt")
	}

	if err := store.load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load auth store from %s: %w", resolved, err)
	}

	if err := os.Chmod(resolved, 0600); err != nil && !os.IsNotExist(err) {
		slog.Warn(fmt.Sprintf("⚠️  [AuthStore] failed to set 0600 permissions on %s", resolved), "err", err)
	}

	// P5: single background worker drains lastSeenCh so the hot auth middleware path
	// never needs to spawn a goroutine per request.
	go store.lastSeenWorker()

	return store, nil
}

// lastSeenWorker drains the lastSeenCh channel and calls UpdateLastSeen sequentially.
func (s *AuthStore) lastSeenWorker() {
	defer close(s.workerDone)
	for {
		select {
		case <-s.stopCh:
			return
		case upd := <-s.lastSeenCh:
			select {
			case <-s.stopCh:
				return
			default:
				s.UpdateLastSeen(upd.deviceID, upd.remoteAddr)
			}
		}
	}
}

// Close stops background workers/GC tickers and waits for any in-flight atomic disk write
// to finish (preventing t.TempDir() RemoveAll races in unit tests).
func (s *AuthStore) Close() {
	if s == nil || s.stopCh == nil {
		return
	}
	s.closeOnce.Do(func() {
		close(s.stopCh)
		<-s.workerDone
		s.bgWG.Wait()
		if s.wsTickets != nil {
			s.wsTickets.Close()
		}
		if s.sidecarSessions != nil {
			s.sidecarSessions.Close()
		}
		s.saveMu.Lock()
		s.saveMu.Unlock()
	})
}

// EnqueueLastSeen sends a non-blocking update to the background worker.
// If the channel is full the update is silently dropped — the debounce logic
// in UpdateLastSeen ensures eventual consistency without data loss.
func (s *AuthStore) EnqueueLastSeen(deviceID, remoteAddr string) {
	if s.lastSeenCh == nil {
		return
	}
	if s.stopCh != nil {
		select {
		case <-s.stopCh:
			return
		default:
		}
		select {
		case <-s.stopCh:
		case s.lastSeenCh <- lastSeenUpdate{deviceID: deviceID, remoteAddr: remoteAddr}:
		default: // channel full — drop; the 30s debounce means we'll catch it next time
		}
		return
	}
	select {
	case s.lastSeenCh <- lastSeenUpdate{deviceID: deviceID, remoteAddr: remoteAddr}:
	default:
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
	return s.resolveSessionDevice(deviceID)
}

// IssueSidecarSession creates a short-lived session token scoped to /gateway/sidecar-ui/.
func (s *AuthStore) IssueSidecarSession(deviceID string) (string, error) {
	if s.sidecarSessions == nil {
		s.sidecarSessions = NewSidecarSessionStore()
	}
	return s.sidecarSessions.Issue(deviceID)
}

// ValidateSidecarSession validates a scoped sidecar session token without consuming it,
// ensuring the underlying paired device has not been revoked.
func (s *AuthStore) ValidateSidecarSession(sessionToken string) (*PairedDevice, bool) {
	if s.sidecarSessions == nil || sessionToken == "" {
		return nil, false
	}
	deviceID, ok := s.sidecarSessions.Validate(sessionToken)
	if !ok || deviceID == "" {
		return nil, false
	}
	return s.resolveSessionDevice(deviceID)
}

func (s *AuthStore) resolveSessionDevice(deviceID string) (*PairedDevice, bool) {
	if deviceID == "admin-local" {
		now := time.Now()
		return &PairedDevice{
			DeviceID:   "admin-local",
			DeviceName: "Local Administrator",
			Platform:   "local",
			CreatedAt:  now,
			LastSeenAt: now,
			LastSeenIP: "127.0.0.1",
		}, true
	}
	if strings.HasPrefix(deviceID, "lan-") {
		ip := strings.TrimPrefix(deviceID, "lan-")
		now := time.Now()
		return &PairedDevice{
			DeviceID:   deviceID,
			DeviceName: "LAN (" + ip + ")",
			Platform:   "lan",
			CreatedAt:  now,
			LastSeenAt: now,
			LastSeenIP: ip,
		}, true
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

// snapshotLocked captures a point-in-time slice of devices and assigns a monotonically
// increasing sequence number. Caller MUST hold s.mu.Lock().
func (s *AuthStore) snapshotLocked() ([]PairedDevice, uint64) {
	s.saveSeq++
	seq := s.saveSeq
	list := make([]PairedDevice, 0, len(s.devices))
	for _, dev := range s.devices {
		list = append(list, dev)
	}
	return list, seq
}

// persistSnapshot writes a snapshot to disk atomically under saveMu (never holding s.mu).
// If a newer snapshot (higher seq) has already been persisted, the stale write is skipped
// so a concurrent UpdateLastSeen never overwrites a newly paired or revoked device (P1-5).
func (s *AuthStore) persistSnapshot(list []PairedDevice, seq uint64) error {
	s.saveMu.Lock()
	defer s.saveMu.Unlock()

	if seq < s.lastSavedSeq {
		return nil
	}

	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create auth store dir: %w", err)
	}

	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}

	tmpFile := fmt.Sprintf("%s.tmp.%d", s.filePath, time.Now().UnixNano())
	if err := os.WriteFile(tmpFile, data, 0600); err != nil {
		return err
	}

	if err := os.Rename(tmpFile, s.filePath); err != nil {
		_ = os.Remove(tmpFile)
		return err
	}
	s.lastSavedSeq = seq
	return nil
}

// AddDevice adds or updates a paired device.
func (s *AuthStore) AddDevice(dev PairedDevice) error {
	s.mu.Lock()
	// If updating an existing device, remove its previous token to prevent orphaned active credentials
	if oldDev, exists := s.devices[dev.DeviceID]; exists && oldDev.TokenHash != "" {
		delete(s.tokenMap, oldDev.TokenHash)
	}

	s.devices[dev.DeviceID] = dev
	if dev.TokenHash != "" {
		s.tokenMap[dev.TokenHash] = dev.DeviceID
	}
	snap, seq := s.snapshotLocked()
	s.mu.Unlock()

	return s.persistSnapshot(snap, seq)
}

// FindByDeviceKey returns the paired device registered with the given client device key.
func (s *AuthStore) FindByDeviceKey(key string) (PairedDevice, bool) {
	if key == "" {
		return PairedDevice{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, d := range s.devices {
		if d.DeviceKey == key {
			return d, true
		}
	}
	return PairedDevice{}, false
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
	// Re-verify d.TokenHash == expectedLegacy under write lock so a concurrent token rotation
	// for the same DeviceKey is never overwritten by a stale migration (LOW-01).
	if isLegacy && hash != "" {
		expectedLegacy := LegacyHashToken(rawToken)
		auditInfo("TOKEN_MIGRATION", "device_id", deviceID, "detail", "legacy unsalted hash upgraded to salted hash")
		if s.stopCh != nil {
			select {
			case <-s.stopCh:
				devCopy := dev
				return &devCopy, true
			default:
			}
		}
		s.bgWG.Add(1)
		go func() {
			defer s.bgWG.Done()
			s.mu.Lock()
			d, ok := s.devices[deviceID]
			if !ok || d.TokenHash != expectedLegacy {
				s.mu.Unlock()
				return
			}
			delete(s.tokenMap, expectedLegacy)
			d.TokenHash = hash
			s.devices[deviceID] = d
			s.tokenMap[hash] = deviceID
			snap, seq := s.snapshotLocked()
			s.mu.Unlock()
			_ = s.persistSnapshot(snap, seq)
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
	dev, ok = s.devices[deviceID]
	if !ok {
		s.mu.Unlock()
		return
	}

	// Throttle disk writes: only save to disk if last saved was more than 1 minute ago or IP changed
	shouldSave := now.Sub(dev.LastSeenAt) > 1*time.Minute || dev.LastSeenIP != cleanIP

	dev.LastSeenAt = now
	dev.LastSeenIP = cleanIP
	s.devices[deviceID] = dev

	var snap []PairedDevice
	var seq uint64
	if shouldSave {
		snap, seq = s.snapshotLocked()
	}
	s.mu.Unlock()

	if shouldSave {
		_ = s.persistSnapshot(snap, seq)
	}
}

// ListDevices returns all paired devices without leaking secret fields.
func (s *AuthStore) ListDevices() []PairedDevice {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]PairedDevice, 0, len(s.devices))
	for _, dev := range s.devices {
		d := dev
		d.TokenHash = ""  // hide hash from external listing
		d.E2EEKeyB64 = "" // never leak E2EE session key in API listings
		result = append(result, d)
	}
	return result
}

// SetDeviceE2EEKey stores a device's 32-byte AES-256-GCM E2EE key and marks the device E2EE-ready.
func (s *AuthStore) SetDeviceE2EEKey(deviceID string, key []byte) error {
	if len(key) != 32 {
		return fmt.Errorf("invalid E2EE key length: %d", len(key))
	}
	s.mu.Lock()
	dev, ok := s.devices[deviceID]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("device not found: %s", deviceID)
	}
	dev.E2EEKeyB64 = base64.RawURLEncoding.EncodeToString(key)
	dev.E2EEReady = true
	s.devices[deviceID] = dev
	snap, seq := s.snapshotLocked()
	s.mu.Unlock()
	return s.persistSnapshot(snap, seq)
}

// GetDeviceE2EEKey returns a copy of the 32-byte AES-256-GCM key for deviceID, if configured.
func (s *AuthStore) GetDeviceE2EEKey(deviceID string) ([]byte, bool) {
	s.mu.RLock()
	dev, ok := s.devices[deviceID]
	s.mu.RUnlock()
	if !ok || !dev.E2EEReady || dev.E2EEKeyB64 == "" {
		return nil, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(dev.E2EEKeyB64)
	if err != nil || len(raw) != 32 {
		return nil, false
	}
	return raw, true
}

// HasLegacyNonE2EEDevices reports whether there is at least one paired device that has not yet upgraded to E2EE.
// Brand-new gateways (0 paired devices) return false so they can start directly on E2EE relay_v1.
func (s *AuthStore) HasLegacyNonE2EEDevices() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, dev := range s.devices {
		if !dev.E2EEReady {
			return true
		}
	}
	return false
}

// RemoveDevice removes a device by device_id.
func (s *AuthStore) RemoveDevice(deviceID string) error {
	s.mu.Lock()
	dev, ok := s.devices[deviceID]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("device not found: %s", deviceID)
	}

	delete(s.devices, deviceID)
	delete(s.tokenMap, dev.TokenHash)
	snap, seq := s.snapshotLocked()
	cb := s.onDeviceRemoved
	s.mu.Unlock()

	err := s.persistSnapshot(snap, seq)
	if err == nil && cb != nil {
		cb(deviceID)
	}
	return err
}

// ClearAll removes all paired devices. Returns the number of removed devices.
func (s *AuthStore) ClearAll() (int, error) {
	s.mu.Lock()
	count := len(s.devices)
	s.devices = make(map[string]PairedDevice)
	s.tokenMap = make(map[string]string)
	snap, seq := s.snapshotLocked()
	cb := s.onClearAll
	s.mu.Unlock()

	if err := s.persistSnapshot(snap, seq); err != nil {
		return 0, err
	}
	if cb != nil {
		cb()
	}
	return count, nil
}

// HasDevices returns whether any devices are currently paired.
func (s *AuthStore) HasDevices() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.devices) > 0
}
