package cockpit

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	storageKeyFile = "secure-account-storage.key"
	storageVersion = 1
	storageAlgo    = "AES-256-GCM"
	storageKeyID   = "local-secure-account-storage-v1"
)

// EncryptedAccountEnvelope represents the AES-256-GCM encrypted envelope used by Cockpit Tools.
type EncryptedAccountEnvelope struct {
	Version     int    `json:"version"`
	Kind        string `json:"kind"`
	Algorithm   string `json:"algorithm"`
	KeyID       string `json:"key_id"`
	Nonce       string `json:"nonce"`
	Ciphertext  string `json:"ciphertext"`
	EncryptedAt int64  `json:"encrypted_at"`
}

// CockpitTokenData holds OAuth token information for an account.
type CockpitTokenData struct {
	AccessToken     string `json:"access_token"`
	RefreshToken    string `json:"refresh_token"`
	IDToken         string `json:"id_token,omitempty"`
	ExpiryTimestamp int64  `json:"expiry_timestamp"`
	ExpiresIn       int64  `json:"expires_in,omitempty"`
	TokenType       string `json:"token_type,omitempty"`
	OAuthClientKey  string `json:"oauth_client_key,omitempty"`
	IsGCPToS        *bool  `json:"is_gcp_tos,omitempty"`
	ProjectID       string `json:"project_id,omitempty"`
}

// CockpitAccountDetail represents the full decrypted account document stored in ~/.antigravity_cockpit/accounts/<id>.json.
type CockpitAccountDetail struct {
	ID        string           `json:"id"`
	Email     string           `json:"email"`
	Name      *string          `json:"name"`
	Disabled  bool             `json:"disabled"`
	CreatedAt int64            `json:"created_at"`
	LastUsed  int64            `json:"last_used"`
	Token     CockpitTokenData `json:"token"`
	Quota     any              `json:"quota,omitempty"`
}

var (
	cachedStorageKey   []byte
	cachedStorageKeyMu sync.RWMutex
)

// GetStorageKey reads the 32-byte AES-256 key from secure-account-storage.key, caching it in memory.
func GetStorageKey() ([]byte, error) {
	cachedStorageKeyMu.RLock()
	if len(cachedStorageKey) == 32 {
		k := make([]byte, 32)
		copy(k, cachedStorageKey)
		cachedStorageKeyMu.RUnlock()
		return k, nil
	}
	cachedStorageKeyMu.RUnlock()

	cachedStorageKeyMu.Lock()
	defer cachedStorageKeyMu.Unlock()
	if len(cachedStorageKey) == 32 {
		k := make([]byte, 32)
		copy(k, cachedStorageKey)
		return k, nil
	}

	dataDir, err := GetCockpitDataDir()
	if err != nil {
		return nil, err
	}
	keyPath := filepath.Join(dataDir, storageKeyFile)
	raw, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("read storage key file (%s): %w", keyPath, err)
	}
	keyBytes, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil {
		return nil, fmt.Errorf("decode storage key: %w", err)
	}
	if len(keyBytes) != 32 {
		return nil, fmt.Errorf("invalid storage key length: %d (expected 32)", len(keyBytes))
	}
	cachedStorageKey = keyBytes
	k := make([]byte, 32)
	copy(k, cachedStorageKey)
	return k, nil
}

// ResolveAccountID takes an account ID or email and returns the canonical account UUID.
func ResolveAccountID(accountIDOrEmail string) (string, error) {
	target := strings.TrimSpace(accountIDOrEmail)
	if target == "" {
		return "", errors.New("empty account ID or email")
	}

	dataDir, err := GetCockpitDataDir()
	if err != nil {
		return target, nil
	}
	accBytes, err := os.ReadFile(filepath.Join(dataDir, "accounts.json"))
	if err != nil {
		return target, nil
	}
	var idx accountsIndex
	if err := json.Unmarshal(accBytes, &idx); err != nil {
		return target, nil
	}

	targetLower := strings.ToLower(target)
	for _, acc := range idx.Accounts {
		if strings.EqualFold(acc.ID, target) || strings.ToLower(strings.TrimSpace(acc.Email)) == targetLower {
			return acc.ID, nil
		}
	}
	return target, nil
}

// DecryptAccountDetail decrypts and loads account details from ~/.antigravity_cockpit/accounts/<id>.json.
func DecryptAccountDetail(accountIDOrEmail string) (*CockpitAccountDetail, error) {
	accountID, err := ResolveAccountID(accountIDOrEmail)
	if err != nil {
		return nil, err
	}

	dataDir, err := GetCockpitDataDir()
	if err != nil {
		return nil, err
	}

	accountFilePath := filepath.Join(dataDir, "accounts", accountID+".json")
	docBytes, err := os.ReadFile(accountFilePath)
	if err != nil {
		return nil, fmt.Errorf("read account file (%s): %w", accountFilePath, err)
	}

	// Support both encrypted envelope and legacy plaintext JSON
	var envelope EncryptedAccountEnvelope
	if err := json.Unmarshal(docBytes, &envelope); err == nil && envelope.Algorithm == storageAlgo && envelope.Ciphertext != "" {
		key, kErr := GetStorageKey()
		if kErr != nil {
			return nil, fmt.Errorf("get storage key for decryption: %w", kErr)
		}
		nonce, nErr := base64.StdEncoding.DecodeString(strings.TrimSpace(envelope.Nonce))
		if nErr != nil || len(nonce) != 12 {
			return nil, fmt.Errorf("invalid nonce in account envelope: %v", nErr)
		}
		ciphertext, cErr := base64.StdEncoding.DecodeString(strings.TrimSpace(envelope.Ciphertext))
		if cErr != nil {
			return nil, fmt.Errorf("invalid ciphertext in account envelope: %v", cErr)
		}

		block, bErr := aes.NewCipher(key)
		if bErr != nil {
			return nil, fmt.Errorf("create aes cipher: %w", bErr)
		}
		aesgcm, gErr := cipher.NewGCM(block)
		if gErr != nil {
			return nil, fmt.Errorf("create gcm: %w", gErr)
		}

		plaintext, dErr := aesgcm.Open(nil, nonce, ciphertext, nil)
		if dErr != nil {
			return nil, fmt.Errorf("decrypt account envelope: %w", dErr)
		}

		var detail CockpitAccountDetail
		if err := json.Unmarshal(plaintext, &detail); err != nil {
			return nil, fmt.Errorf("unmarshal decrypted account detail: %w", err)
		}
		return &detail, nil
	}

	// Fallback to plain JSON
	var detail CockpitAccountDetail
	if err := json.Unmarshal(docBytes, &detail); err != nil {
		return nil, fmt.Errorf("unmarshal plaintext account detail: %w", err)
	}
	return &detail, nil
}

// SaveAccountDetail encrypts and writes account details back to ~/.antigravity_cockpit/accounts/<id>.json.
func SaveAccountDetail(acc *CockpitAccountDetail) error {
	if acc == nil || strings.TrimSpace(acc.ID) == "" {
		return errors.New("cannot save nil account or empty account ID")
	}

	dataDir, err := GetCockpitDataDir()
	if err != nil {
		return err
	}
	accountsDir := filepath.Join(dataDir, "accounts")
	if err := os.MkdirAll(accountsDir, 0700); err != nil {
		return fmt.Errorf("create accounts dir: %w", err)
	}

	key, err := GetStorageKey()
	if err != nil {
		return fmt.Errorf("get storage key for encryption: %w", err)
	}

	plainBytes, err := json.Marshal(acc)
	if err != nil {
		return fmt.Errorf("marshal account detail: %w", err)
	}

	nonce := make([]byte, 12)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return fmt.Errorf("generate nonce: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return fmt.Errorf("create aes cipher: %w", err)
	}
	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("create gcm: %w", err)
	}

	ciphertext := aesgcm.Seal(nil, nonce, plainBytes, nil)

	envelope := EncryptedAccountEnvelope{
		Version:     storageVersion,
		Kind:        "antigravity",
		Algorithm:   storageAlgo,
		KeyID:       storageKeyID,
		Nonce:       base64.StdEncoding.EncodeToString(nonce),
		Ciphertext:  base64.StdEncoding.EncodeToString(ciphertext),
		EncryptedAt: time.Now().Unix(),
	}

	envelopeBytes, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal encrypted envelope: %w", err)
	}

	targetPath := filepath.Join(accountsDir, acc.ID+".json")
	tmpPath := targetPath + ".tmp"
	if err := os.WriteFile(tmpPath, append(envelopeBytes, '\n'), 0600); err != nil {
		return fmt.Errorf("write tmp account file: %w", err)
	}
	if err := os.Rename(tmpPath, targetPath); err != nil {
		return fmt.Errorf("rename tmp account file: %w", err)
	}

	return nil
}

// ListAccountsFromStorage returns all decrypted accounts found in the Cockpit storage directory.
func ListAccountsFromStorage() ([]*CockpitAccountDetail, error) {
	dataDir, err := GetCockpitDataDir()
	if err != nil {
		return nil, err
	}
	accountsFile := filepath.Join(dataDir, "accounts.json")
	accBytes, err := os.ReadFile(accountsFile)
	if err != nil {
		return nil, err
	}
	var idx accountsIndex
	if err := json.Unmarshal(accBytes, &idx); err != nil {
		return nil, err
	}

	results := make([]*CockpitAccountDetail, 0, len(idx.Accounts))
	for _, a := range idx.Accounts {
		detail, err := DecryptAccountDetail(a.ID)
		if err != nil {
			continue
		}
		results = append(results, detail)
	}
	return results, nil
}
