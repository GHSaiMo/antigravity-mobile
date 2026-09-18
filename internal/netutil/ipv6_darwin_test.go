//go:build darwin

package netutil

import (
	"testing"
)

func TestCheckMacOSIPv6Status(t *testing.T) {
	status, err := CheckMacOSIPv6Status()
	if err != nil {
		t.Logf("CheckMacOSIPv6Status note: %v", err)
		return
	}
	if status != nil {
		t.Logf("Detected status: service=%s, device=%s, auto=%v, mode=%s",
			status.ServiceName, status.Device, status.IsAutomatic, status.CurrentMode)
		if status.ServiceName == "" {
			t.Errorf("expected non-empty service name")
		}
	}
}
