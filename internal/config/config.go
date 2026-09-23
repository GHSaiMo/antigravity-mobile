package config

import (
	"bufio"
	"fmt"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// DefaultAntigravityIcon is the public URL for the Antigravity icon in this repository.
const DefaultAntigravityIcon = "https://raw.githubusercontent.com/GHSaiMo/antigravity-mobile/main/web/icons/icon-192.png"

// NotificationConfig holds settings for push notifications (Bark / FCM / Webhook).
type NotificationConfig struct {
	Enabled       bool
	BarkEndpoint  string // Normalized POST/GET endpoint, e.g. "https://api.day.app/YOUR_DEVICE_KEY"
	BarkRawURL    string
	IconURL       string
	Group         string
	SoundAction   string
	SoundComplete string

	// FCM (Android Push) fields
	FCMEnabled     bool
	FCMServerKey   string // Legacy FCM Server Key
	FCMDeviceToken string // Android device registration token
	FCMEndpoint    string // Custom FCM endpoint (defaults to https://fcm.googleapis.com/fcm/send)
}

// GetDataDir returns the active configuration and data directory (~/.multigravity or MULTIGRAVITY_DATA_DIR).
func GetDataDir() string {
	if custom := strings.TrimSpace(os.Getenv("MULTIGRAVITY_DATA_DIR")); custom != "" {
		return custom
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".multigravity"
	}
	return filepath.Join(home, ".multigravity")
}

// LoadDotEnv searches for a .env file in standard locations
// (explicit env, CWD, ~/.multigravity/.env, binary directory)
// and populates environment variables that are not already set.
func LoadDotEnv(paths ...string) {
	searchPaths := make([]string, 0, len(paths)+8)
	searchPaths = append(searchPaths, paths...)

	if custom := os.Getenv("MULTIGRAVITY_ENV"); custom != "" {
		searchPaths = append(searchPaths, custom)
	}

	// 1. Current directory and parent (for local development)
	searchPaths = append(searchPaths, ".env", "../.env")

	// 2. Global user directory (~/.multigravity/.env)
	if home, err := os.UserHomeDir(); err == nil {
		searchPaths = append(searchPaths, filepath.Join(home, ".multigravity", ".env"))
	}

	// 3. Executable directory and parent
	if execPath, err := os.Executable(); err == nil {
		searchPaths = append(searchPaths, filepath.Join(filepath.Dir(execPath), ".env"))
		searchPaths = append(searchPaths, filepath.Join(filepath.Dir(execPath), "..", ".env"))
	}

	for _, p := range searchPaths {
		if p == "" {
			continue
		}
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			parseEnvFile(p)
		}
	}
}

func parseEnvFile(filename string) {
	file, err := os.Open(filename)
	if err != nil {
		return
	}
	defer file.Close()

	// SEC: Enforce restrictive permissions on .env files if they contain sensitive secrets
	if fi, err := file.Stat(); err == nil {
		if fi.Mode().Perm()&0077 != 0 {
			_ = os.Chmod(filename, 0600)
		}
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		// Strip surrounding quotes
		if (strings.HasPrefix(val, "\"") && strings.HasSuffix(val, "\"")) ||
			(strings.HasPrefix(val, "'") && strings.HasSuffix(val, "'")) {
			if len(val) >= 2 {
				val = val[1 : len(val)-1]
			}
		}

		if os.Getenv(key) == "" {
			os.Setenv(key, val)
		}
	}
}

// NormalizeBarkEndpoint extracts the clean Bark API base device URL from various input formats.
// Supported inputs:
//   - "myDeviceKey123" -> "https://api.day.app/myDeviceKey123"
//   - "https://api.day.app/myDeviceKey123/" -> "https://api.day.app/myDeviceKey123"
//   - "https://api.day.app/myDeviceKey123/xxx?icon=yyy" -> "https://api.day.app/myDeviceKey123"
//   - "https://my-bark.com/mykey" -> "https://my-bark.com/mykey"
func NormalizeBarkEndpoint(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	// Case 1: Just the device key without URL scheme
	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		// Clean any trailing slashes or path
		key := strings.Trim(raw, "/")
		if idx := strings.Index(key, "/"); idx != -1 {
			key = key[:idx]
		}
		if key != "" {
			return "https://api.day.app/" + key
		}
		return ""
	}

	// Case 2: Full URL
	u, err := url.Parse(raw)
	if err != nil {
		return strings.TrimRight(raw, "/")
	}

	segments := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(segments) > 0 && segments[0] != "" {
		// Scheme + Host + first path segment (device key)
		return fmt.Sprintf("%s://%s/%s", u.Scheme, u.Host, segments[0])
	}

	return fmt.Sprintf("%s://%s", u.Scheme, u.Host)
}

// RedactBarkEndpoint hides the device key path segment for logs.
func RedactBarkEndpoint(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "(redacted)"
	}
	u.RawQuery = ""
	u.Fragment = ""
	u.Path = ""
	u.RawPath = ""
	return fmt.Sprintf("%s://%s/***", u.Scheme, u.Host)
}

// GetNotificationConfig parses environment variables and returns a NotificationConfig.
func GetNotificationConfig() NotificationConfig {
	barkRaw := os.Getenv("BARK_URL")
	if barkRaw == "" {
		barkRaw = os.Getenv("BARK_SERVER_URL")
	}
	if barkRaw == "" {
		barkRaw = os.Getenv("BARK_KEY")
	}

	endpoint := NormalizeBarkEndpoint(barkRaw)

	fcmServerKey := strings.TrimSpace(os.Getenv("FCM_SERVER_KEY"))
	fcmDeviceToken := strings.TrimSpace(os.Getenv("FCM_DEVICE_TOKEN"))
	fcmEndpoint := strings.TrimSpace(os.Getenv("FCM_ENDPOINT"))
	if fcmEndpoint == "" {
		fcmEndpoint = "https://fcm.googleapis.com/fcm/send"
	}

	fcmEnabled := fcmServerKey != "" && fcmDeviceToken != ""
	if v := os.Getenv("FCM_ENABLE"); v != "" {
		vLower := strings.ToLower(v)
		fcmEnabled = (vLower == "1" || vLower == "true" || vLower == "yes")
	}

	// Enabled if explicitly set or if a valid Bark or FCM endpoint is present
	enabled := endpoint != "" || fcmEnabled
	if v := os.Getenv("NOTIFICATION_ENABLE"); v != "" {
		vLower := strings.ToLower(v)
		enabled = (vLower == "1" || vLower == "true" || vLower == "yes")
	} else if v := os.Getenv("BARK_ENABLE"); v != "" {
		vLower := strings.ToLower(v)
		enabled = (vLower == "1" || vLower == "true" || vLower == "yes")
	}

	icon := os.Getenv("BARK_ICON")
	if icon == "" {
		icon = os.Getenv("BARK_ICON_URL")
	}
	if icon == "" {
		icon = DefaultAntigravityIcon
	}

	group := os.Getenv("BARK_GROUP")
	if group == "" {
		group = "Antigravity"
	}

	soundAction := os.Getenv("BARK_SOUND_ACTION")
	if soundAction == "" {
		soundAction = "alarm"
	}

	soundComplete := os.Getenv("BARK_SOUND_COMPLETE")
	if soundComplete == "" {
		soundComplete = "glass"
	}

	return NotificationConfig{
		Enabled:        enabled,
		BarkEndpoint:   endpoint,
		BarkRawURL:     barkRaw,
		IconURL:        icon,
		Group:          group,
		SoundAction:    soundAction,
		SoundComplete:  soundComplete,
		FCMEnabled:     fcmEnabled,
		FCMServerKey:   fcmServerKey,
		FCMDeviceToken: fcmDeviceToken,
		FCMEndpoint:    fcmEndpoint,
	}
}

// RedactFCMKey masks sensitive FCM server keys or tokens for logs.
func RedactFCMKey(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if len(raw) <= 8 {
		return "***"
	}
	return raw[:4] + "..." + raw[len(raw)-4:]
}

// TunnelConfig holds settings for embedded FRP cloud relay tunnel.
type TunnelConfig struct {
	Enabled    bool
	ServerAddr string
	ServerPort int
	Token      string
	RemotePort int
	TLSEnable  bool
}

// GetTunnelConfig parses environment variables for the embedded FRP tunnel.
func GetTunnelConfig() TunnelConfig {
	serverAddr := strings.TrimSpace(os.Getenv("FRP_SERVER_ADDR"))
	if serverAddr == "" {
		serverAddr = strings.TrimSpace(os.Getenv("FRP_HOST"))
	}

	serverPort := 7000
	if pStr := os.Getenv("FRP_SERVER_PORT"); pStr != "" {
		if p, err := strconv.Atoi(pStr); err == nil && p > 0 {
			serverPort = p
		}
	}

	token := strings.TrimSpace(os.Getenv("FRP_TOKEN"))

	remotePort := 58900
	if pStr := os.Getenv("FRP_REMOTE_PORT"); pStr != "" {
		if p, err := strconv.Atoi(pStr); err == nil && p > 0 {
			remotePort = p
		}
	} else if pStr := os.Getenv("MULTIGRAVITY_PORT"); pStr != "" {
		if p, err := strconv.Atoi(pStr); err == nil && p > 0 {
			remotePort = p
		}
	}

	enabled := serverAddr != ""
	if v := os.Getenv("FRP_ENABLED"); v != "" {
		vLower := strings.ToLower(v)
		enabled = (vLower == "1" || vLower == "true" || vLower == "yes")
	}

	tlsEnable := true
	if v := os.Getenv("FRP_TLS_ENABLE"); v != "" {
		vLower := strings.ToLower(v)
		tlsEnable = (vLower == "1" || vLower == "true" || vLower == "yes")
	} else if v := os.Getenv("FRP_TLS"); v != "" {
		vLower := strings.ToLower(v)
		tlsEnable = (vLower == "1" || vLower == "true" || vLower == "yes")
	}

	return TunnelConfig{
		Enabled:    enabled,
		ServerAddr: serverAddr,
		ServerPort: serverPort,
		Token:      token,
		RemotePort: remotePort,
		TLSEnable:  tlsEnable,
	}
}

// ValidateFRPTokenStrength checks if the configured FRP_TOKEN is weak or low-entropy (C-2).
// It returns an advisory warning message if the token is sub-optimal.
func ValidateFRPTokenStrength(token string) string {
	tok := strings.TrimSpace(token)
	if tok == "" {
		return "⚠️  FRP_TOKEN is empty! Cloud relay requires a valid token to authenticate."
	}
	weakTokens := []string{
		"admin", "123456", "12345678", "password", "frp", "frp123", "frptoken",
		"your_frp_auth_token", "your_token", "default", "secret", "agysecure2026token",
	}
	tokLower := strings.ToLower(tok)
	for _, w := range weakTokens {
		if tokLower == w {
			return fmt.Sprintf("⚠️  [SECURITY WARNING] FRP_TOKEN %q is a well-known weak/example token! Generate a secure random token using: openssl rand -hex 32", tok)
		}
	}

	// Detect dictionary combinations or predictable word patterns
	predictableWords := []string{"agy", "token", "secure", "pass", "admin", "server", "2026", "2025"}
	hitCount := 0
	for _, pw := range predictableWords {
		if strings.Contains(tokLower, pw) {
			hitCount++
		}
	}
	if hitCount >= 2 && len(tok) < 32 {
		return fmt.Sprintf("⚠️  [SECURITY WARNING] FRP_TOKEN %q appears to be composed of predictable dictionary words. Recommended: openssl rand -hex 32", tok)
	}

	// SEC-AUDIT L-3: Shannon entropy detection — catch low-entropy tokens that bypass
	// the dictionary check (e.g. "aaaaabbbbbccccc", "abc123abc123").
	if len(tok) < 32 {
		entropy := shannonEntropy(tok)
		if entropy < 3.0 {
			return fmt.Sprintf("⚠️  [SECURITY WARNING] FRP_TOKEN entropy is very low (%.1f bits/char). For internet-facing relay security, generate a high-entropy token: openssl rand -hex 32", entropy)
		}
	}

	if len(tok) < 24 {
		return fmt.Sprintf("⚠️  [SECURITY WARNING] FRP_TOKEN length (%d) is shorter than 24 characters. For internet-facing relay security, generate at least 32 random hex characters: openssl rand -hex 32", len(tok))
	}
	return ""
}

// shannonEntropy calculates the Shannon entropy in bits per character of a string.
// SEC-AUDIT L-3: Used to detect low-entropy FRP tokens that pass dictionary checks.
func shannonEntropy(s string) float64 {
	if len(s) == 0 {
		return 0
	}
	freq := make(map[rune]int)
	for _, r := range s {
		freq[r]++
	}
	length := float64(len([]rune(s)))
	var entropy float64
	for _, count := range freq {
		p := float64(count) / length
		if p > 0 {
			entropy -= p * math.Log2(p)
		}
	}
	return entropy
}

// AdvertisePublicIPv6 reports whether pairing QR / endpoints should include the
// machine's global unicast IPv6. Defaults to true whenever a global IPv6 is detected,
// unless explicitly disabled via INCLUDE_PUBLIC_IPV6=0, false, or no.
func AdvertisePublicIPv6(sslEnabled bool) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("MULTIGRAVITY_INCLUDE_PUBLIC_IPV6")))
	if v == "" {
		v = strings.ToLower(strings.TrimSpace(os.Getenv("INCLUDE_PUBLIC_IPV6")))
	}
	if v == "0" || v == "false" || v == "no" {
		return false
	}
	return true
}

// DefaultCloudflareWorkerURL is the default public dispatcher URL.
const DefaultCloudflareWorkerURL = "https://dispatcher.jiuge.space"

// CloudflareConfig holds settings for the automated Cloudflare Tunnel dispatcher.
type CloudflareConfig struct {
	Enabled       bool
	WorkerURL     string
	InviteCode    string
	Token         string // manual token override if desired
	EdgeIPVersion string // "auto", "4", "6"
	Protocol      string // "quic", "http2"
	Region        string // optional region code
}

// GetCloudflareConfig extracts Cloudflare Tunnel settings from environment variables.
// Cloudflare Tunnel is ENABLED by default out of the box using DefaultCloudflareWorkerURL.
func GetCloudflareConfig() CloudflareConfig {
	workerURL := strings.TrimSpace(os.Getenv("CF_WORKER_URL"))
	if workerURL == "" {
		workerURL = strings.TrimSpace(os.Getenv("CLOUDFLARE_WORKER_URL"))
	}
	if workerURL == "" {
		workerURL = DefaultCloudflareWorkerURL
	}
	inviteCode := strings.TrimSpace(os.Getenv("CF_INVITE_CODE"))
	if inviteCode == "" {
		inviteCode = strings.TrimSpace(os.Getenv("CLOUDFLARE_INVITE_CODE"))
	}
	token := strings.TrimSpace(os.Getenv("CF_TUNNEL_TOKEN"))
	if token == "" {
		token = strings.TrimSpace(os.Getenv("CLOUDFLARE_TUNNEL_TOKEN"))
	}

	edgeIPVersion := strings.TrimSpace(os.Getenv("CF_EDGE_IP_VERSION"))
	if edgeIPVersion == "" {
		edgeIPVersion = strings.TrimSpace(os.Getenv("TUNNEL_EDGE_IP_VERSION"))
	}
	if edgeIPVersion == "" {
		edgeIPVersion = "4" // 默认优先 IPv4，避免跨洋 IPv6 Anycast 绕路
	}

	protocol := strings.TrimSpace(os.Getenv("CF_PROTOCOL"))
	if protocol == "" {
		protocol = strings.TrimSpace(os.Getenv("TUNNEL_TRANSPORT_PROTOCOL"))
	}
	if protocol == "" {
		protocol = "http2" // 默认走 TCP HTTP/2，避免国内运营商对 UDP/QUIC 丢包限速，且与代理兼容性极佳
	}

	region := strings.TrimSpace(os.Getenv("CF_REGION"))
	if region == "" {
		region = strings.TrimSpace(os.Getenv("TUNNEL_REGION"))
	}

	enabled := true
	if v := os.Getenv("CF_TUNNEL_ENABLED"); v != "" {
		vLower := strings.ToLower(v)
		enabled = (vLower == "1" || vLower == "true" || vLower == "yes")
	} else if v := os.Getenv("CLOUDFLARE_TUNNEL_ENABLED"); v != "" {
		vLower := strings.ToLower(v)
		enabled = (vLower == "1" || vLower == "true" || vLower == "yes")
	}

	return CloudflareConfig{
		Enabled:       enabled,
		WorkerURL:     workerURL,
		InviteCode:    inviteCode,
		Token:         token,
		EdgeIPVersion: edgeIPVersion,
		Protocol:      protocol,
		Region:        region,
	}
}

