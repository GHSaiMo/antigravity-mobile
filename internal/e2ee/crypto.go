package e2ee

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// NonceSize is the standard 96-bit AES-GCM nonce length.
	NonceSize = 12
	// KeySize is the 256-bit AES-GCM key length.
	KeySize = 32
	// PairSalt is the domain-separation salt for pairing handshake key derivation.
	PairSalt = "mgy-e2ee-pair-v1"
)

type relayContextKey struct{}

// WithRelayTransport marks a request/connection context as having arrived through the untrusted relay transport.
func WithRelayTransport(ctx context.Context) context.Context {
	return context.WithValue(ctx, relayContextKey{}, true)
}

// IsRelayTransport reports whether the request arrived through the untrusted relay transport.
func IsRelayTransport(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	v, _ := ctx.Value(relayContextKey{}).(bool)
	return v
}

// Identity holds the gateway's persistent P-256 ECDH keypair.
type Identity struct {
	priv *ecdh.PrivateKey
	pub  *ecdh.PublicKey
}

type persistedIdentity struct {
	Curve      string `json:"curve"`
	PrivateB64 string `json:"private_b64"`
	PublicB64  string `json:"public_b64"`
}

// GenerateIdentity generates a fresh ephemeral P-256 ECDH keypair (useful for tests & clients).
func GenerateIdentity() (*Identity, error) {
	priv, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate P-256 key: %w", err)
	}
	return &Identity{priv: priv, pub: priv.PublicKey()}, nil
}

// LoadOrCreateIdentity loads ~/.multigravity/e2ee_identity.json or generates and persists a new P-256 keypair.
func LoadOrCreateIdentity(dataDir string) (*Identity, error) {
	path := filepath.Join(dataDir, "e2ee_identity.json")
	if data, err := os.ReadFile(path); err == nil {
		var saved persistedIdentity
		if err := json.Unmarshal(data, &saved); err == nil && saved.PrivateB64 != "" {
			if rawPriv, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(saved.PrivateB64)); err == nil {
				if priv, err := ecdh.P256().NewPrivateKey(rawPriv); err == nil {
					_ = os.Chmod(path, 0600)
					return &Identity{priv: priv, pub: priv.PublicKey()}, nil
				}
			}
		}
	}

	id, err := GenerateIdentity()
	if err != nil {
		return nil, err
	}

	_ = os.MkdirAll(dataDir, 0700)
	payload, err := json.MarshalIndent(persistedIdentity{
		Curve:      "P-256",
		PrivateB64: base64.RawURLEncoding.EncodeToString(id.priv.Bytes()),
		PublicB64:  id.PublicKeyB64(),
	}, "", "  ")
	if err != nil {
		return nil, err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, payload, 0600); err == nil {
		if rErr := os.Rename(tmp, path); rErr != nil {
			_ = os.Remove(tmp)
			_ = os.WriteFile(path, payload, 0600)
		}
	} else {
		_ = os.WriteFile(path, payload, 0600)
	}
	return id, nil
}

// PublicKeyB64 returns the 65-byte uncompressed SEC1 P-256 public key encoded as Base64URL (no padding).
func (id *Identity) PublicKeyB64() string {
	return base64.RawURLEncoding.EncodeToString(id.pub.Bytes())
}

// ECDH computes the 32-byte raw P-256 ECDH shared secret with a peer's Base64URL-encoded 65-byte uncompressed public key.
func (id *Identity) ECDH(peerPubB64 string) ([]byte, error) {
	rawPeer, err := DecodeBase64Flexible(peerPubB64)
	if err != nil {
		return nil, fmt.Errorf("decode peer pubkey: %w", err)
	}
	peerPub, err := ecdh.P256().NewPublicKey(rawPeer)
	if err != nil {
		return nil, fmt.Errorf("invalid peer P-256 pubkey: %w", err)
	}
	secret, err := id.priv.ECDH(peerPub)
	if err != nil {
		return nil, fmt.Errorf("ecdh failed: %w", err)
	}
	return secret, nil
}

// DecodeBase64Flexible decodes Base64URL (raw or padded) or standard Base64.
func DecodeBase64Flexible(raw string) ([]byte, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return nil, errors.New("empty base64 string")
	}
	if b, err := base64.RawURLEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	if b, err := base64.URLEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	if b, err := base64.StdEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	return base64.RawStdEncoding.DecodeString(s)
}

// HKDFSHA256 implements single-block (32-byte) RFC 5869 HKDF-SHA256.
// Identical implementation across Go, Swift CryptoKit, Android JCA, and Web Crypto API.
func HKDFSHA256(ikm, salt, info []byte) []byte {
	if len(salt) == 0 {
		salt = make([]byte, sha256.Size)
	}
	extract := hmac.New(sha256.New, salt)
	extract.Write(ikm)
	prk := extract.Sum(nil)

	expand := hmac.New(sha256.New, prk)
	expand.Write(info)
	expand.Write([]byte{0x01})
	return expand.Sum(nil) // 32 bytes
}

// DerivePairKey derives the one-time 32-byte AES-256-GCM key used for encrypted pairing handshakes.
func DerivePairKey(sharedSecret []byte, clientPubB64, gatewayPubB64 string) []byte {
	info := []byte("pair:" + strings.TrimSpace(clientPubB64) + ":" + strings.TrimSpace(gatewayPubB64))
	return HKDFSHA256(sharedSecret, []byte(PairSalt), info)
}

// DeriveDeviceKey derives the persistent 32-byte per-device AES-256-GCM key.
func DeriveDeviceKey(sharedSecret, salt []byte, deviceID string) []byte {
	info := []byte("mgy-e2ee-device-v1:" + strings.TrimSpace(deviceID))
	return HKDFSHA256(sharedSecret, salt, info)
}

// RandomBytes returns n cryptographically random bytes.
func RandomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return b, nil
}

// Encrypt encrypts plaintext with AES-256-GCM, returning `nonce(12B) || ciphertext || tag(16B)`.
func Encrypt(key, plaintext, aad []byte) ([]byte, error) {
	if len(key) != KeySize {
		return nil, fmt.Errorf("invalid AES-256 key size %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, NonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	out := make([]byte, NonceSize, NonceSize+len(plaintext)+gcm.Overhead())
	copy(out, nonce)
	return gcm.Seal(out, nonce, plaintext, aad), nil
}

// Decrypt decrypts `nonce(12B) || ciphertext || tag(16B)` using AES-256-GCM.
func Decrypt(key, frame, aad []byte) ([]byte, error) {
	if len(key) != KeySize {
		return nil, fmt.Errorf("invalid AES-256 key size %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(frame) < NonceSize+gcm.Overhead() {
		return nil, errors.New("e2ee frame too short")
	}
	nonce := frame[:NonceSize]
	ciphertext := frame[NonceSize:]
	return gcm.Open(nil, nonce, ciphertext, aad)
}

// FrameAAD builds canonical AAD binding device ID and direction ("c2s", "s2c", "ws-open", "ws-c2s", "ws-s2c").
func FrameAAD(deviceID, direction string) []byte {
	return []byte("mgy-e2ee-v1:" + strings.TrimSpace(deviceID) + ":" + direction)
}

// PairFrameAAD builds canonical AAD for pairing handshake frames.
func PairFrameAAD(clientPubB64, direction string) []byte {
	return []byte("mgy-e2ee-pair-v1:" + strings.TrimSpace(clientPubB64) + ":" + direction)
}
