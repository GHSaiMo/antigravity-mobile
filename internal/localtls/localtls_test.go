package localtls

import (
	"context"
	"testing"
	"time"
)

func TestIsLoopbackHost(t *testing.T) {
	for _, h := range []string{"127.0.0.1", "::1", "localhost", "LOCALHOST", "[::1]"} {
		if !IsLoopbackHost(h) {
			t.Errorf("expected %q to be loopback", h)
		}
	}
	for _, h := range []string{"192.168.1.1", "example.com", "8.8.8.8"} {
		if IsLoopbackHost(h) {
			t.Errorf("expected %q not to be loopback", h)
		}
	}
}

func TestDialTLSContextRejectsNonLoopback(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := DialTLSContext(ctx, "tcp", "example.com:443")
	if err == nil {
		t.Fatal("expected non-loopback insecure TLS dial to fail")
	}
}

func TestNewLoopbackTransportRejectsNonLoopback(t *testing.T) {
	tr := NewLoopbackTransport()
	if tr == nil || tr.DialTLSContext == nil {
		t.Fatal("expected non-nil transport and DialTLSContext")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := tr.DialTLSContext(ctx, "tcp", "1.1.1.1:443")
	if err == nil {
		t.Fatal("expected non-loopback dial via transport to fail")
	}
}

