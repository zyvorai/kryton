package api

import (
	"bufio"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

type hijackable struct {
	http.ResponseWriter
	hijacked bool
}

func (h *hijackable) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h.hijacked = true
	c1, c2 := net.Pipe()
	_ = c2.Close()
	return c1, bufio.NewReadWriter(bufio.NewReader(c1), bufio.NewWriter(c1)), nil
}

func TestStatusWriterHijack(t *testing.T) {
	base := &hijackable{ResponseWriter: httptest.NewRecorder()}
	sw := &statusWriter{ResponseWriter: base}
	if _, _, err := sw.Hijack(); err != nil {
		t.Fatalf("Hijack: %v", err)
	}
	if !base.hijacked {
		t.Fatal("underlying Hijack not called")
	}
}
