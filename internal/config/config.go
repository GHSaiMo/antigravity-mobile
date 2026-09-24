package config

import (
	"bufio"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
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

// IsPlaceholderBarkKey reports whether raw contains unconfigured example placeholders like YOUR_DEVICE_KEY.
func IsPlaceholderBarkKey(raw string) bool {
	upper := strings.ToUpper(strings.TrimSpace(raw))
	return strings.Contains(upper, "YOUR_DEVICE_KEY") || strings.Contains(upper, "YOUR_KEY")
}

// NormalizeBarkEndpoint extracts the clean Bark API base device URL from various input formats.
// Supported inputs:
//   - "myDeviceKey123" -> "https://api.day.app/myDeviceKey123"
//   - "https://api.day.app/myDeviceKey123/" -> "https://api.day.app/myDeviceKey123"
//   - "https://api.day.app/myDeviceKey123/xxx?icon=yyy" -> "https://api.day.app/myDeviceKey123"
//   - "https://my-bark.com/mykey" -> "https://my-bark.com/mykey"
func NormalizeBarkEndpoint(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || IsPlaceholderBarkKey(raw) {
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

