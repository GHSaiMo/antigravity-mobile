package relay

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

const tokenPrefix = "mgyr1"

var (
	reShortID   = regexp.MustCompile(`^[0-9a-f]{8}$`)
	reOwnerHash = regexp.MustCompile(`^[0-9a-f]{16,64}$`)
	reHostname  = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)+$`)
)

// TokenClaims matches the stateless HMAC-SHA256 token payload signed by scripts/cloudflare-worker/worker.js.
type TokenClaims struct {
	Version   int    `json:"v"`
	ShortID   string `json:"sid"`
	OwnerHash string `json:"oh"`
	Domain    string `json:"dom"`
	Region    string `json:"reg,omitempty"`
	IssuedAt  int64  `json:"iat"`
}

// SignToken creates a wire-compatible `mgyr1.<payload_b64url>.<sig_b64url>` bearer token.
func SignToken(secret string, claims TokenClaims) (string, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return "", errors.New("relay signing secret is empty")
	}
	if claims.Version == 0 {
		claims.Version = 1
	}
	claims.ShortID = strings.ToLower(strings.TrimSpace(claims.ShortID))
	claims.OwnerHash = strings.ToLower(strings.TrimSpace(claims.OwnerHash))
	claims.Domain = strings.ToLower(strings.TrimSpace(claims.Domain))
	if claims.IssuedAt == 0 {
		claims.IssuedAt = time.Now().Unix()
	}

	rawJSON, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal token claims: %w", err)
	}
	payloadB64 := base64.RawURLEncoding.EncodeToString(rawJSON)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payloadB64))
	sigB64 := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return tokenPrefix + "." + payloadB64 + "." + sigB64, nil
}

// VerifyToken validates a `mgyr1.<payload_b64url>.<sig_b64url>` token in constant time.
// If maxAge > 0, tokens older than maxAge are rejected.
func VerifyToken(secret, rawToken string, now time.Time, maxAge time.Duration) (*TokenClaims, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return nil, errors.New("relay signing secret not configured")
	}
	rawToken = strings.TrimSpace(rawToken)
	parts := strings.Split(rawToken, ".")
	if len(parts) != 3 || parts[0] != tokenPrefix || parts[1] == "" || parts[2] == "" {
		return nil, errors.New("malformed relay token")
	}

	payloadB64 := parts[1]
	sigBytes, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, errors.New("invalid token signature encoding")
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payloadB64))
	expectedSig := mac.Sum(nil)
	if !hmac.Equal(sigBytes, expectedSig) {
		return nil, errors.New("invalid token signature")
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(payloadB64)
	if err != nil {
		return nil, errors.New("invalid token payload encoding")
	}

	var claims TokenClaims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, errors.New("invalid token payload json")
	}

	claims.ShortID = strings.ToLower(strings.TrimSpace(claims.ShortID))
	claims.OwnerHash = strings.ToLower(strings.TrimSpace(claims.OwnerHash))
	claims.Domain = strings.ToLower(strings.TrimSpace(claims.Domain))
	claims.Region = strings.ToLower(strings.TrimSpace(claims.Region))

	if claims.Version != 1 {
		return nil, fmt.Errorf("unsupported token version %d", claims.Version)
	}
	if !reShortID.MatchString(claims.ShortID) {
		return nil, errors.New("invalid token short_id")
	}
	if !reOwnerHash.MatchString(claims.OwnerHash) {
		return nil, errors.New("invalid token owner_hash")
	}
	if !reHostname.MatchString(claims.Domain) {
		return nil, errors.New("invalid token domain")
	}
	if claims.IssuedAt <= 0 {
		return nil, errors.New("missing token iat")
	}

	if now.IsZero() {
		now = time.Now()
	}
	issued := time.Unix(claims.IssuedAt, 0)
	if issued.After(now.Add(5 * time.Minute)) {
		return nil, errors.New("token issued in the future")
	}
	if maxAge > 0 && now.Sub(issued) > maxAge {
		return nil, errors.New("token expired")
	}

	return &claims, nil
}

// PeekTokenClaims parses and validates the structural fields of a `mgyr1.*` token payload
// without verifying its HMAC signature (used by the gateway client to derive PublicURL/Subdomain
// from an offline-issued BYO-Relay token when MGY_CUSTOM_RELAY_PUBLIC_URL is omitted).
func PeekTokenClaims(rawToken string) (*TokenClaims, error) {
	rawToken = strings.TrimSpace(rawToken)
	parts := strings.Split(rawToken, ".")
	if len(parts) != 3 || parts[0] != tokenPrefix || parts[1] == "" || parts[2] == "" {
		return nil, errors.New("malformed relay token")
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, errors.New("invalid token payload encoding")
	}
	var claims TokenClaims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, errors.New("invalid token payload json")
	}
	claims.ShortID = strings.ToLower(strings.TrimSpace(claims.ShortID))
	claims.OwnerHash = strings.ToLower(strings.TrimSpace(claims.OwnerHash))
	claims.Domain = strings.ToLower(strings.TrimSpace(claims.Domain))
	claims.Region = strings.ToLower(strings.TrimSpace(claims.Region))
	if claims.Version != 1 {
		return nil, fmt.Errorf("unsupported token version %d", claims.Version)
	}
	if !reHostname.MatchString(claims.Domain) {
		return nil, errors.New("invalid token domain")
	}
	return &claims, nil
}
