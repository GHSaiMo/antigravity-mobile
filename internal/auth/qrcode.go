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

// CompressHost strips the default domain suffix (e.g. .jiuge.space) from the host if present,
// reducing QR code density and avoiding direct exposure of the apex domain in URIs.
func CompressHost(h string) string {
	clean := strings.TrimSpace(h)
	lower := strings.ToLower(clean)
	if strings.HasSuffix(lower, DefaultDomainSuffix) && len(clean) > len(DefaultDomainSuffix) {
		return clean[:len(clean)-len(DefaultDomainSuffix)]
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
	IPv6Host    string
	DDNSHost    string
	RelayHost   string
}

// GenerateMultiHostPairingURI formats the pairing URI according to the agy:// schema specification
// embedding candidate network endpoints in standardized order:
// code, host (public/primary), lan (local), port, ssl, platform, followed by optional ipv6/ddns/relay.
func GenerateMultiHostPairingURI(p MultiHostPairingParams) string {
	cleanHost := strings.TrimSpace(p.PrimaryHost)
	if cleanHost == "" {
		cleanHost = "127.0.0.1"
	}

	displayHost := CompressHost(cleanHost)

	sslVal := "0"
	if p.SSL {
		sslVal = "1"
	}

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
	// 4. port
	items = append(items, paramItem{"port", fmt.Sprintf("%d", p.Port)})
	// 5. ssl
	items = append(items, paramItem{"ssl", sslVal})
	// 6. platform
	items = append(items, paramItem{"platform", runtime.GOOS})

	// Optional endpoints
	if ipv6 := strings.TrimSpace(p.IPv6Host); ipv6 != "" {
		items = append(items, paramItem{"ipv6", ipv6})
	}
	if ddns := strings.TrimSpace(p.DDNSHost); ddns != "" && ddns != cleanHost {
		items = append(items, paramItem{"ddns", ddns})
	}
	if relay := strings.TrimSpace(p.RelayHost); relay != "" && relay != cleanHost {
		items = append(items, paramItem{"relay", relay})
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
// candidate network endpoints (LAN IPv4, IPv6, DDNS, Cloud Relay) from primaryHost and extraHosts.
func BuildMultiHostPairingParams(primaryHost string, port int, code string, ssl bool, extraHosts ...string) MultiHostPairingParams {
	var lanHost, ipv6Host, ddnsHost, relayHost string

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
		if h == "" || h == "127.0.0.1" || h == "localhost" {
			return
		}
		if strings.Contains(h, ":") {
			if ipv6Host == "" {
				ipv6Host = h
			}
		} else if strings.Count(h, ".") == 3 && !strings.ContainsAny(h, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ") {
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
		IPv6Host:    ipv6Host,
		DDNSHost:    ddnsHost,
		RelayHost:   relayHost,
	}
}

// FormatPairingQRCode renders the complete pairing banner, ANSI QR code, and URI instructions
// into a single formatted string.
func FormatPairingQRCode(primaryHost string, port int, code string, ssl bool, extraHosts ...string) string {
	params := BuildMultiHostPairingParams(primaryHost, port, code, ssl, extraHosts...)
	uri := GenerateMultiHostPairingURI(params)

	var b strings.Builder

	qr, err := qrcode.New(uri, qrcode.Medium)
	if err != nil {
		fmt.Fprintf(&b, "\n⚠️  无法生成配对二维码: %v\n🔗 配对链接: %s\n\n", err, uri)
		return b.String()
	}

	b.WriteString("\n  📱 Multigravity 客户端扫码一键配对 (5分钟内有效)\n")
	qrStr := qr.ToSmallString(false)
	b.WriteString(qrStr)
	if !strings.HasSuffix(qrStr, "\n") {
		b.WriteString("\n")
	}
	b.WriteString("  请使用 Multigravity 手机客户端扫描上方二维码\n")

	if ssl || (strings.Contains(primaryHost, ".") && strings.ContainsAny(primaryHost, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")) {
		b.WriteString("  ☁️  Cloudflare 专属域名已生成\n")
		fmt.Fprintf(&b, "  ☁️  公网配对 URI: %s\n", uri)
	} else {
		scheme := "http://"
		if ssl {
			scheme = "https://"
		}
		if port != 80 && port != 443 {
			fmt.Fprintf(&b, "  🌐 访问地址: %s%s:%d\n", scheme, primaryHost, port)
		} else {
			fmt.Fprintf(&b, "  🌐 访问地址: %s%s\n", scheme, primaryHost)
		}
	}

	if params.LANHost != "" {
		lanURI := GeneratePairingURI(params.LANHost, 58900, code, false)
		fmt.Fprintf(&b, "  🏠 局域网 Wi-Fi 直连 URI: %s\n", lanURI)
	}
	b.WriteString("\n")

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

// PrintRawPairingQRCode prints the pairing banner, QR code, and instructions for a given URI and code.
func PrintRawPairingQRCode(code string, uri string) {
	var b strings.Builder
	qr, err := qrcode.New(uri, qrcode.Medium)
	if err != nil {
		fmt.Fprintf(&b, "\n⚠️  无法生成配对二维码: %v\n🔗 配对链接: %s\n\n", err, uri)
	} else {
		b.WriteString("\n  📱 Multigravity 客户端扫码一键配对 (5分钟内有效)\n")
		if code != "" {
			fmt.Fprintf(&b, "  🔑 配对码: %s\n", code)
		}
		qrStr := qr.ToSmallString(false)
		b.WriteString(qrStr)
		if !strings.HasSuffix(qrStr, "\n") {
			b.WriteString("\n")
		}
		b.WriteString("  请使用 Multigravity 手机客户端扫描上方二维码\n")

		if u, err := url.Parse(uri); err == nil {
			q := u.Query()
			h := q.Get("host")
			pStr := q.Get("port")
			pVal, _ := strconv.Atoi(pStr)
			sVal := q.Get("ssl") == "1"
			lan := q.Get("lan")

			if sVal || (strings.Contains(h, ".") && strings.ContainsAny(h, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")) {
				b.WriteString("  ☁️  Cloudflare 专属域名已生成\n")
				fmt.Fprintf(&b, "  ☁️  公网配对 URI: %s\n", uri)
			} else if h != "" {
				scheme := "http://"
				if sVal {
					scheme = "https://"
				}
				if pVal != 80 && pVal != 443 {
					fmt.Fprintf(&b, "  🌐 访问地址: %s%s:%d\n", scheme, h, pVal)
				} else {
					fmt.Fprintf(&b, "  🌐 访问地址: %s%s\n", scheme, h)
				}
			}

			if lan != "" {
				fmt.Fprintf(&b, "  🏠 局域网 Wi-Fi 直连 URI: %s\n", GeneratePairingURI(lan, 58900, code, false))
			}
		}
		b.WriteString("\n")
	}

	consoleMu.Lock()
	defer consoleMu.Unlock()
	_, _ = os.Stdout.WriteString(b.String())
}

