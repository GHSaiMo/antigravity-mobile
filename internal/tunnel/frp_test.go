package tunnel

import (
	"testing"
)

func TestTunnelConfigAndRemoteURL(t *testing.T) {
	cfg := Config{
		ServerAddr: "198.51.100.1",
		ServerPort: 7000,
		Token:      "secret123",
		LocalPort:  58900,
		RemotePort: 58900,
	}

	tun := New(cfg)
	if tun.cfg.LocalIP != "127.0.0.1" {
		t.Fatalf("expected 127.0.0.1, got %s", tun.cfg.LocalIP)
	}

	url := tun.RemoteURL(false)
	if url != "http://198.51.100.1:58900" {
		t.Fatalf("expected http://198.51.100.1:58900, got %s", url)
	}

	sslURL := tun.RemoteURL(true)
	if sslURL != "https://198.51.100.1:58900" {
		t.Fatalf("expected https://198.51.100.1:58900, got %s", sslURL)
	}

	named := tun.RemoteURLFor("agy.example.com", true)
	if named != "https://agy.example.com:58900" {
		t.Fatalf("expected https://agy.example.com:58900, got %s", named)
	}
}
