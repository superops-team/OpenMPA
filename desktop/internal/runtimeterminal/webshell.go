package runtimeterminal

import (
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"sync"
	"time"

	"golang.org/x/net/websocket"
)

type socket interface {
	ReadMessage() ([]byte, error)
	WriteJSON(any) error
	Close() error
}

type websocketDialer interface {
	Dial(string) (socket, error)
}

type websocketDialerFunc func(string) (socket, error)

func (f websocketDialerFunc) Dial(endpoint string) (socket, error) {
	return f(endpoint)
}

type xnetSocket struct{ conn *websocket.Conn }

func (s xnetSocket) ReadMessage() ([]byte, error) {
	var raw []byte
	err := websocket.Message.Receive(s.conn, &raw)
	return raw, err
}

func (s xnetSocket) WriteJSON(v any) error { return websocket.JSON.Send(s.conn, v) }
func (s xnetSocket) Close() error          { return s.conn.Close() }

type xnetDialer struct{}

func (xnetDialer) Dial(endpoint string) (socket, error) {
	origin := "https://localhost"
	if u, err := url.Parse(endpoint); err == nil && u.Host != "" {
		scheme := "https"
		if u.Scheme == "ws" {
			scheme = "http"
		}
		origin = scheme + "://" + u.Host
	}
	conn, err := websocket.Dial(endpoint, "", origin)
	if err != nil {
		return nil, err
	}
	return xnetSocket{conn: conn}, nil
}

type webshellConn struct {
	socket socket
	out    chan []byte
	done   chan struct{}
	once   sync.Once
	wmu    sync.Mutex
	mu     sync.RWMutex
	cols   int
	rows   int
}

type webshellFrame struct {
	Op   string `json:"Op"`
	Data string `json:"Data,omitempty"`
	Rows int    `json:"Rows,omitempty"`
	Cols int    `json:"Cols,omitempty"`
}

func newWebshellConn(socket socket) *webshellConn {
	c := &webshellConn{socket: socket, out: make(chan []byte, 64), done: make(chan struct{}), cols: 120, rows: 40}
	go c.readLoop()
	go c.heartbeat()
	return c
}

func dialWebshell(endpoint string, dialer websocketDialer) (*webshellConn, error) {
	if dialer == nil {
		dialer = xnetDialer{}
	}
	ws, err := dialer.Dial(endpoint)
	if err != nil {
		return nil, err
	}
	return newWebshellConn(ws), nil
}

func (c *webshellConn) readLoop() {
	defer c.Close()
	for {
		raw, err := c.socket.ReadMessage()
		if err != nil {
			return
		}
		var msg webshellFrame
		if json.Unmarshal(raw, &msg) != nil {
			c.deliver(raw)
			continue
		}
		switch msg.Op {
		case "stdout":
			c.deliver([]byte(msg.Data))
		case "error":
			c.deliver([]byte("\r\n" + msg.Data + "\r\n"))
		}
	}
}

func (c *webshellConn) heartbeat() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			_, _ = c.Write(nil)
		case <-c.done:
			return
		}
	}
}

func (c *webshellConn) deliver(data []byte) {
	if len(data) == 0 {
		return
	}
	copied := append([]byte(nil), data...)
	select {
	case c.out <- copied:
	case <-c.done:
	}
}

func (c *webshellConn) Read(p []byte) (int, error) {
	select {
	case data := <-c.out:
		if len(data) > len(p) {
			n := copy(p, data[:len(p)])
			rest := append([]byte(nil), data[n:]...)
			go c.deliver(rest)
			return n, nil
		}
		return copy(p, data), nil
	case <-c.done:
		return 0, io.EOF
	}
}

func (c *webshellConn) Write(p []byte) (int, error) {
	c.mu.RLock()
	rows, cols := c.rows, c.cols
	c.mu.RUnlock()
	if err := c.writeFrame(webshellFrame{Op: "stdin", Data: string(p), Rows: rows, Cols: cols}); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (c *webshellConn) Resize(cols, rows int) error {
	if cols < 1 || rows < 1 {
		return errors.New("invalid terminal size")
	}
	c.mu.Lock()
	c.cols, c.rows = cols, rows
	c.mu.Unlock()
	return c.writeFrame(webshellFrame{Op: "resize", Rows: rows, Cols: cols})
}

func (c *webshellConn) writeFrame(frame webshellFrame) error {
	select {
	case <-c.done:
		return io.ErrClosedPipe
	default:
	}
	c.wmu.Lock()
	defer c.wmu.Unlock()
	return c.socket.WriteJSON(frame)
}

func (c *webshellConn) Close() error {
	c.once.Do(func() {
		close(c.done)
		_ = c.socket.Close()
	})
	return nil
}
