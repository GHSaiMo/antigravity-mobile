package cockpit

import (
	"strconv"
	"strings"
)

type parsedNetworkSetupProxy struct {
	enabled    bool
	hasEnabled bool
	server     string
	port       int
}

func parseNetworkSetupProxyOutput(out string) (parsedNetworkSetupProxy, bool) {
	var res parsedNetworkSetupProxy
	for _, rawLine := range strings.Split(out, "\n") {
		line := strings.TrimSpace(rawLine)
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(k))
		val := strings.TrimSpace(v)
		switch key {
		case "enabled":
			res.hasEnabled = true
			res.enabled = strings.EqualFold(val, "yes")
		case "server":
			res.server = strings.ToLower(val)
		case "port":
			if p, err := strconv.Atoi(val); err == nil {
				res.port = p
			}
		}
	}
	if !res.hasEnabled || res.server == "" || res.port <= 0 {
		return parsedNetworkSetupProxy{}, false
	}
	return res, true
}

func isLoopbackProxyServer(server string) bool {
	s := strings.ToLower(strings.TrimSpace(server))
	return s == "127.0.0.1" || s == "localhost"
}

// shouldRestoreNetworkSetupProxies returns true ONLY when both webproxy and securewebproxy
// are explicitly configured to loopback (127.0.0.1/localhost) AND their configured Port
// strictly matches targetPort, preventing accidental or malicious activation of stale proxy ports.
func shouldRestoreNetworkSetupProxies(webOut, secureOut string, targetPort int) bool {
	if targetPort <= 0 {
		return false
	}
	webCfg, okW := parseNetworkSetupProxyOutput(webOut)
	secCfg, okS := parseNetworkSetupProxyOutput(secureOut)
	if !okW || !okS {
		return false
	}
	if !isLoopbackProxyServer(webCfg.server) || !isLoopbackProxyServer(secCfg.server) {
		return false
	}
	if webCfg.port != targetPort || secCfg.port != targetPort {
		return false
	}
	return !webCfg.enabled || !secCfg.enabled
}
