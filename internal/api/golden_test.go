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
	"encoding/json"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/zyvorai/kryton/internal/auth"
	"github.com/zyvorai/kryton/internal/catalog"
	"github.com/zyvorai/kryton/internal/demo"
	"github.com/zyvorai/kryton/internal/events"
	"github.com/zyvorai/kryton/internal/golden"
	"github.com/zyvorai/kryton/internal/metrics"
	"github.com/zyvorai/kryton/internal/model"
)

func testServerWithGolden(t *testing.T, g *golden.Manager) http.Handler {
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
	s := New(Config{
		Provider: demo.New(), Catalog: cat, Events: bus, Auth: a, Metrics: metrics.New(), Web: web,
		Projects: []string{"default"}, DefaultProject: "default", AuthMode: "disabled", Log: log,
		Golden: g,
	})
	return s.Handler()
}

func TestGoldenListEmptyWhenManagerNil(t *testing.T) {
	h := testServerWithGolden(t, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/golden", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Items []model.GoldenBuild `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil || resp.Items == nil || len(resp.Items) != 0 {
		t.Fatalf("expected empty items array, got %+v err=%v", resp, err)
	}
}

func TestGoldenGetNotFoundWhenManagerNil(t *testing.T) {
	h := testServerWithGolden(t, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/golden/b1", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGoldenStartUnavailableWhenManagerNil(t *testing.T) {
	h := testServerWithGolden(t, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/golden", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGoldenBootstrapUnavailableWhenManagerNil(t *testing.T) {
	h := testServerWithGolden(t, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/golden/b1/bootstrap", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGoldenGetNotFoundForUnknownBuild(t *testing.T) {
	g, err := golden.New(golden.Config{BaseDir: t.TempDir()})
	if err != nil {
		t.Fatalf("golden.New: %v", err)
	}
	h := testServerWithGolden(t, g)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/golden/does-not-exist", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGoldenBootstrapNotFoundForUnknownBuild(t *testing.T) {
	bootstrapScript := filepath.Join(t.TempDir(), "bootstrap.sh")
	if err := os.WriteFile(bootstrapScript, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake script: %v", err)
	}
	g, err := golden.New(golden.Config{BaseDir: t.TempDir(), BootstrapPath: bootstrapScript})
	if err != nil {
		t.Fatalf("golden.New: %v", err)
	}
	h := testServerWithGolden(t, g)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/golden/does-not-exist/bootstrap", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func writeGoldenStatus(t *testing.T, g *golden.Manager, id string, build model.GoldenBuild) {
	t.Helper()
	dir := filepath.Join(g.BaseDir(), id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	build.ID = id
	b, err := json.Marshal(build)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "status.json"), b, 0o644); err != nil {
		t.Fatalf("write status: %v", err)
	}
}

func TestGoldenBootstrapDisabledWithoutBootstrapScript(t *testing.T) {
	g, err := golden.New(golden.Config{BaseDir: t.TempDir()})
	if err != nil {
		t.Fatalf("golden.New: %v", err)
	}
	writeGoldenStatus(t, g, "b1", model.GoldenBuild{
		State: model.GoldenReady, ImageID: "windows-11-enterprise",
		OutputPath: filepath.Join(t.TempDir(), "artifact.qcow2"), UpdatedAt: time.Now(),
	})
	h := testServerWithGolden(t, g)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/golden/b1/bootstrap", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 (bootstrap script not configured), got %d: %s", w.Code, w.Body.String())
	}
}

func TestGoldenBootstrapBadRequestWhenNotReady(t *testing.T) {
	bootstrapScript := filepath.Join(t.TempDir(), "bootstrap.sh")
	if err := os.WriteFile(bootstrapScript, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake script: %v", err)
	}
	g, err := golden.New(golden.Config{BaseDir: t.TempDir(), BootstrapPath: bootstrapScript})
	if err != nil {
		t.Fatalf("golden.New: %v", err)
	}
	writeGoldenStatus(t, g, "b1", model.GoldenBuild{State: model.GoldenInstalling, ImageID: "windows-11-enterprise", UpdatedAt: time.Now()})
	h := testServerWithGolden(t, g)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/golden/b1/bootstrap", nil))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 (not ready), got %d: %s", w.Code, w.Body.String())
	}
}

func TestGoldenListReflectsManagerState(t *testing.T) {
	g, err := golden.New(golden.Config{BaseDir: t.TempDir()})
	if err != nil {
		t.Fatalf("golden.New: %v", err)
	}
	writeGoldenStatus(t, g, "b1", model.GoldenBuild{State: model.GoldenReady, ImageID: "windows-11-enterprise", UpdatedAt: time.Now()})
	h := testServerWithGolden(t, g)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/golden", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Items []model.GoldenBuild `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil || len(resp.Items) != 1 || resp.Items[0].ID != "b1" {
		t.Fatalf("expected build b1 listed, got %+v err=%v", resp, err)
	}
}
