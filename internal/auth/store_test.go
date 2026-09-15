package auth

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	_ = os.Setenv("AUTH_SALT", "unit-test-auth-salt")
	os.Exit(m.Run())
}

func TestAuthStore_AddAndValidate(t *testing.T) {
	tempDir := t.TempDir()
	storePath := filepath.Join(tempDir, "auth_store.json")

	store, err := NewAuthStore(storePath)
	if err != nil {
		t.Fatalf("failed to create auth store: %v", err)
	}

	token := "tok_secret1234567890abcdef"
	dev := PairedDevice{
		DeviceID:   "dev_test01",
		DeviceName: "iPhone 15",
		Platform:   "ios",
		TokenHash:  HashToken(token),
		CreatedAt:  time.Now(),
		LastSeenAt: time.Now(),
		LastSeenIP: "192.168.1.100",
	}

	if err := store.AddDevice(dev); err != nil {
		t.Fatalf("failed to add device: %v", err)
	}

	// Validate valid token
	foundDev, ok := store.ValidateToken(token)
	if !ok {
		t.Fatalf("expected token to be valid")
	}
	if foundDev.DeviceID != dev.DeviceID {
		t.Errorf("expected device ID %s, got %s", dev.DeviceID, foundDev.DeviceID)
	}

	// Validate invalid token
	_, ok = store.ValidateToken("tok_invalid")
	if ok {
		t.Errorf("expected invalid token to fail")
	}

	// Reload from disk and verify persistence
	store2, err := NewAuthStore(storePath)
	if err != nil {
		t.Fatalf("failed to reload auth store: %v", err)
	}
	foundDev2, ok := store2.ValidateToken(token)
	if !ok || foundDev2.DeviceID != dev.DeviceID {
		t.Errorf("expected device to persist across reloads")
	}

	// Test RemoveDevice
	if err := store2.RemoveDevice(dev.DeviceID); err != nil {
		t.Fatalf("failed to remove device: %v", err)
	}
	_, ok = store2.ValidateToken(token)
	if ok {
		t.Errorf("expected removed device token to be invalid")
	}
}

func TestResolvePath(t *testing.T) {
	home, _ := os.UserHomeDir()
	res := ResolvePath("~/test/path")
	expected := filepath.Join(home, "test/path")
	if res != expected {
		t.Errorf("expected %s, got %s", expected, res)
	}

	normal := "/tmp/file.json"
	if ResolvePath(normal) != normal {
		t.Errorf("expected %s, got %s", normal, ResolvePath(normal))
	}
}

func TestAuthStore_LegacyTokenCompatibility(t *testing.T) {
	tempDir := t.TempDir()
	storePath := filepath.Join(tempDir, "auth_store_legacy.json")

	store, err := NewAuthStore(storePath)
	if err != nil {
		t.Fatalf("failed to create auth store: %v", err)
	}

	rawToken := "tok_legacy_token_12345"
	// Manually insert device with legacy unsalted hash
	legacyDev := PairedDevice{
		DeviceID:   "dev_legacy_01",
		DeviceName: "Legacy iPad",
		Platform:   "ios",
		TokenHash:  LegacyHashToken(rawToken),
		CreatedAt:  time.Now(),
		LastSeenAt: time.Now(),
	}

	if err := store.AddDevice(legacyDev); err != nil {
		t.Fatalf("failed to add legacy device: %v", err)
	}

	// ValidateToken MUST succeed using legacy fallback
	dev, ok := store.ValidateToken(rawToken)
	if !ok {
		t.Fatalf("expected legacy unsalted token to validate successfully")
	}
	if dev.DeviceID != "dev_legacy_01" {
		t.Errorf("expected device ID dev_legacy_01, got %s", dev.DeviceID)
	}
}

func TestAuthStore_TokenRotation(t *testing.T) {
	tempDir := t.TempDir()
	storePath := filepath.Join(tempDir, "auth_store_rotation.json")

	store, err := NewAuthStore(storePath)
	if err != nil {
		t.Fatalf("failed to create auth store: %v", err)
	}

	oldToken := "tok_initial_token_111"
	newToken := "tok_rotated_token_222"

	dev := PairedDevice{
		DeviceID:   "dev_rotate_01",
		DeviceName: "My Phone",
		Platform:   "ios",
		TokenHash:  HashToken(oldToken),
		CreatedAt:  time.Now(),
	}
	_ = store.AddDevice(dev)

	// Old token valid
	if _, ok := store.ValidateToken(oldToken); !ok {
		t.Fatalf("expected old token to be valid initially")
	}

	// Rotate token for same device
	dev.TokenHash = HashToken(newToken)
	_ = store.AddDevice(dev)

	// Old token MUST be revoked (no orphan active tokens)
	if _, ok := store.ValidateToken(oldToken); ok {
		t.Errorf("expected old token to be revoked after rotation, but was still valid!")
	}

	// New token MUST be valid
	if _, ok := store.ValidateToken(newToken); !ok {
		t.Errorf("expected new token to be valid after rotation")
	}
}
