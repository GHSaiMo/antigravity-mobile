package tunnel

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

func TestIsCloudflareEdgeIP(t *testing.T) {
	for _, s := range []string{"198.41.192.7", "198.41.200.53", "2606:4700:a0::2", "162.159.36.1"} {
		if !IsCloudflareEdgeIP(net.ParseIP(s)) {
			t.Errorf("%s should be a Cloudflare edge IP", s)
		}
	}
	for _, s := range []string{"103.73.220.188", "8.8.8.8", "240e:3a1::1"} {
		if IsCloudflareEdgeIP(net.ParseIP(s)) {
			t.Errorf("%s should not be a Cloudflare edge IP", s)
		}
	}
}

func TestChooseEdgeIPVersion(t *testing.T) {
	ms := time.Millisecond
	cases := []struct {
		v4, v6 time.Duration
		want   string
	}{
		{0, 0, "auto"},
		{0, 150 * ms, "6"},
		{150 * ms, 0, "4"},
		{200 * ms, 140 * ms, "6"},
		{140 * ms, 200 * ms, "4"},
		{150 * ms, 140 * ms, "auto"},
	}
	for _, c := range cases {
		if got := chooseEdgeIPVersion(c.v4, c.v6); got != c.want {
			t.Errorf("choose(%v,%v)=%s want %s", c.v4, c.v6, got, c.want)
		}
	}
}

func TestProbeEdge_FiltersPollutionAndPicksFaster(t *testing.T) {
	resolve := func(_ context.Context, host string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("103.73.220.188"), net.ParseIP("198.41.192.7"), net.ParseIP("2606:4700:a0::2")}, nil
	}
	dial := func(_ context.Context, addr string) (time.Duration, error) {
		host, _, _ := net.SplitHostPort(addr)
		switch host {
		case "198.41.192.7":
			return 230 * time.Millisecond, nil
		case "2606:4700:a0::2":
			return 140 * time.Millisecond, nil
		}
		return 0, errors.New("must not dial polluted address: " + host)
	}
	p := ProbeEdge(context.Background(), resolve, dial)
	if p.Chosen != "6" {
		t.Errorf("expected IPv6 chosen, got %s (%+v)", p.Chosen, p)
	}
	if len(p.Polluted) != 1 || p.Polluted[0] != "103.73.220.188" {
		t.Errorf("unexpected polluted list: %v", p.Polluted)
	}
}

func TestProbeEdge_ResolveFailureKeepsAuto(t *testing.T) {
	resolve := func(context.Context, string) ([]net.IP, error) { return nil, errors.New("i/o timeout") }
	p := ProbeEdge(context.Background(), resolve, nil)
	if p.Chosen != "auto" {
		t.Errorf("expected auto, got %s", p.Chosen)
	}
}

func TestParseRegisteredLine(t *testing.T) {
	line := "2026-10-07T13:21:53Z INF Registered tunnel connection connIndex=2 connection=abc event=0 ip=198.41.192.7 location=lax13 protocol=http2"
	idx, colo, ok := parseRegisteredLine(line)
	if !ok || idx != "2" || colo != "lax13" {
		t.Fatalf("got %q %q %v", idx, colo, ok)
	}
	if _, _, ok := parseRegisteredLine("INF something else"); ok {
		t.Errorf("should not parse unrelated line")
	}
}

func TestParseFailedEdgeIP(t *testing.T) {
	line := `ERR Unable to establish connection with Cloudflare edge error="DialContext error: dial tcp 103.73.220.188:7844: i/o timeout" connIndex=0 event=0 ip=103.73.220.188`
	ip, ok := parseFailedEdgeIP(line)
	if !ok || ip.String() != "103.73.220.188" || IsCloudflareEdgeIP(ip) {
		t.Fatalf("got %v %v", ip, ok)
	}
	if _, ok := parseFailedEdgeIP("INF Registered tunnel connection ip=1.2.3.4"); ok {
		t.Errorf("unrelated line must not match")
	}
}

func TestRecordLocationDedup(t *testing.T) {
	tun := NewCloudflareTunnel(&CFTunnelResult{})
	tun.recordLocation("0", "lax13")
	tun.recordLocation("1", "lax07")
	tun.recordLocation("2", "lax13")
	got := tun.Locations()
	if len(got) != 2 || got[0] != "lax07" || got[1] != "lax13" {
		t.Errorf("unexpected locations: %v", got)
	}
}
