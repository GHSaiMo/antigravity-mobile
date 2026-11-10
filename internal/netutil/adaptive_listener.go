package netutil

import (
	"bufio"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/pires/go-proxyproto"
)

var bufioReaderPool = sync.Pool{
	New: func() any {
		return bufio.NewReaderSize(nil, 512)
	},
}

// AdaptiveListener wraps an underlying net.Listener and automatically inspects
// the incoming connection for PROXY protocol (v1 or v2) headers.
//
// If a PROXY protocol header is present, it parses the header, consumes it,
// and rewrites RemoteAddr() on the returned connection to the real client address.
//
// If no PROXY protocol header is detected (e.g., local curl, direct LAN, or
// direct TLS ClientHello without proxy protocol), it transparently yields
// the original connection without failing or discarding buffered bytes.
//
// SEC-05: Header sniffing is deferred to the first Read() or RemoteAddr() call
// inside the per-connection goroutine so a slow or idle loopback preconnect
// socket never head-of-line blocks the global Accept() loop.
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

// Accept waits for and returns the next connection to the listener without
// performing any blocking I/O on the accept loop.
func (l *AdaptiveListener) Accept() (net.Conn, error) {
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

	// SEC: only a loopback peer (the local cloudflared connector) may supply a PROXY header.
	// Anyone else could forge "PROXY TCP4 127.0.0.1 ..." and pose as localhost, which the
	// auth middleware trusts. Non-loopback peers are passed through untouched, so a forged
	// header simply reaches the HTTP parser as garbage.
	if !isLoopbackPeer(conn.RemoteAddr()) {
		return conn, nil
	}

	timeout := l.HeaderTimeout
	if timeout <= 0 {
		timeout = 3 * time.Second
	}

	return &adaptiveConn{
		Conn:          conn,
		headerTimeout: timeout,
	}, nil
}

func isLoopbackPeer(a net.Addr) bool {
	if a == nil {
		return false
	}
	host, _, err := net.SplitHostPort(a.String())
	if err != nil {
		host = a.String()
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// adaptiveConn lazily inspects the initial bytes on a loopback connection for
// PROXY protocol v1/v2 headers upon the first Read() or RemoteAddr() call.
type adaptiveConn struct {
	net.Conn
	headerTimeout time.Duration

	initOnce   sync.Once
	sniffDone  atomic.Bool
	sniffErr   error
	br         *bufio.Reader
	remoteAddr net.Addr

	deadlineMu   sync.Mutex
	readDeadline time.Time
}

func (c *adaptiveConn) sniff() {
	c.deadlineMu.Lock()
	userDeadline := c.readDeadline
	c.deadlineMu.Unlock()

	sniffDeadline := time.Now().Add(c.headerTimeout)
	if !userDeadline.IsZero() && userDeadline.Before(sniffDeadline) {
		sniffDeadline = userDeadline
	}
	_ = c.Conn.SetReadDeadline(sniffDeadline)

	br := bufioReaderPool.Get().(*bufio.Reader)
	br.Reset(c.Conn)
	header, parseErr := proxyproto.Read(br)

	c.deadlineMu.Lock()
	restoreDeadline := c.readDeadline
	c.sniffDone.Store(true)
	c.deadlineMu.Unlock()
	_ = c.Conn.SetReadDeadline(restoreDeadline)

	if parseErr == nil {
		if header != nil && header.SourceAddr != nil {
			c.remoteAddr = header.SourceAddr
		}
		if br.Buffered() == 0 {
			br.Reset(nil)
			bufioReaderPool.Put(br)
		} else {
			c.br = br
		}
		return
	}

	if errors.Is(parseErr, proxyproto.ErrNoProxyProtocol) {
		// Transparent fallback: connection does not use PROXY protocol.
		// Preserve any bytes buffered during sniffing and release the reader once drained.
		if br.Buffered() == 0 {
			br.Reset(nil)
			bufioReaderPool.Put(br)
		} else {
			c.br = br
		}
		return
	}

	// Malformed PROXY header, read timeout, or peer aborted during handshake: fail closed
	// so a stalled/malformed tunnel connection can never retain loopback trust.
	br.Reset(nil)
	bufioReaderPool.Put(br)
	c.sniffErr = parseErr
	_ = c.Conn.Close()
}

func (c *adaptiveConn) Read(b []byte) (int, error) {
	c.initOnce.Do(c.sniff)
	if c.sniffErr != nil {
		return 0, c.sniffErr
	}
	if c.br != nil {
		n, err := c.br.Read(b)
		if c.br.Buffered() == 0 {
			c.br.Reset(nil)
			bufioReaderPool.Put(c.br)
			c.br = nil
		}
		return n, err
	}
	return c.Conn.Read(b)
}

func (c *adaptiveConn) RemoteAddr() net.Addr {
	c.initOnce.Do(c.sniff)
	if c.remoteAddr != nil {
		return c.remoteAddr
	}
	return c.Conn.RemoteAddr()
}

func (c *adaptiveConn) SetReadDeadline(t time.Time) error {
	c.deadlineMu.Lock()
	c.readDeadline = t
	done := c.sniffDone.Load()
	c.deadlineMu.Unlock()
	if done {
		return c.Conn.SetReadDeadline(t)
	}
	return nil
}

func (c *adaptiveConn) SetDeadline(t time.Time) error {
	c.deadlineMu.Lock()
	c.readDeadline = t
	done := c.sniffDone.Load()
	c.deadlineMu.Unlock()
	if err := c.Conn.SetWriteDeadline(t); err != nil {
		return err
	}
	if done {
		return c.Conn.SetReadDeadline(t)
	}
	return nil
}

// CloseWrite supports HTTP/2 graceful write closure if supported by the underlying connection.
func (c *adaptiveConn) CloseWrite() error {
	if cw, ok := c.Conn.(interface{ CloseWrite() error }); ok {
		return cw.CloseWrite()
	}
	return nil
}

// TCPConn provides access to the underlying *net.TCPConn if applicable.
func (c *adaptiveConn) TCPConn() (*net.TCPConn, bool) {
	if tc, ok := c.Conn.(*net.TCPConn); ok {
		return tc, true
	}
	return nil, false
}

// SyscallConn exposes underlying raw connection controls if supported.
func (c *adaptiveConn) SyscallConn() (syscall.RawConn, error) {
	if sc, ok := c.Conn.(syscall.Conn); ok {
		return sc.SyscallConn()
	}
	return nil, errors.New("syscall.Conn not supported")
}
