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
