package cockpit

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestStorage_RoundtripEncryptionAndDecryption(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("COCKPIT_DATA_DIR", tmpDir)

	// 1. Generate and save a 32-byte key
	rawKey := make([]byte, 32)
	_, _ = rand.Read(rawKey)
	keyB64 := base64.StdEncoding.EncodeToString(rawKey)
	if err := os.WriteFile(filepath.Join(tmpDir, storageKeyFile), []byte(keyB64), 0600); err != nil {
		t.Fatalf("failed to write key file: %v", err)
	}

	// 2. Write accounts.json index
	idx := accountsIndex{
		CurrentAccountID: "acc-1",
		Accounts: []struct {
			ID    string `json:"id"`
			Email string `json:"email"`
			Name  string `json:"name"`
		}{
			{ID: "acc-1", Email: "test@example.com", Name: "Test User"},
		},
	}
	idxBytes, _ := json.Marshal(idx)
	_ = os.WriteFile(filepath.Join(tmpDir, "accounts.json"), idxBytes, 0644)

	// 3. Save account detail
	detail := &CockpitAccountDetail{
		ID:        "acc-1",
		Email:     "test@example.com",
		CreatedAt: 1234567,
		Token: CockpitTokenData{
			AccessToken:     "access-token-123",
			RefreshToken:    "refresh-token-456",
			ExpiryTimestamp: 9999999999,
		},
	}
	if err := SaveAccountDetail(detail); err != nil {
		t.Fatalf("SaveAccountDetail failed: %v", err)
	}

	// 4. Verify file on disk is an encrypted envelope
	content, err := os.ReadFile(filepath.Join(tmpDir, "accounts", "acc-1.json"))
	if err != nil {
		t.Fatalf("read saved account file: %v", err)
	}
	var env EncryptedAccountEnvelope
	if err := json.Unmarshal(content, &env); err != nil {
		t.Fatalf("account file is not a valid envelope: %v", err)
	}
	if env.Algorithm != "AES-256-GCM" || env.Ciphertext == "" {
		t.Fatalf("unexpected envelope content: %+v", env)
	}

	// 5. Decrypt by account ID
	decrypted, err := DecryptAccountDetail("acc-1")
	if err != nil {
		t.Fatalf("DecryptAccountDetail by ID failed: %v", err)
	}
	if decrypted.Token.AccessToken != "access-token-123" {
		t.Fatalf("access token mismatch: got %q", decrypted.Token.AccessToken)
	}
	if decrypted.Token.RefreshToken != "refresh-token-456" {
		t.Fatalf("refresh token mismatch: got %q", decrypted.Token.RefreshToken)
	}

	// 6. Decrypt by email (case insensitive)
	decryptedByEmail, err := DecryptAccountDetail("TEST@example.com")
	if err != nil {
		t.Fatalf("DecryptAccountDetail by Email failed: %v", err)
	}
	if decryptedByEmail.ID != "acc-1" {
		t.Fatalf("account ID mismatch: got %q", decryptedByEmail.ID)
	}
}
