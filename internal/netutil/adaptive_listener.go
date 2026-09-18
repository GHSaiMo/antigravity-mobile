package netutil

import (
	"bufio"
	"errors"
	"io"
	"net"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/pires/go-proxyproto"
)

// AdaptiveListener wraps an underlying net.Listener and automatically inspects
// the incoming connection for PROXY protocol (v1 or v2) headers.
//
// If a PROXY protocol header is present, it parses the header, consumes it,
// and rewrites RemoteAddr() on the returned connection to the real client address.
//
// If no PROXY protocol header is detected (e.g., local curl, direct LAN, or
// direct TLS ClientHello without proxy protocol), it transparently yields
// the original connection without failing or discarding buffered bytes.
type AdaptiveListener struct {
	net.Listener
	HeaderTimeout time.Duration
	closed        atomic.Bool
}

// NewAdaptiveListener constructs an AdaptiveListener wrapping l.
func NewAdaptiveListener(l net.Listener) *AdaptiveListener {
	return &AdaptiveListener{
		Listener:      l,
		HeaderTimeout: 3 * time.Second,
	}
}

// Close closes the underlying listener and marks the adaptive listener as closed.
func (l *AdaptiveListener) Close() error {
	l.closed.Store(true)
	return l.Listener.Close()
}

// Accept waits for and returns the next connection to the listener, adapting
// dynamically to PROXY protocol and non-PROXY connections.
func (l *AdaptiveListener) Accept() (net.Conn, error) {
	for {
		if l.closed.Load() {
			return nil, net.ErrClosed
		}

		conn, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}

		if l.closed.Load() {
			_ = conn.Close()
			return nil, net.ErrClosed
		}

		timeout := l.HeaderTimeout
		if timeout <= 0 {
			timeout = 3 * time.Second
		}

		// Set a short read deadline while sniffing the initial bytes
		_ = conn.SetReadDeadline(time.Now().Add(timeout))
		br := bufio.NewReaderSize(conn, 512)
		header, parseErr := proxyproto.Read(br)
		// Clear read deadline so normal application timeouts take over
		_ = conn.SetReadDeadline(time.Time{})

		if parseErr != nil {
			if errors.Is(parseErr, proxyproto.ErrNoProxyProtocol) {
				// Transparent fallback: connection does not use PROXY protocol.
				// Preserve any bytes read by br and return connection as-is.
				return &bufferedConn{
					Conn: conn,
					r:    br,
				}, nil
			}

			// Malformed header, read timeout, or peer aborted during handshake
			_ = conn.Close()
			if l.closed.Load() {
				return nil, net.ErrClosed
			}
			continue
		}

		// PROXY protocol header successfully parsed
		var remoteAddr net.Addr
		if header != nil && header.SourceAddr != nil {
			remoteAddr = header.SourceAddr
		}

		return &bufferedConn{
			Conn:       conn,
			r:          br,
			remoteAddr: remoteAddr,
		}, nil
	}
}

// bufferedConn wraps net.Conn with a buffered reader to ensure bytes peeked
// or buffered during PROXY protocol detection are not lost.
type bufferedConn struct {
	net.Conn
	r          io.Reader
	remoteAddr net.Addr
}

func (c *bufferedConn) Read(b []byte) (int, error) {
	return c.r.Read(b)
}

func (c *bufferedConn) RemoteAddr() net.Addr {
	if c.remoteAddr != nil {
		return c.remoteAddr
	}
	return c.Conn.RemoteAddr()
}

// CloseWrite supports HTTP/2 graceful write closure if supported by the underlying connection.
func (c *bufferedConn) CloseWrite() error {
	if cw, ok := c.Conn.(interface{ CloseWrite() error }); ok {
		return cw.CloseWrite()
	}
	return nil
}

// TCPConn provides access to the underlying *net.TCPConn if applicable.
func (c *bufferedConn) TCPConn() (*net.TCPConn, bool) {
	if tc, ok := c.Conn.(*net.TCPConn); ok {
		return tc, true
	}
	return nil, false
}

// SyscallConn exposes underlying raw connection controls if supported.
func (c *bufferedConn) SyscallConn() (syscall.RawConn, error) {
	if sc, ok := c.Conn.(syscall.Conn); ok {
		return sc.SyscallConn()
	}
	return nil, errors.New("syscall.Conn not supported")
}
