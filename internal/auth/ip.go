package auth

import (
	"net"
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
