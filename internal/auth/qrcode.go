package auth

import (
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
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
func InitConsoleSync() {
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
// embedding multiple candidate network endpoints (LAN, IPv6, DDNS, Relay).
func GenerateMultiHostPairingURI(p MultiHostPairingParams) string {
	cleanHost := strings.TrimSpace(p.PrimaryHost)
	if cleanHost == "" {
		cleanHost = "127.0.0.1"
	}

	sslVal := "0"
	if p.SSL {
		sslVal = "1"
	}

	params := url.Values{}
	params.Set("host", cleanHost)
	params.Set("port", fmt.Sprintf("%d", p.Port))
	params.Set("code", p.Code)
	params.Set("ssl", sslVal)

	if lan := strings.TrimSpace(p.LANHost); lan != "" && lan != cleanHost {
		params.Set("lan", lan)
	}
	if ipv6 := strings.TrimSpace(p.IPv6Host); ipv6 != "" && ipv6 != cleanHost {
		params.Set("ipv6", ipv6)
	}
	if ddns := strings.TrimSpace(p.DDNSHost); ddns != "" && ddns != cleanHost {
		params.Set("ddns", ddns)
	}
	if relay := strings.TrimSpace(p.RelayHost); relay != "" && relay != cleanHost {
		params.Set("relay", relay)
	}

	return fmt.Sprintf("agy://pair?%s", params.Encode())
}

// GeneratePairingURI formats the pairing URI according to the agy:// schema specification.
// Format: agy://pair?host=<MAC_HOST>&port=<PORT>&code=<PAIRING_CODE>&ssl=1
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
				if relayHost == "" {
					relayHost = h
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

	b.WriteString("\n==================================================\n")
	b.WriteString("📱 Antigravity Mobile 客户端扫码一键配对\n")
	b.WriteString("==================================================\n")
	qrStr := qr.ToSmallString(false)
	b.WriteString(qrStr)
	if !strings.HasSuffix(qrStr, "\n") {
		b.WriteString("\n")
	}
	b.WriteString("请使用 Antigravity 手机客户端扫描上方二维码 (5分钟内有效)\n\n")
	fmt.Fprintf(&b, "🔗 复合配对 URI:          %s\n", uri)

	if params.LANHost != "" {
		lanURI := GeneratePairingURI(params.LANHost, port, code, ssl)
		fmt.Fprintf(&b, "🏠 局域网 Wi-Fi 直连 URI: %s\n", lanURI)
	}
	if params.IPv6Host != "" {
		ipv6URI := GeneratePairingURI(params.IPv6Host, port, code, ssl)
		fmt.Fprintf(&b, "🌐 外网 IPv6 直连 URI:   %s\n", ipv6URI)
	}
	if params.RelayHost != "" {
		relayURI := GeneratePairingURI(params.RelayHost, port, code, ssl)
		fmt.Fprintf(&b, "☁️ 云服务器中继 URI:     %s\n", relayURI)
	}
	if params.DDNSHost != "" {
		ddnsURI := GeneratePairingURI(params.DDNSHost, port, code, ssl)
		fmt.Fprintf(&b, "⚡ DDNS / 域名直连 URI:  %s\n", ddnsURI)
	}

	b.WriteString("\n💡 提示: 扫码会自动同步局域网、IPv6 与云服务器中继网址，局域网极速秒连，外网智能自适应。\n")
	b.WriteString("==================================================\n\n")

	return b.String()
}

// PrintPairingQRCode generates and renders an ANSI QR code to stdout encoding all candidate
// network endpoints (e.g. public IPv6, LAN IPv4, Cloud Relay), and displays informative pairing instructions.
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
