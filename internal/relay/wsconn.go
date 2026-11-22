package relay

import (
	"io"
	"net"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	wsPingInterval = 25 * time.Second // well below Cloudflare's 100s idle timeout
	wsPongWait     = 75 * time.Second
	wsWriteWait    = 15 * time.Second
)

// wsNetConn adapts a gorilla/websocket.Conn into a streaming net.Conn for yamux multiplexing,
// with automatic WebSocket Ping/Pong keepalives to survive Cloudflare Edge idle timeouts.
type wsNetConn struct {
	ws *websocket.Conn

	readMu sync.Mutex
	reader io.Reader

	writeMu sync.Mutex
	closed  bool
	done    chan struct{}
	once    sync.Once
}

func newWSNetConn(ws *websocket.Conn) *wsNetConn {
	c := &wsNetConn{
		ws:   ws,
		done: make(chan struct{}),
	}
	_ = ws.SetReadDeadline(time.Now().Add(wsPongWait))
	ws.SetPongHandler(func(string) error {
		return ws.SetReadDeadline(time.Now().Add(wsPongWait))
	})
	ws.SetPingHandler(func(appData string) error {
		_ = ws.SetReadDeadline(time.Now().Add(wsPongWait))
		c.writeMu.Lock()
		defer c.writeMu.Unlock()
		if c.closed {
			return net.ErrClosed
		}
		_ = ws.SetWriteDeadline(time.Now().Add(wsWriteWait))
		return ws.WriteMessage(websocket.PongMessage, []byte(appData))
	})
	go c.pingLoop()
	return c
}

func (c *wsNetConn) pingLoop() {
	ticker := time.NewTicker(wsPingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			c.writeMu.Lock()
			if c.closed {
				c.writeMu.Unlock()
				return
			}
			_ = c.ws.SetWriteDeadline(time.Now().Add(wsWriteWait))
			err := c.ws.WriteMessage(websocket.PingMessage, nil)
			c.writeMu.Unlock()
			if err != nil {
				_ = c.Close()
				return
			}
		}
	}
}

func (c *wsNetConn) Read(p []byte) (int, error) {
	c.readMu.Lock()
	defer c.readMu.Unlock()

	for {
		if c.reader == nil {
			msgType, r, err := c.ws.NextReader()
			if err != nil {
				return 0, err
			}
			_ = c.ws.SetReadDeadline(time.Now().Add(wsPongWait))
			if msgType != websocket.BinaryMessage {
				// Drain unexpected non-binary frames cleanly
				_, _ = io.Copy(io.Discard, r)
				continue
			}
			c.reader = r
		}

		n, err := c.reader.Read(p)
		if err == io.EOF {
			c.reader = nil
			if n > 0 {
				return n, nil
			}
			continue
		}
		return n, err
	}
}

func (c *wsNetConn) Write(p []byte) (int, error) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if c.closed {
		return 0, net.ErrClosed
	}
	_ = c.ws.SetWriteDeadline(time.Now().Add(wsWriteWait))
	if err := c.ws.WriteMessage(websocket.BinaryMessage, p); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (c *wsNetConn) Close() error {
	var err error
	c.once.Do(func() {
		close(c.done)
		c.writeMu.Lock()
		c.closed = true
		c.writeMu.Unlock()
		err = c.ws.Close()
	})
	return err
}

func (c *wsNetConn) LocalAddr() net.Addr  { return c.ws.LocalAddr() }
func (c *wsNetConn) RemoteAddr() net.Addr { return c.ws.RemoteAddr() }

func (c *wsNetConn) SetDeadline(t time.Time) error {
	// Keepalive manages idle deadlines on the underlying WS; yamux handles per-stream deadlines.
	return nil
}

func (c *wsNetConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *wsNetConn) SetWriteDeadline(t time.Time) error { return nil }
