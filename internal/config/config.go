package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// DefaultAntigravityIcon is the public URL for the Antigravity icon in this repository.
const DefaultAntigravityIcon = "https://raw.githubusercontent.com/GHSaiMo/antigravity-mobile/main/web/icons/icon-192.png"

// DefaultAPNSRelayURL is the default stateless APNs relay Worker URL for App Store iOS clients.
const DefaultAPNSRelayURL = "https://push.multigravity.app"

// NotificationConfig holds settings for push notifications (APNs / FCM / Webhook).
type NotificationConfig struct {
	Enabled       bool
	IconURL       string
	Group         string
	SoundAction   string
	SoundComplete string

	// APNs (iOS Native Push & Live Activity) fields
	APNSEnabled             bool
	APNSRelayURL            string // Stateless APNs relay endpoint (defaults to DefaultAPNSRelayURL)
	LiveActivityPushEnabled bool   // Enable remote Dynamic Island / Live Activity updates via APNs
	PushStorePath           string // Persisted push token store path (defaults to ~/.multigravity/push_devices.json)
	PushRedact              bool   // Strip session titles, commands, paths, and text previews from outgoing push relays

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

// GetNotificationConfig parses environment variables and returns a NotificationConfig.
func GetNotificationConfig() NotificationConfig {
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

	// Enabled if explicitly set or if FCM is configured (APNs dynamically activates when iOS devices register)
	enabled := fcmEnabled
	if v := os.Getenv("NOTIFICATION_ENABLE"); v != "" {
		vLower := strings.ToLower(v)
		enabled = (vLower == "1" || vLower == "true" || vLower == "yes")
	}

	icon := os.Getenv("NOTIFICATION_ICON")
	if icon == "" {
		icon = DefaultAntigravityIcon
	}

	group := os.Getenv("NOTIFICATION_GROUP")
	if group == "" {
		group = "Antigravity"
	}

	soundAction := "alarm"
	soundComplete := "glass"

	apnsRelayURL := strings.TrimRight(strings.TrimSpace(os.Getenv("APNS_RELAY_URL")), "/")
	if apnsRelayURL == "" {
		apnsRelayURL = DefaultAPNSRelayURL
	}
	apnsEnabled := true
	if v := strings.TrimSpace(os.Getenv("APNS_ENABLED")); v != "" {
		vLower := strings.ToLower(v)
		apnsEnabled = (vLower == "1" || vLower == "true" || vLower == "yes" || vLower == "on")
	} else if v := strings.TrimSpace(os.Getenv("APNS_ENABLE")); v != "" {
		vLower := strings.ToLower(v)
		apnsEnabled = (vLower == "1" || vLower == "true" || vLower == "yes" || vLower == "on")
	}

	liveActivityPushEnabled := apnsEnabled
	if v := strings.TrimSpace(os.Getenv("LIVE_ACTIVITY_PUSH_ENABLED")); v != "" {
		vLower := strings.ToLower(v)
		liveActivityPushEnabled = apnsEnabled && (vLower == "1" || vLower == "true" || vLower == "yes" || vLower == "on")
	}

	pushStorePath := strings.TrimSpace(os.Getenv("MULTIGRAVITY_PUSH_STORE_PATH"))
	if pushStorePath == "" {
		pushStorePath = filepath.Join(GetDataDir(), "push_devices.json")
	}

	pushRedact := envBoolAny("MGY_PUSH_REDACT", "PUSH_REDACT", "APNS_REDACT")

	return NotificationConfig{
		Enabled:                 enabled,
		IconURL:                 icon,
		Group:                   group,
		SoundAction:             soundAction,
		SoundComplete:           soundComplete,
		APNSEnabled:             apnsEnabled,
		APNSRelayURL:            apnsRelayURL,
		LiveActivityPushEnabled: liveActivityPushEnabled,
		PushStorePath:           pushStorePath,
		PushRedact:              pushRedact,
		FCMEnabled:              fcmEnabled,
		FCMServerKey:            fcmServerKey,
		FCMDeviceToken:          fcmDeviceToken,
		FCMEndpoint:             fcmEndpoint,
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

// DefaultCloudflareWorkerURL is the default public dispatcher URL.
const DefaultCloudflareWorkerURL = "https://dispatcher.multigravity.app"

// CloudflareConfig holds settings for the automated Cloudflare Tunnel dispatcher.
type CloudflareConfig struct {
	Enabled       bool
	WorkerURL     string
	InviteCode    string
	Token         string // manual token override if desired
	EdgeIPVersion string // "auto", "4", "6"
	Protocol      string // "quic", "http2"
	Region        string // optional region code
	DNSResolvers  string // optional custom DNS resolvers, e.g. "223.5.5.5:53,119.29.29.29:53"
	EdgeProbe     bool   // probe IPv4/IPv6 edge RTT before launch and pin the faster family (only when EdgeIPVersion is auto)
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
		edgeIPVersion = "auto" // 默认双栈自适应 (auto)，有 IPv6 时优先/直连干净节点，避免纯 IPv4 遭遇国内 DNS 投毒
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

	dnsResolvers := strings.TrimSpace(os.Getenv("CF_DNS_RESOLVERS"))
	if dnsResolvers == "" {
		dnsResolvers = strings.TrimSpace(os.Getenv("TUNNEL_DNS_RESOLVER_ADDRS"))
	}
	// 默认不强制注入境内公共 DNS，避免境内明文 UDP 递归解析 *.argotunnel.com 命中 GFW 假 IP 污染

	enabled := true
	if v := os.Getenv("CF_TUNNEL_ENABLED"); v != "" {
		vLower := strings.ToLower(v)
		enabled = (vLower == "1" || vLower == "true" || vLower == "yes")
	} else if v := os.Getenv("CLOUDFLARE_TUNNEL_ENABLED"); v != "" {
		vLower := strings.ToLower(v)
		enabled = (vLower == "1" || vLower == "true" || vLower == "yes")
	}

	edgeProbe := true
	switch strings.ToLower(strings.TrimSpace(os.Getenv("CF_EDGE_PROBE"))) {
	case "0", "false", "no", "off":
		edgeProbe = false
	}

	return CloudflareConfig{
		EdgeProbe:     edgeProbe,
		Enabled:       enabled,
		WorkerURL:     workerURL,
		InviteCode:    inviteCode,
		Token:         token,
		EdgeIPVersion: edgeIPVersion,
		Protocol:      protocol,
		Region:        region,
		DNSResolvers:  dnsResolvers,
	}
}

// GetTrustLAN reports whether LAN clients are trusted without requiring pairing tokens.
// Controlled via MULTIGRAVITY_TRUST_LAN, TRUST_LAN, or ALLOW_LAN. Defaults to false (secure pairing required).
func GetTrustLAN() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("MULTIGRAVITY_TRUST_LAN")))
	if v == "" {
		v = strings.ToLower(strings.TrimSpace(os.Getenv("TRUST_LAN")))
	}
	if v == "" {
		v = strings.ToLower(strings.TrimSpace(os.Getenv("ALLOW_LAN")))
	}
	return v == "1" || v == "true" || v == "yes"
}

// DefaultGatewayName returns a clean human-readable display name for this host gateway.
// Respects MGY_GATEWAY_NAME / MULTIGRAVITY_GATEWAY_NAME when explicitly set; otherwise derives from os.Hostname().
func DefaultGatewayName() string {
	raw := strings.TrimSpace(os.Getenv("MGY_GATEWAY_NAME"))
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("MULTIGRAVITY_GATEWAY_NAME"))
	}
	if raw == "" {
		if h, err := os.Hostname(); err == nil {
			raw = strings.TrimSpace(h)
		}
	}
	lower := strings.ToLower(raw)
	for _, suffix := range []string{".localdomain", ".local", ".lan", ".home"} {
		if strings.HasSuffix(lower, suffix) && len(raw) > len(suffix) {
			raw = raw[:len(raw)-len(suffix)]
			break
		}
	}
	raw = strings.TrimSpace(raw)
	runes := []rune(raw)
	if len(runes) > 24 {
		raw = string(runes[:24])
	}
	return raw
}

func envBoolAny(keys ...string) bool {
	for _, key := range keys {
		raw := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
		if raw == "" {
			continue
		}
		return raw == "1" || raw == "true" || raw == "yes" || raw == "on"
	}
	return false
}

// CustomRelayConfig holds settings for a self-hosted mgy-relay instance (BYO-Relay),
// bypassing the centralized dispatcher Worker and cloudflared completely.
type CustomRelayConfig struct {
	Endpoint  string // e.g. "wss://relay.example.com/_mgy_tunnel"
	Token     string // offline-signed mgyr1.* token
	PublicURL string // optional override e.g. "https://dev.example.com" (auto-derived from token domain if empty)
}

// Enabled reports whether a custom self-hosted relay endpoint and token are both configured.
func (c CustomRelayConfig) Enabled() bool {
	return strings.TrimSpace(c.Endpoint) != "" && strings.TrimSpace(c.Token) != ""
}

// GetCustomRelayConfig parses BYO-Relay environment variables.
func GetCustomRelayConfig() CustomRelayConfig {
	endpoint := strings.TrimSpace(os.Getenv("MGY_CUSTOM_RELAY_ENDPOINT"))
	if endpoint != "" {
		lower := strings.ToLower(endpoint)
		switch {
		case strings.HasPrefix(lower, "https://"):
			endpoint = "wss://" + endpoint[len("https://"):]
		case strings.HasPrefix(lower, "http://"):
			endpoint = "ws://" + endpoint[len("http://"):]
		case !strings.HasPrefix(lower, "wss://") && !strings.HasPrefix(lower, "ws://"):
			endpoint = "wss://" + endpoint
		}
		trimmed := strings.TrimRight(endpoint, "/")
		// Append default tunnel upgrade path if caller passed only scheme://host[:port].
		if schemeIdx := strings.Index(trimmed, "://"); schemeIdx >= 0 {
			afterScheme := trimmed[schemeIdx+3:]
			if !strings.Contains(afterScheme, "/") {
				endpoint = trimmed + "/_mgy_tunnel"
			}
		}
	}
	publicURL := strings.TrimRight(strings.TrimSpace(os.Getenv("MGY_CUSTOM_RELAY_PUBLIC_URL")), "/")
	if publicURL != "" && !strings.Contains(publicURL, "://") {
		publicURL = "https://" + publicURL
	}
	return CustomRelayConfig{
		Endpoint:  endpoint,
		Token:     strings.TrimSpace(os.Getenv("MGY_CUSTOM_RELAY_TOKEN")),
		PublicURL: publicURL,
	}
}

// GetRelayDisableWeb reports whether static Web/PWA shell assets (and /healthz e2ee_pub)
// should be blocked on relay/E2EE-enforced transports (MGY_RELAY_DISABLE_WEB=1).
func GetRelayDisableWeb() bool {
	return envBoolAny("MGY_RELAY_DISABLE_WEB", "MULTIGRAVITY_RELAY_DISABLE_WEB")
}

// GetForceE2EE reports whether external/proxied requests on the main HTTP server
// must be strictly gated by RelayGuardMiddleware (MGY_FORCE_E2EE=1).
func GetForceE2EE() bool {
	return envBoolAny("MGY_FORCE_E2EE", "MULTIGRAVITY_FORCE_E2EE")
}

// GetStrictE2EEReplay reports whether legacy E2EE requests without replay timestamps/IDs
// should be rejected (MGY_E2EE_STRICT_REPLAY=1).
func GetStrictE2EEReplay() bool {
	return envBoolAny("MGY_E2EE_STRICT_REPLAY", "MULTIGRAVITY_E2EE_STRICT_REPLAY")
}

// GetEnableIPv6 reports whether the gateway should bind a dual-stack IPv4/IPv6 TCP socket
// instead of the default IPv4-only ("tcp4") listener.
// Controlled via MULTIGRAVITY_ENABLE_IPV6 or MGY_ENABLE_IPV6. Defaults to false.
func GetEnableIPv6() bool {
	return envBoolAny("MULTIGRAVITY_ENABLE_IPV6", "MGY_ENABLE_IPV6")
}

// DefaultTLSPort is the default dedicated HTTPS listen port when native TLS cert/key are configured.
const DefaultTLSPort = 58943

// TLSConfig holds optional native HTTPS listener settings for direct DDNS / IPv6 access without a reverse proxy.
type TLSConfig struct {
	CertFile string
	KeyFile  string
	Port     int
}

// Enabled reports whether both certificate and private key paths are configured.
func (c TLSConfig) Enabled() bool {
	return strings.TrimSpace(c.CertFile) != "" && strings.TrimSpace(c.KeyFile) != ""
}

// GetTLSConfig parses optional native TLS listener settings from environment variables.
func GetTLSConfig() TLSConfig {
	certFile := strings.TrimSpace(os.Getenv("MGY_TLS_CERT"))
	if certFile == "" {
		certFile = strings.TrimSpace(os.Getenv("MULTIGRAVITY_TLS_CERT"))
	}
	keyFile := strings.TrimSpace(os.Getenv("MGY_TLS_KEY"))
	if keyFile == "" {
		keyFile = strings.TrimSpace(os.Getenv("MULTIGRAVITY_TLS_KEY"))
	}
	port := DefaultTLSPort
	for _, key := range []string{"MGY_TLS_PORT", "MULTIGRAVITY_TLS_PORT"} {
		if raw := strings.TrimSpace(os.Getenv(key)); raw != "" {
			if p, err := strconv.Atoi(raw); err == nil && p > 0 && p <= 65535 {
				port = p
				break
			}
		}
	}
	return TLSConfig{
		CertFile: certFile,
		KeyFile:  keyFile,
		Port:     port,
	}
}


