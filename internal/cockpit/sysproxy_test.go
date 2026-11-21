package cockpit

import (
	"runtime"
	"testing"
	"time"
)

func TestIsProxyPortListening(t *testing.T) {
	// Port <= 0 should immediately return false
	if IsProxyPortListening(0, 100*time.Millisecond) {
		t.Error("expected false for port 0")
	}
	if IsProxyPortListening(-1, 100*time.Millisecond) {
		t.Error("expected false for port -1")
	}
}

func TestGetPrimaryNetworkService(t *testing.T) {
	service, err := GetPrimaryNetworkService()
	if runtime.GOOS == "darwin" {
		if err != nil {
			t.Logf("GetPrimaryNetworkService returned error (may be offline/virtualized): %v", err)
		} else {
			t.Logf("GetPrimaryNetworkService detected: %s", service)
		}
	} else {
		if service != "" || err != nil {
			t.Errorf("expected empty string and nil error on non-darwin, got service=%s, err=%v", service, err)
		}
	}
}

func TestEnsureSystemProxy(t *testing.T) {
	// Call EnsureSystemProxy with port 0 (defaults to 7890)
	// It should safely execute without panic or breaking network
	repaired, err := EnsureSystemProxy(0)
	t.Logf("EnsureSystemProxy result: repaired=%v, err=%v", repaired, err)
}

func TestShouldRestoreNetworkSetupProxies_PortMatchGuard(t *testing.T) {
	stale8080 := "Enabled: No\nServer: 127.0.0.1\nPort: 8080\nAuthenticated Proxy Enabled: 0\n"
	clash7890Off := "Enabled: No\nServer: 127.0.0.1\nPort: 7890\nAuthenticated Proxy Enabled: 0\n"
	clash7890On := "Enabled: Yes\nServer: 127.0.0.1\nPort: 7890\nAuthenticated Proxy Enabled: 0\n"
	remote7890 := "Enabled: No\nServer: 10.0.0.2\nPort: 7890\nAuthenticated Proxy Enabled: 0\n"

	if shouldRestoreNetworkSetupProxies(stale8080, stale8080, 7890) {
		t.Fatal("must reject restoring system proxy when configured port (8080) != targetPort (7890)")
	}
	if shouldRestoreNetworkSetupProxies(clash7890Off, stale8080, 7890) {
		t.Fatal("must reject when securewebproxy port mismatches targetPort")
	}
	if shouldRestoreNetworkSetupProxies(remote7890, remote7890, 7890) {
		t.Fatal("must reject non-loopback proxy server")
	}
	if shouldRestoreNetworkSetupProxies(clash7890On, clash7890On, 7890) {
		t.Fatal("must not re-enable when both proxies are already enabled")
	}
	if !shouldRestoreNetworkSetupProxies(clash7890Off, clash7890Off, 7890) {
		t.Fatal("expected true when both web & secure proxies point to 127.0.0.1:7890 and are disabled")
	}
}
