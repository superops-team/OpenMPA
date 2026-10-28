package runtimeterminal

import (
	"encoding/json"
	"io"
	"sync"
	"testing"
	"time"
)

type fakeSocket struct {
	in     chan []byte
	closed chan struct{}
	once   sync.Once
	mu     sync.Mutex
	writes []webshellFrame
}

func newFakeSocket() *fakeSocket {
	return &fakeSocket{in: make(chan []byte, 8), closed: make(chan struct{})}
}

func (s *fakeSocket) ReadMessage() ([]byte, error) {
	select {
	case msg := <-s.in:
		return msg, nil
	case <-s.closed:
		return nil, io.EOF
	}
}

func (s *fakeSocket) WriteJSON(v any) error {
	var frame webshellFrame
	raw, _ := json.Marshal(v)
	_ = json.Unmarshal(raw, &frame)
	s.mu.Lock()
	s.writes = append(s.writes, frame)
	s.mu.Unlock()
	return nil
}

func (s *fakeSocket) Close() error {
	s.once.Do(func() { close(s.closed) })
	return nil
}

func (s *fakeSocket) written() []webshellFrame {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]webshellFrame(nil), s.writes...)
}

func TestWebshellConnMapsTerminalInputOutputAndResize(t *testing.T) {
	socket := newFakeSocket()
	conn := newWebshellConn(socket)
	defer conn.Close()

	if err := conn.Resize(100, 30); err != nil {
		t.Fatal(err)
	}
	if n, err := conn.Write([]byte("pwd\n")); err != nil || n != 4 {
		t.Fatalf("write n=%d err=%v", n, err)
	}
	socket.in <- []byte(`{"Op":"stdout","Data":"ok\r\n"}`)
	buf := make([]byte, 32)
	n, err := conn.Read(buf)
	if err != nil || string(buf[:n]) != "ok\r\n" {
		t.Fatalf("read %q %v", string(buf[:n]), err)
	}
	writes := socket.written()
	if len(writes) < 2 || writes[0].Op != "resize" || writes[0].Cols != 100 || writes[0].Rows != 30 || writes[1].Op != "stdin" || writes[1].Data != "pwd\n" || writes[1].Cols != 100 || writes[1].Rows != 30 {
		t.Fatalf("bad frames %+v", writes)
	}
}

func TestWebshellConnClosesReadOnSocketClose(t *testing.T) {
	socket := newFakeSocket()
	conn := newWebshellConn(socket)
	socket.Close()
	done := make(chan error, 1)
	go func() {
		_, err := conn.Read(make([]byte, 8))
		done <- err
	}()
	select {
	case err := <-done:
		if err != io.EOF {
			t.Fatalf("read err %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("read did not unblock")
	}
}
