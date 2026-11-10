package localtls

import (
	"context"
	"net/http"
	"net/http/httptest"
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

func TestNewLoopbackTransportRejectsNonLoopbackAndNegotiatesHTTP2(t *testing.T) {
	tr := NewLoopbackTransport()
	if tr == nil || tr.DialContext == nil || !tr.ForceAttemptHTTP2 {
		t.Fatal("expected non-nil transport with DialContext and ForceAttemptHTTP2")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := tr.DialContext(ctx, "tcp", "1.1.1.1:443")
	if err == nil {
		t.Fatal("expected non-loopback dial via transport to fail")
	}

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	srv.EnableHTTP2 = true
	srv.StartTLS()
	defer srv.Close()

	client := &http.Client{Transport: tr, Timeout: 2 * time.Second}
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("expected loopback TLS request to succeed, got %v", err)
	}
	_ = resp.Body.Close()
	if resp.ProtoMajor != 2 {
		t.Fatalf("expected HTTP/2 negotiation over LoopbackTransport, got HTTP/%d.%d", resp.ProtoMajor, resp.ProtoMinor)
	}
}

