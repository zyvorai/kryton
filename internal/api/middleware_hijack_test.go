// Copyright 2026 Kryton contributors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
