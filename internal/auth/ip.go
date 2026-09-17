package auth

import (
	"net"
	"net/http"
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

	interfaces, err := net.Interfaces()
	if err != nil {
		return result
	}

	// Sort/prioritize: scan en0 first if available
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		name := strings.ToLower(iface.Name)
		// Skip virtual/bridge interfaces to prioritize physical NICs
		if strings.HasPrefix(name, "utun") || strings.HasPrefix(name, "bridge") ||
			strings.HasPrefix(name, "vmenet") || strings.HasPrefix(name, "anpi") ||
			strings.HasPrefix(name, "awdl") || strings.HasPrefix(name, "llw") {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
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
				if result.LANIPv4 == "" {
					result.LANIPv4 = ip4.String()
				}
				continue
			}

			// IPv6: global unicast 2000::/3 (starts with 001 in top 3 bits)
			if len(ip) == net.IPv6len && (ip[0]&0xe0) == 0x20 {
				// Prefer permanent address if possible, but any global unicast works
				if result.PublicIPv6 == "" {
					result.PublicIPv6 = ip.String()
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
			return CleanIP(cfIP)
		}
		if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
			return CleanIP(realIP)
		}
		if fwd := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); fwd != "" {
			parts := strings.Split(fwd, ",")
			if len(parts) > 0 {
				candidate := strings.TrimSpace(parts[0])
				if candidate != "" {
					return CleanIP(candidate)
				}
			}
		}
	}

	return ip
}


