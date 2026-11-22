package relay

import (
	"errors"
	"net"
	"sync"
)

type relayAddr string

func (a relayAddr) Network() string { return "mgy-relay" }
func (a relayAddr) String() string  { return string(a) }

// StreamListener implements net.Listener fed directly by multiplexed yamux streams from mgy-relay.
// Serving an http.Server on StreamListener provides an unspoofable boundary: every connection
// accepted here is guaranteed to have originated from the untrusted relay transport.
type StreamListener struct {
	ch        chan net.Conn
	closeOnce sync.Once
	closed    chan struct{}
}

// NewStreamListener creates a ready-to-serve in-process listener for relay streams.
func NewStreamListener() *StreamListener {
	return &StreamListener{
		ch:     make(chan net.Conn, 128),
		closed: make(chan struct{}),
	}
}

// Enqueue hands an incoming yamux.Stream to the listening http.Server.
func (l *StreamListener) Enqueue(conn net.Conn) bool {
	select {
	case <-l.closed:
		_ = conn.Close()
		return false
	case l.ch <- conn:
		return true
	}
}

func (l *StreamListener) Accept() (net.Conn, error) {
	select {
	case <-l.closed:
		return nil, net.ErrClosed
	case c := <-l.ch:
		return c, nil
	}
}

func (l *StreamListener) Close() error {
	l.closeOnce.Do(func() {
		close(l.closed)
		for {
			select {
			case c := <-l.ch:
				_ = c.Close()
			default:
				return
			}
		}
	})
	return nil
}

func (l *StreamListener) Addr() net.Addr {
	return relayAddr("mgy-relay://in-process")
}

// ErrPoolExhausted indicates all configured relay pool nodes failed consecutively.
var ErrPoolExhausted = errors.New("all relay pool nodes unreachable")
