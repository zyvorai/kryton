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
	bus, err := events.New(100, "", "", "", log)
	if err != nil {
		t.Fatal(err)
	}
	var web fs.FS = fstest.MapFS{"index.html": {Data: []byte("ok")}}
	s := New(Config{Provider: demo.New(), Catalog: cat, Events: bus, Auth: a, Metrics: metrics.New(), Web: web, Projects: []string{"default"}, DefaultProject: "default", AuthMode: "disabled", Log: log})
	return s.Handler(), bus
}

func testServerCORS(t *testing.T, origins []string) (http.Handler, *events.Bus) {
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
	bus, err := events.New(100, "", "", "", log)
	if err != nil {
		t.Fatal(err)
	}
	var web fs.FS = fstest.MapFS{"index.html": {Data: []byte("ok")}}
	s := New(Config{Provider: demo.New(), Catalog: cat, Events: bus, Auth: a, Metrics: metrics.New(), Web: web, Projects: []string{"default"}, DefaultProject: "default", AuthMode: "disabled", Log: log, CORSOrigins: origins})
	return s.Handler(), bus
}

func TestDoctor(t *testing.T) {
	h, _ := testServer(t)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/doctor", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("doctor status=%d body=%s", w.Code, w.Body.String())
	}
	var report struct {
		Healthy  bool  `json:"healthy"`
		Findings []any `json:"findings"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &report); err != nil || !report.Healthy || len(report.Findings) == 0 {
		t.Fatalf("bad doctor report: %v %s", err, w.Body.String())
	}
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

func TestSnapshotListRestoreDelete(t *testing.T) {
	h, _ := testServer(t)
	body := map[string]any{"project": "default", "name": "win-snap", "image": "windows-server-2025", "compute": map[string]any{"cpu": 4, "memoryMiB": 8192}, "disk": map[string]any{"sizeGiB": 80}}
	b, _ := json.Marshal(body)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/machines", bytes.NewReader(b))
	r.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", w.Code, w.Body.String())
	}
	var m struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	sr := httptest.NewRequest(http.MethodPost, "/api/v1/machines/"+m.ID+"/snapshot?project=default", bytes.NewBufferString(`{"name":"before"}`))
	sr.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, sr)
	if w.Code != http.StatusCreated {
		t.Fatalf("snapshot status=%d body=%s", w.Code, w.Body.String())
	}
	var snap struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &snap); err != nil || snap.ID == "" {
		t.Fatalf("bad snapshot %v %s", err, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/machines/"+m.ID+"/snapshots?project=default", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", w.Code, w.Body.String())
	}
	var list struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil || len(list.Items) != 1 || list.Items[0].ID != snap.ID {
		t.Fatalf("list=%s", w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/machines/"+m.ID+"/snapshots/"+snap.ID+"/restore?project=default", nil))
	if w.Code != http.StatusAccepted {
		t.Fatalf("restore status=%d body=%s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/api/v1/machines/"+m.ID+"/snapshots/"+snap.ID+"?project=default", nil))
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d body=%s", w.Code, w.Body.String())
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
