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

	if lan := strings.TrimSpace(p.LANHost); lan != "" && (lan != cleanHost || strings.Contains(cleanHost, ":")) {
		params.Set("lan", lan)
	}
	if ipv6 := strings.TrimSpace(p.IPv6Host); ipv6 != "" {
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

	b.WriteString("\n==================================================\n")
	b.WriteString("📱 Multigravity 客户端扫码一键配对\n")
	b.WriteString("==================================================\n")
	qrStr := qr.ToSmallString(false)
	b.WriteString(qrStr)
	if !strings.HasSuffix(qrStr, "\n") {
		b.WriteString("\n")
	}
	b.WriteString("请使用 Multigravity 手机客户端扫描上方二维码 (5分钟内有效)\n\n")
	fmt.Fprintf(&b, "🔗 复合配对 URI:          %s\n", uri)

	if params.LANHost != "" {
		lanURI := GeneratePairingURI(params.LANHost, port, code, ssl)
		fmt.Fprintf(&b, "🏠 局域网 Wi-Fi 直连 URI: %s\n", lanURI)
	}
	if params.IPv6Host != "" {
		ipv6URI := GeneratePairingURI(params.IPv6Host, port, code, ssl)
		fmt.Fprintf(&b, "🌐 外网 IPv6 直连 URI:   %s\n", ipv6URI)
		appendIPv6Instructions(&b, params.IPv6Host, port, ssl)
	} else {
		appendIPv6DisabledInstructions(&b)
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

func appendIPv6DisabledInstructions(b *strings.Builder) {
	b.WriteString("\n--------------------------------------------------\n")
	b.WriteString("💡 IPv6 外网直连提示:\n")
	switch runtime.GOOS {
	case "windows":
		b.WriteString("   当前未检测到公网 IPv6 地址。若需在外网 5G/4G 随时随地直连 Windows 电脑:\n")
		b.WriteString("   1. 检查 Windows「设置 -> 网络和 Internet」中当前网络连接属性，确保已勾选启用「Internet 协议版本 6 (TCP/IPv6)」；\n")
		b.WriteString("   2. 确保家中光猫或主路由器已开启 IPv6 分配（SLAAC/DHCPv6）；\n")
		b.WriteString("   3. 获取到 IPv6 后重新运行 mgy，将自动优先使用公网 IPv6 写入二维码！\n")
	case "darwin":
		b.WriteString("   当前未检测到公网 IPv6 地址。若需在外网 5G/4G 随时随地直连 Mac:\n")
		b.WriteString("   1. 检查 Mac「系统设置 -> 网络 -> TCP/IP」中「配置 IPv6」是否已设为「自动」；\n")
		b.WriteString("   2. 确保家中光猫或主路由器已开启 IPv6 分配（SLAAC/DHCPv6）；\n")
		b.WriteString("   3. 获取到 IPv6 后重新运行 mgy，将自动优先使用公网 IPv6 写入二维码！\n")
	default:
		b.WriteString("   当前未检测到公网 IPv6 地址。若需在外网 5G/4G 随时随地直连当前设备:\n")
		b.WriteString("   1. 检查系统网络接口配置，确保已启用 IPv6 自动获取；\n")
		b.WriteString("   2. 确保家中光猫或主路由器已开启 IPv6 分配（SLAAC/DHCPv6）；\n")
		b.WriteString("   3. 获取到 IPv6 后重新运行 mgy，将自动优先使用公网 IPv6 写入二维码！\n")
	}
}

func appendIPv6Instructions(b *strings.Builder, ipv6Host string, port int, ssl bool) {
	if ipv6Host == "" {
		return
	}
	scheme := "http"
	if ssl {
		scheme = "https"
	}
	testURL := fmt.Sprintf("%s://[%s]:%d", scheme, ipv6Host, port)
	b.WriteString("\n--------------------------------------------------\n")
	b.WriteString("📱 移动端 5G/4G 外网直连验证指引:\n")
	b.WriteString("   1. 手机断开家中 Wi-Fi（切换至 5G/4G 移动蜂窝网络）；\n")
	fmt.Fprintf(b, "   2. 手机自带浏览器直接访问测试地址:\n      %s\n", testURL)
	b.WriteString("   3. 验收标准与排错说明:\n")
	b.WriteString("      • 正常打开网页: 家中光猫/主路由已放行 IPv6 入站流量，外网直连完全畅通！\n")
	b.WriteString("      • 访问超时/无法连接: 通常因家用光猫或路由器开启了『IPv6 防火墙入站阻断』。\n")
	if runtime.GOOS == "windows" {
		b.WriteString("        解决办法: 1) 登录光猫/主路由管理后台，关闭 IPv6 防火墙或添加 58900 端口放行；\n")
		b.WriteString("                 2) 检查 Windows Defender 防火墙是否允许 mgy.exe 入站连接。\n")
	} else {
		b.WriteString("        解决办法: 登录光猫/主路由管理后台，关闭 IPv6 防火墙或添加 58900 端口放行即可。\n")
	}
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

// PrintRawPairingQRCode prints the pairing banner, QR code, and instructions for a given URI and code.
func PrintRawPairingQRCode(code string, uri string) {
	var b strings.Builder
	qr, err := qrcode.New(uri, qrcode.Medium)
	if err != nil {
		fmt.Fprintf(&b, "\n⚠️  无法生成配对二维码: %v\n🔗 配对链接: %s\n\n", err, uri)
	} else {
		b.WriteString("\n==================================================\n")
		b.WriteString("📱 Multigravity 客户端扫码一键配对\n")
		b.WriteString("==================================================\n")
		if code != "" {
			fmt.Fprintf(&b, "🔑 配对码 (5分钟有效):\n   %s\n\n", code)
		}
		qrStr := qr.ToSmallString(false)
		b.WriteString(qrStr)
		if !strings.HasSuffix(qrStr, "\n") {
			b.WriteString("\n")
		}
		b.WriteString("请使用 Multigravity 手机客户端扫描上方二维码 (5分钟内有效)\n\n")
		fmt.Fprintf(&b, "🔗 复合配对 URI:          %s\n", uri)

		if u, err := url.Parse(uri); err == nil {
			q := u.Query()
			h := q.Get("host")
			pStr := q.Get("port")
			pVal, _ := strconv.Atoi(pStr)
			sVal := q.Get("ssl") == "1"
			lan := q.Get("lan")
			ipv6 := q.Get("ipv6")
			relay := q.Get("relay")
			ddns := q.Get("ddns")

			if lan != "" {
				fmt.Fprintf(&b, "🏠 局域网 Wi-Fi 直连 URI: %s\n", GeneratePairingURI(lan, pVal, code, sVal))
			} else if h != "" && !strings.Contains(h, ":") {
				fmt.Fprintf(&b, "🏠 局域网 Wi-Fi 直连 URI: %s\n", GeneratePairingURI(h, pVal, code, sVal))
			}
			if ipv6 != "" {
				fmt.Fprintf(&b, "🌐 外网 IPv6 直连 URI:   %s\n", GeneratePairingURI(ipv6, pVal, code, sVal))
				appendIPv6Instructions(&b, ipv6, pVal, sVal)
			} else if strings.Contains(h, ":") {
				fmt.Fprintf(&b, "🌐 外网 IPv6 直连 URI:   %s\n", GeneratePairingURI(h, pVal, code, sVal))
				appendIPv6Instructions(&b, h, pVal, sVal)
			} else {
				appendIPv6DisabledInstructions(&b)
			}
			if ddns != "" {
				fmt.Fprintf(&b, "⚡ DDNS / 域名直连 URI:  %s\n", GeneratePairingURI(ddns, pVal, code, sVal))
			}
			if relay != "" {
				fmt.Fprintf(&b, "☁️  云服务器中继 URI:     %s\n", GeneratePairingURI(relay, pVal, code, sVal))
			}
		}

		b.WriteString("\n💡 提示: 扫码会自动同步局域网、IPv6 与云服务器中继网址，局域网极速秒连，外网智能自适应。\n")
		b.WriteString("==================================================\n\n")
	}

	consoleMu.Lock()
	defer consoleMu.Unlock()
	_, _ = os.Stdout.WriteString(b.String())
}

