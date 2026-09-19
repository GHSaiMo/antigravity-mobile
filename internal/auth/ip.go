package auth

import (
	"net"
	"net/http"
	"os"
	"strings"
)

// NetworkAddresses holds detected network IPs suitable for pairing.
type NetworkAddresses struct {
	LANIPv4    string
	PublicIPv6 string
}

// DetectNetworkAddresses discovers local network IPv4 and public IPv6 addresses.
func DetectNetworkAddresses() NetworkAddresses {
	var result NetworkAddresses

	if v := strings.TrimSpace(os.Getenv("MULTIGRAVITY_LAN_IPV4")); v != "" {
		result.LANIPv4 = v
	} else if v := strings.TrimSpace(os.Getenv("LAN_IPV4")); v != "" {
		result.LANIPv4 = v
	}

	if v := strings.TrimSpace(os.Getenv("MULTIGRAVITY_PUBLIC_IPV6")); v != "" {
		result.PublicIPv6 = v
	} else if v := strings.TrimSpace(os.Getenv("PUBLIC_IPV6")); v != "" {
		result.PublicIPv6 = v
	}

	if result.LANIPv4 != "" && result.PublicIPv6 != "" {
		return result
	}

	interfaces, err := net.Interfaces()
	if err != nil {
		return result
	}

	// isVirtualNetworkInterface checks whether an interface is a virtual adapter
	// (e.g. TUN/TAP, Clash, FlClash, sing-box, Tailscale, ZeroTier, VM, Docker, etc.).
	isVirtualNetworkInterface := func(name string) bool {
		n := strings.ToLower(name)
		keywords := []string{
			"utun", "tun", "tap", "wintun", "bridge", "vmenet", "anpi", "awdl", "llw",
			"clash", "sing-box", "singbox", "tailscale", "zerotier", "vmware",
			"virtual", "vbox", "vethernet", "hyper-v", "bluetooth", "蓝牙",
			"docker", "wg", "wireguard", "npcap", "teredo",
		}
		for _, kw := range keywords {
			if strings.Contains(n, kw) {
				return true
			}
		}
		return false
	}

	// isPhysicalNICName checks whether an interface looks like a primary physical network card.
	isPhysicalNICName := func(name string) bool {
		n := strings.ToLower(name)
		for _, kw := range []string{"wlan", "wi-fi", "wifi", "以太网", "ethernet", "en0", "en1", "eth0", "eth1"} {
			if strings.Contains(n, kw) {
				return true
			}
		}
		return false
	}

	// isPrivateOrLANIPv4 verifies that an IPv4 is a valid RFC 1918 private address:
	// - 10.0.0.0/8
	// - 172.16.0.0/12
	// - 192.168.0.0/16
	// Explicitly rejects APIPA (169.254.x.x) and Fake-IP / Benchmark (198.18.0.0/15).
	isPrivateOrLANIPv4 := func(ip net.IP) bool {
		ip4 := ip.To4()
		if ip4 == nil {
			return false
		}
		// Reject Loopback (127.0.0.0/8)
		if ip4[0] == 127 {
			return false
		}
		// Reject APIPA link-local (169.254.0.0/16)
		if ip4[0] == 169 && ip4[1] == 254 {
			return false
		}
		// Reject Benchmark / Proxy Fake-IP (198.18.0.0/15)
		if ip4[0] == 198 && (ip4[1] == 18 || ip4[1] == 19) {
			return false
		}
		// 10.0.0.0/8
		if ip4[0] == 10 {
			return true
		}
		// 172.16.0.0/12
		if ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31 {
			return true
		}
		// 192.168.0.0/16
		if ip4[0] == 192 && ip4[1] == 168 {
			return true
		}
		return false
	}

	type ifaceInfo struct {
		iface net.Interface
		addrs []net.Addr
	}

	var physicalNICs []ifaceInfo
	var otherNICs []ifaceInfo

	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if isVirtualNetworkInterface(iface.Name) {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil || len(addrs) == 0 {
			continue
		}
		info := ifaceInfo{iface: iface, addrs: addrs}
		if isPhysicalNICName(iface.Name) {
			physicalNICs = append(physicalNICs, info)
		} else {
			otherNICs = append(otherNICs, info)
		}
	}

	// Priority 1: Scan physical NICs (Wi-Fi, Ethernet, en0, eth0)
	// Priority 2: Scan other non-virtual NICs
	candidateGroups := [][]ifaceInfo{physicalNICs, otherNICs}

	for _, group := range candidateGroups {
		for _, item := range group {
			for _, addr := range item.addrs {
				var ip net.IP
				switch v := addr.(type) {
				case *net.IPNet:
					ip = v.IP
				case *net.IPAddr:
					ip = v.IP
				}

				if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
					continue
				}

				// IPv4
				if ip4 := ip.To4(); ip4 != nil {
					if result.LANIPv4 == "" && isPrivateOrLANIPv4(ip4) {
						result.LANIPv4 = ip4.String()
					}
					continue
				}

				// IPv6: global unicast 2000::/3 (starts with 001 in top 3 bits)
				if len(ip) == net.IPv6len && (ip[0]&0xe0) == 0x20 {
					if result.PublicIPv6 == "" {
						result.PublicIPv6 = ip.String()
					}
				}
			}
		}
	}

	return result
}

// IsLoopbackAddr checks whether a remote address (host:port or bare IP) is from localhost.
func IsLoopbackAddr(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// IsListenAddrLoopback reports whether the gateway listen host is loopback-only.
// Empty host, 0.0.0.0, :: and [::] bind all interfaces and are not loopback.
func IsListenAddrLoopback(host string) bool {
	h := strings.TrimSpace(host)
	h = strings.Trim(h, "[]")
	if h == "" || h == "0.0.0.0" || h == "::" {
		return false
	}
	if strings.EqualFold(h, "localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

// ExtractClientIP resolves the real client IP address from an incoming HTTP request.
// It prioritizes CleanIP(r.RemoteAddr) (which carries the true client IP
// when using AdaptiveListener with PROXY protocol or direct socket connections).
// If r.RemoteAddr is a loopback address, it also checks trusted reverse-proxy
// headers (CF-Connecting-IP, X-Real-IP, X-Forwarded-For) as a fallback.
func ExtractClientIP(r *http.Request) string {
	ip := CleanIP(r.RemoteAddr)

	// If the socket IP is loopback or local, check if an upstream proxy provided client headers
	if IsLoopbackAddr(ip) || ip == "localhost" || ip == "" {
		if cfIP := strings.TrimSpace(r.Header.Get("CF-Connecting-IP")); cfIP != "" {
			c := CleanIP(cfIP)
			if net.ParseIP(c) != nil {
				return c
			}
		}
		if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
			c := CleanIP(realIP)
			if net.ParseIP(c) != nil {
				return c
			}
		}
		if fwd := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); fwd != "" {
			parts := strings.Split(fwd, ",")
			if len(parts) > 0 {
				candidate := CleanIP(strings.TrimSpace(parts[0]))
				if candidate != "" && net.ParseIP(candidate) != nil {
					return candidate
				}
			}
		}
	}

	return ip
}

// RateLimitKeyIP normalizes an IP for rate-limiting.
// For IPv6 addresses, it aggregates by the /64 subnet (standard ISP subscriber allocation)
// to prevent rate-limit evasion via SLAAC/privacy address rotation (M-4).
// Loopback and IPv4 addresses are returned as single IPs.
func RateLimitKeyIP(addr string) string {
	ipStr := CleanIP(addr)
	parsed := net.ParseIP(ipStr)
	if parsed == nil {
		return ipStr
	}
	if parsed.IsLoopback() {
		return "loopback"
	}
	if ip4 := parsed.To4(); ip4 != nil {
		return ip4.String()
	}
	// IPv6: aggregate by /64 prefix
	mask64 := net.CIDRMask(64, 128)
	masked := parsed.Mask(mask64)
	return masked.String() + "/64"
}



