package auth

import (
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

// AuthStore manages paired devices and persists them to disk.
type AuthStore struct {
	mu       sync.RWMutex
	devices  map[string]PairedDevice // device_id -> PairedDevice
	tokenMap map[string]string       // token_hash -> device_id
	filePath string
}

// HashToken computes SHA-256 hex string of raw token.
func HashToken(rawToken string) string {
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
		devices:  make(map[string]PairedDevice),
		tokenMap: make(map[string]string),
		filePath: resolved,
	}

	if err := store.load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load auth store from %s: %w", resolved, err)
	}

	return store, nil
}

// load reads the store JSON from disk.
func (s *AuthStore) load() error {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return err
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

	s.devices[dev.DeviceID] = dev
	if dev.TokenHash != "" {
		s.tokenMap[dev.TokenHash] = dev.DeviceID
	}

	return s.save()
}

// ValidateToken checks whether rawToken is valid and returns the paired device.
func (s *AuthStore) ValidateToken(rawToken string) (*PairedDevice, bool) {
	if rawToken == "" {
		return nil, false
	}
	hash := HashToken(rawToken)

	s.mu.RLock()
	defer s.mu.RUnlock()

	deviceID, ok := s.tokenMap[hash]
	if !ok {
		return nil, false
	}

	dev, exists := s.devices[deviceID]
	if !exists {
		return nil, false
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
func (s *AuthStore) UpdateLastSeen(deviceID, remoteAddr string) {
	cleanIP := CleanIP(remoteAddr)

	s.mu.Lock()
	dev, ok := s.devices[deviceID]
	if !ok {
		s.mu.Unlock()
		return
	}

	// Throttle disk writes: only save to disk if last saved was more than 1 minute ago
	now := time.Now()
	shouldSave := now.Sub(dev.LastSeenAt) > 1*time.Minute || dev.LastSeenIP != cleanIP

	dev.LastSeenAt = now
	dev.LastSeenIP = cleanIP
	s.devices[deviceID] = dev
	s.mu.Unlock()

	if shouldSave {
		s.mu.Lock()
		_ = s.save()
		s.mu.Unlock()
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
