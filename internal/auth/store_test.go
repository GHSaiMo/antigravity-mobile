package auth

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

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
