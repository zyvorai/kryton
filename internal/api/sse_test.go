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
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// syncRecorder is a minimal http.ResponseWriter + http.Flusher backed by a
// mutex-guarded buffer, safe to read from a test goroutine while the SSE
// handler concurrently writes to it — unlike httptest.ResponseRecorder,
// whose *bytes.Buffer is not safe for that.
type syncRecorder struct {
	mu     sync.Mutex
	header http.Header
	status int
	buf    bytes.Buffer
}

func newSyncRecorder() *syncRecorder { return &syncRecorder{header: http.Header{}} }

func (r *syncRecorder) Header() http.Header { return r.header }

func (r *syncRecorder) Write(b []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.buf.Write(b)
}

func (r *syncRecorder) WriteHeader(code int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status = code
}

// Flush satisfies http.Flusher; the buffer is already visible to readers
// as soon as Write returns, so there is nothing extra to do.
func (r *syncRecorder) Flush() {}

func (r *syncRecorder) String() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.buf.String()
}

func waitForSubstring(t *testing.T, get func() string, substr string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if strings.Contains(get(), substr) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %q in stream, got: %s", substr, get())
}

func TestStreamEventsDeliversPublishedEvent(t *testing.T) {
	h, bus := testServer(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/events/stream", nil).WithContext(ctx)
	w := newSyncRecorder()

	done := make(chan struct{})
	go func() {
		h.ServeHTTP(w, r)
		close(done)
	}()

	// Wait for the handler to reach its Subscribe call and write the
	// preamble comment, so the event below isn't published before anyone
	// is listening.
	waitForSubstring(t, w.String, "kryton event stream", time.Second)

	bus.Publish(context.Background(), "io.kryton.machine.created", "machines/m1", map[string]any{"machineId": "m1"})

	waitForSubstring(t, w.String, "event: cloudevent", 2*time.Second)
	if !strings.Contains(w.String(), `"machineId":"m1"`) {
		t.Fatalf("expected published event data in stream, got: %s", w.String())
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("streamEvents did not return after context cancellation")
	}
}

func TestStreamEventsReturnsImmediatelyWhenAlreadyCanceled(t *testing.T) {
	h, _ := testServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/events/stream", nil).WithContext(ctx)
	w := newSyncRecorder()

	done := make(chan struct{})
	go func() {
		h.ServeHTTP(w, r)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("streamEvents did not return for an already-canceled context")
	}
}
