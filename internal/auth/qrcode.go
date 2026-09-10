package auth

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/skip2/go-qrcode"
)

// GeneratePairingURI formats the pairing URI according to the agy:// schema specification.
// Format: agy://pair?host=<MAC_HOST>&port=<PORT>&code=<PAIRING_CODE>&ssl=1
func GeneratePairingURI(host string, port int, code string, ssl bool) string {
	cleanHost := strings.TrimSpace(host)
	if cleanHost == "" {
		cleanHost = "127.0.0.1"
	}

	sslVal := "0"
	if ssl {
		sslVal = "1"
	}

	params := url.Values{}
	params.Set("host", cleanHost)
	params.Set("port", fmt.Sprintf("%d", port))
	params.Set("code", code)
	params.Set("ssl", sslVal)

	return fmt.Sprintf("agy://pair?%s", params.Encode())
}

// PrintPairingQRCode generates and renders an ANSI QR code to stdout for the primary address,
// and optionally displays additional network URIs (e.g. public IPv6, LAN IPv4).
func PrintPairingQRCode(primaryHost string, port int, code string, ssl bool, extraHosts ...string) {
	uri := GeneratePairingURI(primaryHost, port, code, ssl)

	qr, err := qrcode.New(uri, qrcode.Medium)
	if err != nil {
		fmt.Printf("⚠️  无法生成配对二维码: %v\n", err)
		fmt.Printf("🔗 配对链接: %s\n", uri)
		return
	}

	fmt.Println()
	fmt.Println("==================================================")
	fmt.Println("📱 Antigravity Mobile 客户端扫码配对")
	fmt.Println("==================================================")
	fmt.Println(qr.ToSmallString(false))
	fmt.Printf("请使用手机相机或 Antigravity App 扫描上方二维码 (5分钟内有效)\n\n")
	if strings.Contains(primaryHost, ":") {
		fmt.Printf("🌐 外网 IPv6 配对 URI:   %s\n", uri)
	} else {
		fmt.Printf("🏠 局域网 Wi-Fi 配对 URI: %s\n", uri)
	}

	for _, extra := range extraHosts {
		extra = strings.TrimSpace(extra)
		if extra != "" && extra != primaryHost {
			extraURI := GeneratePairingURI(extra, port, code, ssl)
			if strings.Contains(extra, ":") {
				fmt.Printf("🌐 外网 IPv6 直连 URI:   %s\n", extraURI)
			} else {
				fmt.Printf("🏠 局域网 Wi-Fi 配对 URI: %s\n", extraURI)
			}
		}
	}
	fmt.Println()
	fmt.Println("💡 提示: 同 Wi-Fi 下直接扫码；若在蜂窝网络外出使用，可在 App 扫码页点击「手动输入」粘贴上方 IPv6 直连链接。")
	fmt.Println("==================================================")
	fmt.Println()
}

// GenerateQRCodePNG generates a PNG byte slice for the pairing URI.
func GenerateQRCodePNG(host string, port int, code string, ssl bool, size int) ([]byte, error) {
	if size <= 0 {
		size = 256
	}
	uri := GeneratePairingURI(host, port, code, ssl)
	return qrcode.Encode(uri, qrcode.Medium, size)
}
