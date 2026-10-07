package tunnel

import (
	"context"
	"net"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// cloudflared 的边缘入口域名与端口 (HTTP/2 与 QUIC 均使用 7844)。
var cfEdgeHosts = []string{"region1.v2.argotunnel.com", "region2.v2.argotunnel.com"}

const cfEdgePort = "7844"

// cfEdgeNets 为 Cloudflare 公布的边缘网段。解析结果落在这些网段之外的，
// 基本可以判定是被 DNS 污染/劫持（如日志里出现过的 103.73.220.188）。
var cfEdgeNets = mustParseCIDRs(
	"198.41.128.0/17",
	"162.159.0.0/16",
	"2606:4700::/32",
	"2803:f800::/32",
)

func mustParseCIDRs(cidrs ...string) []*net.IPNet {
	nets := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			panic(err)
		}
		nets = append(nets, n)
	}
	return nets
}

// IsCloudflareEdgeIP reports whether ip belongs to a published Cloudflare range.
func IsCloudflareEdgeIP(ip net.IP) bool {
	for _, n := range cfEdgeNets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// EdgeProbe is the result of a pre-flight reachability test towards the Cloudflare edge.
type EdgeProbe struct {
	V4RTT    time.Duration // 0 = unreachable / no clean address
	V6RTT    time.Duration
	Polluted []string // resolved addresses outside Cloudflare ranges
	Chosen   string   // "4", "6" or "auto"
}

// chooseEdgeIPVersion picks the IP family with the clearly lower TCP connect RTT.
// When both are comparable (within 15%) or neither is reachable, it keeps "auto".
func chooseEdgeIPVersion(v4, v6 time.Duration) string {
	switch {
	case v4 == 0 && v6 == 0:
		return "auto"
	case v4 == 0:
		return "6"
	case v6 == 0:
		return "4"
	case float64(v6) < float64(v4)*0.85:
		return "6"
	case float64(v4) < float64(v6)*0.85:
		return "4"
	}
	return "auto"
}

type edgeDialer func(ctx context.Context, addr string) (time.Duration, error)
type edgeResolver func(ctx context.Context, host string) ([]net.IP, error)

func defaultEdgeResolver(ctx context.Context, host string) ([]net.IP, error) {
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	ips := make([]net.IP, 0, len(addrs))
	for _, a := range addrs {
		ips = append(ips, a.IP)
	}
	return ips, nil
}

func defaultEdgeDialer(ctx context.Context, addr string) (time.Duration, error) {
	start := time.Now()
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return 0, err
	}
	rtt := time.Since(start)
	_ = conn.Close()
	return rtt, nil
}

// ProbeEdge resolves the tunnel edge hosts, drops polluted answers, and measures the
// best TCP connect RTT per IP family so the faster/cleaner family can be pinned.
func ProbeEdge(ctx context.Context, resolve edgeResolver, dial edgeDialer) EdgeProbe {
	if resolve == nil {
		resolve = defaultEdgeResolver
	}
	if dial == nil {
		dial = defaultEdgeDialer
	}
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()

	var (
		mu       sync.Mutex
		v4, v6   []net.IP
		polluted = map[string]bool{}
		wg       sync.WaitGroup
	)
	for _, host := range cfEdgeHosts {
		wg.Add(1)
		go func(h string) {
			defer wg.Done()
			ips, err := resolve(ctx, h)
			if err != nil {
				return
			}
			mu.Lock()
			defer mu.Unlock()
			for _, ip := range ips {
				switch {
				case !IsCloudflareEdgeIP(ip):
					polluted[ip.String()] = true
				case ip.To4() != nil:
					v4 = append(v4, ip)
				default:
					v6 = append(v6, ip)
				}
			}
		}(host)
	}
	wg.Wait()

	best := func(ips []net.IP) time.Duration {
		if len(ips) > 4 {
			ips = ips[:4]
		}
		results := make(chan time.Duration, len(ips))
		for _, ip := range ips {
			go func(ip net.IP) {
				dctx, c := context.WithTimeout(ctx, 2*time.Second)
				defer c()
				rtt, err := dial(dctx, net.JoinHostPort(ip.String(), cfEdgePort))
				if err != nil {
					rtt = 0
				}
				results <- rtt
			}(ip)
		}
		var min time.Duration
		for range ips {
			if r := <-results; r > 0 && (min == 0 || r < min) {
				min = r
			}
		}
		return min
	}

	var p EdgeProbe
	var pwg sync.WaitGroup
	pwg.Add(2)
	go func() { defer pwg.Done(); p.V4RTT = best(v4) }()
	go func() { defer pwg.Done(); p.V6RTT = best(v6) }()
	pwg.Wait()

	for ip := range polluted {
		p.Polluted = append(p.Polluted, ip)
	}
	sort.Strings(p.Polluted)
	p.Chosen = chooseEdgeIPVersion(p.V4RTT, p.V6RTT)
	return p
}

var (
	reConnIndex = regexp.MustCompile(`connIndex=(\d+)`)
	reLocation  = regexp.MustCompile(`location=([A-Za-z0-9]+)`)
	reEdgeIP    = regexp.MustCompile(`\bip=([0-9a-fA-F:.]+)`)
)

// parseRegisteredLine extracts the connection index and edge colo (e.g. "lax13")
// from a cloudflared "Registered tunnel connection" log line.
func parseRegisteredLine(line string) (idx string, location string, ok bool) {
	m1 := reConnIndex.FindStringSubmatch(line)
	m2 := reLocation.FindStringSubmatch(line)
	if m1 == nil || m2 == nil {
		return "", "", false
	}
	return m1[1], strings.ToLower(m2[1]), true
}

// parseFailedEdgeIP returns the edge IP from a failed-dial log line, if present.
func parseFailedEdgeIP(line string) (net.IP, bool) {
	if !strings.Contains(line, "Unable to establish connection") && !strings.Contains(line, "Serve tunnel error") {
		return nil, false
	}
	m := reEdgeIP.FindStringSubmatch(line)
	if m == nil {
		return nil, false
	}
	ip := net.ParseIP(m[1])
	return ip, ip != nil
}
