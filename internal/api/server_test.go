package api

import (
	"bytes"
	"encoding/json"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/zyvorai/kryton/internal/auth"
	"github.com/zyvorai/kryton/internal/catalog"
	"github.com/zyvorai/kryton/internal/demo"
	"github.com/zyvorai/kryton/internal/events"
	"github.com/zyvorai/kryton/internal/metrics"
)

func testServer(t *testing.T) (http.Handler, *events.Bus) {
	t.Helper()
	cat, err := catalog.Load("")
	if err != nil {
		t.Fatal(err)
	}
	a, err := auth.New(auth.Config{Mode: "disabled"})
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	bus := events.New(100, "", log)
	var web fs.FS = fstest.MapFS{"index.html": {Data: []byte("ok")}}
	s := New(Config{Provider: demo.New(), Catalog: cat, Events: bus, Auth: a, Metrics: metrics.New(), Web: web, Projects: []string{"default"}, DefaultProject: "default", Log: log})
	return s.Handler(), bus
}

func TestCreateLifecycleAndEvents(t *testing.T) {
	h, bus := testServer(t)
	body := map[string]any{"project": "default", "name": "win-01", "image": "windows-server-2025", "compute": map[string]any{"cpu": 4, "memoryMiB": 8192}, "disk": map[string]any{"sizeGiB": 80}}
	b, _ := json.Marshal(body)
	r := httptest.NewRequest(http.MethodPost, "/api/v1/machines", bytes.NewReader(b))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", w.Code, w.Body.String())
	}
	var m struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil || m.ID == "" {
		t.Fatalf("bad response %v %s", err, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/machines/"+m.ID+"/stop?project=default", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("stop status=%d body=%s", w.Code, w.Body.String())
	}
	events := bus.List(10)
	if len(events) != 2 || events[0].Type != "io.kryton.machine.stopped" || events[1].Type != "io.kryton.machine.created" {
		t.Fatalf("unexpected events %#v", events)
	}
}

func TestConflictDoesNotPublishSuccessEvent(t *testing.T) {
	h, bus := testServer(t)
	payload := []byte(`{"project":"default","name":"win-01","image":"windows-server-2025","compute":{"cpu":4,"memoryMiB":8192},"disk":{"sizeGiB":80}}`)
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/machines", bytes.NewReader(payload))
		h.ServeHTTP(w, r)
		if i == 0 && w.Code != 201 {
			t.Fatal(w.Code)
		}
		if i == 1 && w.Code != 409 {
			t.Fatalf("expected conflict, got %d", w.Code)
		}
	}
	if got := len(bus.List(10)); got != 1 {
		t.Fatalf("expected one event, got %d", got)
	}
}

func TestInvalidUnknownField(t *testing.T) {
	h, _ := testServer(t)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/machines", bytes.NewBufferString(`{"project":"default","name":"win-01","image":"windows-server-2025","compute":{"cpu":4,"memoryMiB":8192},"disk":{"sizeGiB":80},"surprise":true}`))
	h.ServeHTTP(w, r)
	if w.Code != 400 {
		t.Fatalf("expected 400 got %d: %s", w.Code, w.Body.String())
	}
}
