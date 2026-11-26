package auth

import (
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"antigravity-mobile/internal/i18n"

	"github.com/skip2/go-qrcode"
)

var consoleMu sync.Mutex

type synchronizedWriter struct {
	mu *sync.Mutex
	w  io.Writer
}

func (sw *synchronizedWriter) Write(p []byte) (n int, err error) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	return sw.w.Write(p)
}

// InitConsoleSync configures the standard logger to synchronize with terminal QR code output,
// preventing concurrent background log messages from tearing or cutting into the QR code.
// On Windows, it also switches console code page to UTF-8 and enables virtual terminal processing.
func InitConsoleSync() {
	initConsoleOS()
	log.SetFlags(log.Ltime)
	cur := log.Writer()
	log.SetOutput(&synchronizedWriter{
		mu: &consoleMu,
		w:  cur,
	})
}

// ConsoleLock acquires the console lock to safely print uninterrupted text.
func ConsoleLock() func() {
	consoleMu.Lock()
	return consoleMu.Unlock
}

const DefaultDomainSuffix = ".jiuge.space"

var (
	defaultE2EEPubMu  sync.RWMutex
	defaultE2EEPubKey string

	customRelayHostsMu sync.RWMutex
	customRelayHosts   = map[string]bool{}
)

// RegisterRelayHost records a custom BYO-Relay hostname so QR banners render E2EE labels accurately.
func RegisterRelayHost(host string) {
	h := strings.ToLower(strings.TrimSpace(host))
	if h == "" {
		return
	}
	customRelayHostsMu.Lock()
	customRelayHosts[h] = true
	customRelayHostsMu.Unlock()
}

// SetDefaultE2EEPubKey registers the gateway's P-256 public key so generated pairing QR URIs embed `&pk=...`.
func SetDefaultE2EEPubKey(pubB64 string) {
	defaultE2EEPubMu.Lock()
	defer defaultE2EEPubMu.Unlock()
	defaultE2EEPubKey = strings.TrimSpace(pubB64)
}

func getDefaultE2EEPubKey() string {
	defaultE2EEPubMu.RLock()
	defer defaultE2EEPubMu.RUnlock()
	return defaultE2EEPubKey
}

// CompressHost strips the default domain suffix (e.g. .jiuge.space) from the host if present,
// reducing QR code density and avoiding direct exposure of the apex domain in URIs.
func CompressHost(h string) string {
	clean := strings.TrimSpace(h)
	lower := strings.ToLower(clean)
	for _, suffix := range []string{DefaultDomainSuffix, ".mgy.jiuge.space"} {
		if strings.HasSuffix(lower, suffix) && len(clean) > len(suffix) {
			prefix := clean[:len(clean)-len(suffix)]
			// Only compress if the prefix does NOT contain any dots (i.e. single-level subdomain like "825a5a50").
			if !strings.Contains(prefix, ".") {
				return prefix
			}
		}
	}
	return clean
}

// ExpandHost restores the default domain suffix if host is a compressed subdomain without dots.
func ExpandHost(h string) string {
	clean := strings.TrimSpace(h)
	if clean != "" && !strings.Contains(clean, ".") && !strings.Contains(clean, ":") && strings.ToLower(clean) != "localhost" {
		return clean + DefaultDomainSuffix
	}
	return clean
}

// MultiHostPairingParams specifies parameters for generating multi-endpoint pairing URIs.
type MultiHostPairingParams struct {
	PrimaryHost string
	Port        int
	Code        string
	SSL         bool
	LANHost     string
	DDNSHost    string
	RelayHost   string
	E2EEPubKey  string
}

// GenerateMultiHostPairingURI formats the pairing URI according to the agy:// schema specification
// embedding candidate network endpoints in standardized order:
// code, host (public/primary), lan (local), platform, followed by port/ssl only if non-default.
func GenerateMultiHostPairingURI(p MultiHostPairingParams) string {
	cleanHost := strings.TrimSpace(p.PrimaryHost)
	if cleanHost == "" {
		cleanHost = "127.0.0.1"
	}

	displayHost := CompressHost(cleanHost)

	type paramItem struct {
		key   string
		value string
	}

	var items []paramItem
	// 1. code
	items = append(items, paramItem{"code", p.Code})
	// 2. host (public / primary)
	items = append(items, paramItem{"host", displayHost})
	// 3. lan (host局域网, if present and distinct from primary host)
	if lan := strings.TrimSpace(p.LANHost); lan != "" && (lan != cleanHost || strings.Contains(cleanHost, ":")) {
		items = append(items, paramItem{"lan", lan})
	}
	// 4. port (omitted if default 443)
	if p.Port != 0 && p.Port != 443 {
		items = append(items, paramItem{"port", fmt.Sprintf("%d", p.Port)})
	}
	// 5. ssl (omitted if default: public 443 is SSL=1, non-443 is SSL=0)
	if !p.SSL && p.Port == 443 {
		items = append(items, paramItem{"ssl", "0"})
	} else if p.SSL && p.Port != 443 {
		items = append(items, paramItem{"ssl", "1"})
	}
	// 6. platform
	items = append(items, paramItem{"platform", runtime.GOOS})

	// Optional endpoints
	if ddns := strings.TrimSpace(p.DDNSHost); ddns != "" && ddns != cleanHost {
		items = append(items, paramItem{"ddns", ddns})
	}
	if relay := strings.TrimSpace(p.RelayHost); relay != "" && relay != cleanHost {
		items = append(items, paramItem{"relay", relay})
	}
	pk := strings.TrimSpace(p.E2EEPubKey)
	if pk == "" {
		pk = getDefaultE2EEPubKey()
	}
	if pk != "" {
		items = append(items, paramItem{"pk", pk})
	}

	var pairs []string
	for _, it := range items {
		pairs = append(pairs, fmt.Sprintf("%s=%s", url.QueryEscape(it.key), url.QueryEscape(it.value)))
	}

	return fmt.Sprintf("agy://pair?%s", strings.Join(pairs, "&"))
}

// GeneratePairingURI formats the pairing URI according to the agy:// schema specification.
// Format: agy://pair?code=<PAIRING_CODE>&host=<HOST>&port=<PORT>&ssl=1&platform=<PLATFORM>
func GeneratePairingURI(host string, port int, code string, ssl bool) string {
	return GenerateMultiHostPairingURI(MultiHostPairingParams{
		PrimaryHost: host,
		Port:        port,
		Code:        code,
		SSL:         ssl,
	})
}

// BuildMultiHostPairingParams constructs MultiHostPairingParams by automatically classifying
// candidate network endpoints (LAN IPv4, DDNS, Cloud Relay) from primaryHost and extraHosts.
func BuildMultiHostPairingParams(primaryHost string, port int, code string, ssl bool, extraHosts ...string) MultiHostPairingParams {
	var lanHost, ddnsHost, relayHost string

	isPrivateIPv4 := func(ipStr string) bool {
		if strings.HasPrefix(ipStr, "192.168.") || strings.HasPrefix(ipStr, "10.") {
			return true
		}
		if strings.HasPrefix(ipStr, "172.") {
			parts := strings.Split(ipStr, ".")
			if len(parts) >= 2 {
				if n, err := strconv.Atoi(parts[1]); err == nil && n >= 16 && n <= 31 {
					return true
				}
			}
		}
		return false
	}

	classifyHost := func(h string) {
		h = strings.TrimSpace(h)
		if h == "" || h == "127.0.0.1" || h == "localhost" || strings.Contains(h, ":") {
			return
		}
		if strings.Count(h, ".") == 3 && !strings.ContainsAny(h, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ") {
			if isPrivateIPv4(h) {
				if lanHost == "" {
					lanHost = h
				}
			} else {
				// Only treat routable public IPv4 as relayHost, skip Fake-IP (198.18.x.x) and APIPA (169.254.x.x)
				if !strings.HasPrefix(h, "198.18.") && !strings.HasPrefix(h, "198.19.") &&
					!strings.HasPrefix(h, "169.254.") && !strings.HasPrefix(h, "127.") {
					if relayHost == "" {
						relayHost = h
					}
				}
			}
		} else {
			if ddnsHost == "" {
				ddnsHost = h
			}
		}
	}

	classifyHost(primaryHost)
	for _, eh := range extraHosts {
		classifyHost(eh)
	}

	return MultiHostPairingParams{
		PrimaryHost: primaryHost,
		Port:        port,
		Code:        code,
		SSL:         ssl,
		LANHost:     lanHost,
		DDNSHost:    ddnsHost,
		RelayHost:   relayHost,
	}
}

func qrSupportsANSI() bool {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("TERM")), "dumb") {
		return false
	}
	return consoleSupportsANSI()
}

// renderTerminalQRCode renders qr as compact half-height terminal output using only
// ASCII space (' ') and LOWER HALF BLOCK ('▄', U+2584) with explicit ANSI black/bright-white
// foreground and background colors.
//
// go-qrcode's ToSmallString emits UPPER HALF BLOCK ('▀', U+2580), which is absent from the
// GBK charset and therefore missing from Simplified Chinese Windows' legacy conhost.exe
// default font (NSimSun / 新宋体), turning ~25% of modules and the entire bottom border into
// hollow missing-glyph boxes ('▯') and making the QR code unscannable. Using '▄' (present
// in GBK / NSimSun) with inverted ANSI colors eliminates '▀' completely while also ensuring
// high-contrast black-on-white rendering on both dark and light terminal themes.
func renderTerminalQRCode(qr *qrcode.QRCode) string {
	if qr == nil {
		return ""
	}
	if !qrSupportsANSI() {
		return qr.ToSmallString(false)
	}
	return renderANSIHalfBlockQR(qr.Bitmap())
}

func renderANSIHalfBlockQR(bits [][]bool) string {
	if len(bits) == 0 {
		return ""
	}
	const (
		sgrReset = "\x1b[0m"
		sgrWW    = "\x1b[107m"    // white top + white bottom (bright white bg + space)
		sgrBB    = "\x1b[40m"     // black top + black bottom (black bg + space)
		sgrBW    = "\x1b[40;97m"  // black top + white bottom (black bg + bright white '▄')
		sgrWB    = "\x1b[107;30m" // white top + black bottom (bright white bg + black '▄')
	)

	var b strings.Builder
	for y := 0; y < len(bits); y += 2 {
		topRow := bits[y]
		var botRow []bool
		if y+1 < len(bits) {
			botRow = bits[y+1]
		}

		lastSGR := ""
		for x := range topRow {
			topBlack := topRow[x]
			botBlack := false // odd final row pairs with white quiet-zone padding
			if x < len(botRow) {
				botBlack = botRow[x]
			}

			var sgr, ch string
			switch {
			case !topBlack && !botBlack:
				sgr, ch = sgrWW, " "
			case topBlack && botBlack:
				sgr, ch = sgrBB, " "
			case topBlack && !botBlack:
				sgr, ch = sgrBW, "▄"
			default: // !topBlack && botBlack
				sgr, ch = sgrWB, "▄"
			}

			if sgr != lastSGR {
				b.WriteString(sgr)
				lastSGR = sgr
			}
			b.WriteString(ch)
		}
		if lastSGR != "" {
			b.WriteString(sgrReset)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// FormatPairingQRCode renders the complete pairing banner, ANSI QR code, and URI instructions
// into a single formatted string.
func FormatPairingQRCode(primaryHost string, port int, code string, ssl bool, extraHosts ...string) string {
	params := BuildMultiHostPairingParams(primaryHost, port, code, ssl, extraHosts...)
	uri := GenerateMultiHostPairingURI(params)

	var b strings.Builder

	qr, err := qrcode.New(uri, qrcode.Medium)
	if err != nil {
		fmt.Fprintf(&b, i18n.T("\n⚠️  无法生成配对二维码: %v\n🔗 配对链接: %s\n\n",
			"\n⚠️  Failed to generate pairing QR code: %v\n🔗 Pairing URL: %s\n\n"), err, uri)
		return b.String()
	}

	b.WriteString(i18n.T(
		"\n  📱 Multigravity 客户端扫码一键配对 (5分钟内有效)\n",
		"\n  📱 Multigravity Mobile One-Tap Pairing (Valid for 5 mins)\n",
	))
	qrStr := renderTerminalQRCode(qr)
	b.WriteString(qrStr)
	if !strings.HasSuffix(qrStr, "\n") {
		b.WriteString("\n")
	}
	b.WriteString(i18n.T(
		"  请使用 Multigravity 手机客户端扫描上方二维码\n",
		"  Scan the QR code above using Multigravity mobile app\n",
	))

	if ssl || (strings.Contains(primaryHost, ".") && strings.ContainsAny(primaryHost, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")) {
		if isRelayDomainLabel(primaryHost) {
			b.WriteString(i18n.T(
				"  🔐 E2EE 端到端加密专属通道已就绪\n",
				"  🔐 E2EE exclusive HTTPS relay ready\n",
			))
		} else {
			b.WriteString(i18n.T(
				"  ☁️  Cloudflare 专属域名已就绪\n",
				"  ☁️  Cloudflare exclusive HTTPS domain ready\n",
			))
		}
	} else {
		scheme := "http://"
		if ssl {
			scheme = "https://"
		}
		if port != 80 && port != 443 && port != 0 {
			fmt.Fprintf(&b, i18n.T("  🌐 访问地址: %s%s:%d\n", "  🌐 Access URL: %s%s:%d\n"), scheme, primaryHost, port)
		} else {
			fmt.Fprintf(&b, i18n.T("  🌐 访问地址: %s%s\n", "  🌐 Access URL: %s%s\n"), scheme, primaryHost)
		}
	}

	fmt.Fprintf(&b, i18n.T("  🔗 配对 URI: %s\n\n", "  🔗 Pairing URI: %s\n\n"), uri)
	return b.String()
}

// PrintPairingQRCode generates and renders an ANSI QR code to stdout encoding all candidate
// network endpoints (e.g. Cloudflare, LAN IPv4), and displays informative pairing instructions.
// It executes atomically under console synchronization to prevent concurrent log statements from corrupting the QR code.
func PrintPairingQRCode(primaryHost string, port int, code string, ssl bool, extraHosts ...string) {
	output := FormatPairingQRCode(primaryHost, port, code, ssl, extraHosts...)

	consoleMu.Lock()
	defer consoleMu.Unlock()

	_, _ = os.Stdout.WriteString(output)
}

// GenerateMultiHostQRCodePNG generates a PNG byte slice for the given multi-host pairing parameters.
func GenerateMultiHostQRCodePNG(p MultiHostPairingParams, size int) ([]byte, error) {
	if size <= 0 {
		size = 256
	}
	uri := GenerateMultiHostPairingURI(p)
	return qrcode.Encode(uri, qrcode.Medium, size)
}

// GenerateQRCodePNG generates a PNG byte slice for the pairing URI, supporting multi-endpoint resolution via extraHosts.
func GenerateQRCodePNG(host string, port int, code string, ssl bool, size int, extraHosts ...string) ([]byte, error) {
	if size <= 0 {
		size = 256
	}
	params := BuildMultiHostPairingParams(host, port, code, ssl, extraHosts...)
	return GenerateMultiHostQRCodePNG(params, size)
}

func formatRawPairingQRCode(code string, uri string) string {
	var b strings.Builder
	qr, err := qrcode.New(uri, qrcode.Medium)
	if err != nil {
		fmt.Fprintf(&b, i18n.T("\n⚠️  无法生成配对二维码: %v\n🔗 配对链接: %s\n\n",
			"\n⚠️  Failed to generate pairing QR code: %v\n🔗 Pairing URL: %s\n\n"), err, uri)
	} else {
		b.WriteString(i18n.T(
			"\n  📱 Multigravity 客户端扫码一键配对 (5分钟内有效)\n",
			"\n  📱 Multigravity Mobile One-Tap Pairing (Valid for 5 mins)\n",
		))
		if code != "" {
			fmt.Fprintf(&b, i18n.T("  🔑 配对码: %s\n", "  🔑 Pairing Code: %s\n"), code)
		}
		qrStr := renderTerminalQRCode(qr)
		b.WriteString(qrStr)
		if !strings.HasSuffix(qrStr, "\n") {
			b.WriteString("\n")
		}
		b.WriteString(i18n.T(
			"  请使用 Multigravity 手机客户端扫描上方二维码\n",
			"  Scan the QR code above using Multigravity mobile app\n",
		))

		if u, err := url.Parse(uri); err == nil {
			q := u.Query()
			h := q.Get("host")
			pStr := q.Get("port")
			pVal, _ := strconv.Atoi(pStr)
			sVal := q.Get("ssl") == "1" || (q.Get("ssl") == "" && (pVal == 443 || pVal == 0))

			if sVal || (strings.Contains(h, ".") && strings.ContainsAny(h, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")) {
				if isRelayDomainLabel(h) {
					b.WriteString(i18n.T(
						"  🔐 E2EE 端到端加密专属通道已就绪\n",
						"  🔐 E2EE exclusive HTTPS relay ready\n",
					))
				} else {
					b.WriteString(i18n.T(
						"  ☁️  Cloudflare 专属域名已就绪\n",
						"  ☁️  Cloudflare exclusive HTTPS domain ready\n",
					))
				}
			} else if h != "" {
				scheme := "http://"
				if sVal {
					scheme = "https://"
				}
				if pVal != 80 && pVal != 443 && pVal != 0 {
					fmt.Fprintf(&b, i18n.T("  🌐 访问地址: %s%s:%d\n", "  🌐 Access URL: %s%s:%d\n"), scheme, h, pVal)
				} else {
					fmt.Fprintf(&b, i18n.T("  🌐 访问地址: %s%s\n", "  🌐 Access URL: %s%s\n"), scheme, h)
				}
			}
		}
		fmt.Fprintf(&b, i18n.T("  🔗 配对 URI: %s\n\n", "  🔗 Pairing URI: %s\n\n"), uri)
	}
	return b.String()
}

// PrintRawPairingQRCode prints the pairing banner, QR code, and instructions for a given URI and code.
func PrintRawPairingQRCode(code string, uri string) {
	output := formatRawPairingQRCode(code, uri)

	consoleMu.Lock()
	defer consoleMu.Unlock()
	_, _ = os.Stdout.WriteString(output)
}

// isRelayDomainLabel reports whether host (full FQDN or compressed subdomain)
// belongs to an E2EE VPS relay endpoint (e.g. "r-541ba9e5", "r-541ba9e5.jiuge.space",
// or "r-541ba9e5.tokyo.jiuge.space") rather than a legacy Cloudflare Tunnel subdomain.
func isRelayDomainLabel(host string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	if strings.HasPrefix(h, "r-") || strings.Contains(h, ".relay.") {
		return true
	}
	customRelayHostsMu.RLock()
	matched := customRelayHosts[h]
	customRelayHostsMu.RUnlock()
	return matched
}

