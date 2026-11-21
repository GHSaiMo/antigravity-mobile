package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

const localProxyCacheTTL = 60 * time.Second

var (
	localProxyMu        sync.Mutex
	cachedPortProxyHost string
	cachedPortCheckedAt time.Time
)

// DetectLocalProxy checks standard proxy environment variables first, and if unset,
// probes common local proxy ports (Clash/Mihomo/V2Ray/Surge/Sing-box) with a 60s cache
// so repeated HTTP client constructions do not block on sequential loopback dials.
func DetectLocalProxy() *url.URL {
	for _, envKey := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy", "ALL_PROXY", "all_proxy"} {
		if val := strings.TrimSpace(os.Getenv(envKey)); val != "" {
			if !strings.Contains(val, "://") {
				val = "http://" + val
			}
			if u, err := url.Parse(val); err == nil {
				return u
			}
		}
	}

	now := time.Now()
	localProxyMu.Lock()
	if !cachedPortCheckedAt.IsZero() && now.Sub(cachedPortCheckedAt) < localProxyCacheTTL {
		host := cachedPortProxyHost
		localProxyMu.Unlock()
		if host == "" {
			return nil
		}
		return &url.URL{Scheme: "http", Host: host}
	}
	localProxyMu.Unlock()

	var detectedHost string
	for _, p := range []int{7890, 10808, 1080, 6152} {
		addr := fmt.Sprintf("127.0.0.1:%d", p)
		conn, err := net.DialTimeout("tcp", addr, 150*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			detectedHost = addr
			break
		}
	}

	localProxyMu.Lock()
	cachedPortProxyHost = detectedHost
	cachedPortCheckedAt = time.Now()
	localProxyMu.Unlock()

	if detectedHost == "" {
		return nil
	}
	return &url.URL{Scheme: "http", Host: detectedHost}
}
