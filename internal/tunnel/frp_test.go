package tunnel

import (
	"os"
	"strings"
	"testing"

	"github.com/fatedier/frp/pkg/config"
	v1 "github.com/fatedier/frp/pkg/config/v1"
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

func TestBuildConfigTOML_ProxyProtocol(t *testing.T) {
	cfg := Config{
		ServerAddr:           "198.51.100.1",
		ServerPort:           7000,
		Token:                "secret123",
		LocalPort:            58900,
		RemotePort:           58900,
		TLSEnable:            true,
		ProxyProtocolVersion: "v2",
	}

	tun := New(cfg)
	tomlStr := tun.BuildConfigTOML()
	if !strings.Contains(tomlStr, `transport.proxyProtocolVersion = "v2"`) {
		t.Fatalf("expected transport.proxyProtocolVersion = \"v2\", got:\n%s", tomlStr)
	}

	tmpFile, err := os.CreateTemp("", "frpc-test-*.toml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(tomlStr); err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	common, proxyCfgs, _, _, err := config.LoadClientConfig(tmpFile.Name(), true)
	if err != nil {
		t.Fatalf("failed to parse TOML with LoadClientConfig: %v", err)
	}
	if common.ServerAddr != "198.51.100.1" {
		t.Fatalf("unexpected serverAddr: %s", common.ServerAddr)
	}
	if len(proxyCfgs) != 1 {
		t.Fatalf("expected 1 proxy config, got %d", len(proxyCfgs))
	}
	tcpCfg, ok := proxyCfgs[0].(*v1.TCPProxyConfig)
	if !ok {
		t.Fatalf("expected *v1.TCPProxyConfig, got %T", proxyCfgs[0])
	}
	if tcpCfg.Transport.ProxyProtocolVersion != "v2" {
		t.Fatalf("expected ProxyProtocolVersion v2, got %s", tcpCfg.Transport.ProxyProtocolVersion)
	}
}

