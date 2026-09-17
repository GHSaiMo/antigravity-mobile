package netutil

import (
	"bytes"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/pires/go-proxyproto"
)

func TestAdaptiveListener_NoProxyHeader_HTTP(t *testing.T) {
	baseListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer baseListener.Close()

	adaptive := NewAdaptiveListener(baseListener)
	defer adaptive.Close()

	payload := "GET /healthz HTTP/1.1\r\nHost: localhost\r\n\r\n"

	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := adaptive.Accept()
		if err != nil {
			t.Errorf("adaptive.Accept failed: %v", err)
			return
		}
		defer conn.Close()

		buf := make([]byte, 1024)
		n, err := conn.Read(buf)
		if err != nil && err != io.EOF {
			t.Errorf("read error: %v", err)
			return
		}

		received := string(buf[:n])
		if received != payload {
			t.Errorf("payload mismatch: expected %q, got %q", payload, received)
		}

		// RemoteAddr should be 127.0.0.1 (local client)
		if !strings.HasPrefix(conn.RemoteAddr().String(), "127.0.0.1:") {
			t.Errorf("expected 127.0.0.1 remote addr, got %s", conn.RemoteAddr().String())
		}
	}()

	clientConn, err := net.Dial("tcp", baseListener.Addr().String())
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	defer clientConn.Close()

	if _, err := clientConn.Write([]byte(payload)); err != nil {
		t.Fatalf("failed to write: %v", err)
	}

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("test timed out")
	}
}

func TestAdaptiveListener_NoProxyHeader_TLS(t *testing.T) {
	baseListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer baseListener.Close()

	adaptive := NewAdaptiveListener(baseListener)
	defer adaptive.Close()

	// Simulated TLS ClientHello header (0x16, 0x03, 0x03 ...)
	tlsPayload := []byte{0x16, 0x03, 0x03, 0x00, 0x05, 0x01, 0x02, 0x03, 0x04, 0x05}

	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := adaptive.Accept()
		if err != nil {
			t.Errorf("adaptive.Accept failed: %v", err)
			return
		}
		defer conn.Close()

		buf := make([]byte, 64)
		n, err := conn.Read(buf)
		if err != nil && err != io.EOF {
			t.Errorf("read error: %v", err)
			return
		}

		if !bytes.Equal(buf[:n], tlsPayload) {
			t.Errorf("TLS payload mismatch: expected %x, got %x", tlsPayload, buf[:n])
		}
	}()

	clientConn, err := net.Dial("tcp", baseListener.Addr().String())
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	defer clientConn.Close()

	if _, err := clientConn.Write(tlsPayload); err != nil {
		t.Fatalf("failed to write: %v", err)
	}

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("test timed out")
	}
}

func TestAdaptiveListener_ProxyV1(t *testing.T) {
	baseListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer baseListener.Close()

	adaptive := NewAdaptiveListener(baseListener)
	defer adaptive.Close()

	proxyHeader := "PROXY TCP4 117.136.12.34 124.222.226.143 45123 58900\r\n"
	appPayload := "GET /api/v1/auth/session HTTP/1.1\r\nHost: agy.jiuge.space\r\n\r\n"

	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := adaptive.Accept()
		if err != nil {
			t.Errorf("adaptive.Accept failed: %v", err)
			return
		}
		defer conn.Close()

		// Verify that RemoteAddr is rewritten to 117.136.12.34:45123
		if conn.RemoteAddr().String() != "117.136.12.34:45123" {
			t.Errorf("expected remote addr 117.136.12.34:45123, got %s", conn.RemoteAddr().String())
		}

		// Verify that the proxy header was consumed and only appPayload is read
		buf := make([]byte, 1024)
		n, err := conn.Read(buf)
		if err != nil && err != io.EOF {
			t.Errorf("read error: %v", err)
			return
		}

		if string(buf[:n]) != appPayload {
			t.Errorf("expected app payload %q, got %q", appPayload, string(buf[:n]))
		}
	}()

	clientConn, err := net.Dial("tcp", baseListener.Addr().String())
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	defer clientConn.Close()

	if _, err := clientConn.Write([]byte(proxyHeader + appPayload)); err != nil {
		t.Fatalf("failed to write: %v", err)
	}

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("test timed out")
	}
}

func TestAdaptiveListener_ProxyV2(t *testing.T) {
	baseListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer baseListener.Close()

	adaptive := NewAdaptiveListener(baseListener)
	defer adaptive.Close()

	// Build a valid PROXY protocol v2 binary header
	srcAddr, err := net.ResolveTCPAddr("tcp4", "117.136.88.99:54321")
	if err != nil {
		t.Fatal(err)
	}
	dstAddr, err := net.ResolveTCPAddr("tcp4", "124.222.226.143:58900")
	if err != nil {
		t.Fatal(err)
	}

	header := proxyproto.HeaderProxyFromAddrs(2, srcAddr, dstAddr)
	headerBytes, err := header.Format()
	if err != nil {
		t.Fatalf("failed to format v2 header: %v", err)
	}

	appPayload := []byte("Antigravity Mobile TLS payload")

	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := adaptive.Accept()
		if err != nil {
			t.Errorf("adaptive.Accept failed: %v", err)
			return
		}
		defer conn.Close()

		if conn.RemoteAddr().String() != "117.136.88.99:54321" {
			t.Errorf("expected remote addr 117.136.88.99:54321, got %s", conn.RemoteAddr().String())
		}

		buf := make([]byte, 1024)
		n, err := conn.Read(buf)
		if err != nil && err != io.EOF {
			t.Errorf("read error: %v", err)
			return
		}

		if !bytes.Equal(buf[:n], appPayload) {
			t.Errorf("expected payload %s, got %s", appPayload, buf[:n])
		}
	}()

	clientConn, err := net.Dial("tcp", baseListener.Addr().String())
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	defer clientConn.Close()

	packet := append(headerBytes, appPayload...)
	if _, err := clientConn.Write(packet); err != nil {
		t.Fatalf("failed to write: %v", err)
	}

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("test timed out")
	}
}
