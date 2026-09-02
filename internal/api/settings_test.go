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
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/zyvorai/kryton/internal/auth"
	"github.com/zyvorai/kryton/internal/catalog"
	"github.com/zyvorai/kryton/internal/demo"
	"github.com/zyvorai/kryton/internal/events"
	"github.com/zyvorai/kryton/internal/metrics"
	"github.com/zyvorai/kryton/internal/settings"
)

// testServerWithSettingsStore builds a server like testServer but with an
// in-memory settings.Store wired up — required for settings fields (like
// EventWebhookURL) that putSettings only persists through the store; with
// no store configured, such a field is applied to a local copy and lost
// the moment the response re-reads settings from scratch.
func testServerWithSettingsStore(t *testing.T) (http.Handler, *events.Bus) {
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
	store, err := settings.NewStore("", settings.Runtime{})
	if err != nil {
		t.Fatal(err)
	}
	var web fs.FS = fstest.MapFS{"index.html": {Data: []byte("ok")}}
	s := New(Config{
		Provider: demo.New(), Catalog: cat, Events: bus, Auth: a, Metrics: metrics.New(), Web: web,
		Projects: []string{"default"}, DefaultProject: "default", AuthMode: "disabled", Log: log,
		SettingsStore: store,
	})
	return s.Handler(), bus
}

// testServerMultiProject builds a server like testServer but with two
// configured projects, so settings updates that must pick among several
// projects (e.g. defaultProject) have something valid to switch to.
func testServerMultiProject(t *testing.T) (http.Handler, *events.Bus) {
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
		Projects: []string{"default", "secondary"}, DefaultProject: "default", AuthMode: "disabled", Log: log,
	})
	return s.Handler(), bus
}

// testServerAPIKeyRoles builds a server in apikey auth mode with one
// "viewer" and one "admin" key, both scoped to project "default", so
// role-gated handlers (putSettings, putStorageConfig, postStorageSetup)
// can be exercised for both the allowed and forbidden case.
func testServerAPIKeyRoles(t *testing.T) (h http.Handler, viewerToken, adminToken string) {
	t.Helper()
	viewerToken, adminToken = "viewer-token", "admin-token"
	keys := fmt.Sprintf(`{"keys":[
		{"name":"viewer","sha256":%q,"role":"viewer","projects":["default"]},
		{"name":"admin","sha256":%q,"role":"admin","projects":["default"]}
	]}`, auth.HashToken(viewerToken), auth.HashToken(adminToken))
	dir := t.TempDir()
	path := filepath.Join(dir, "keys.json")
	if err := os.WriteFile(path, []byte(keys), 0o600); err != nil {
		t.Fatal(err)
	}

	cat, err := catalog.Load("")
	if err != nil {
		t.Fatal(err)
	}
	a, err := auth.New(auth.Config{Mode: "apikey", APIKeysFile: path})
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
		Projects: []string{"default"}, DefaultProject: "default", AuthMode: "apikey", Log: log,
	})
	return s.Handler(), viewerToken, adminToken
}

func TestGetSettingsReturnsView(t *testing.T) {
	h, _ := testServer(t)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var view struct {
		Runtime struct {
			DefaultProject string `json:"defaultProject"`
		} `json:"runtime"`
		System struct {
			Provider string `json:"provider"`
		} `json:"system"`
		Editable []string `json:"editable"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
		t.Fatalf("decode: %v body=%s", err, w.Body.String())
	}
	if view.System.Provider != "demo" {
		t.Fatalf("expected provider demo, got %q", view.System.Provider)
	}
	if len(view.Editable) == 0 {
		t.Fatal("expected at least one editable field")
	}
}

func TestPutSettingsUpdatesDefaultProject(t *testing.T) {
	h, bus := testServerMultiProject(t)
	body := bytes.NewBufferString(`{"defaultProject":"secondary"}`)
	r := httptest.NewRequest(http.MethodPut, "/api/v1/settings", body)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var view struct {
		Runtime struct {
			DefaultProject string `json:"defaultProject"`
		} `json:"runtime"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil || view.Runtime.DefaultProject != "secondary" {
		t.Fatalf("expected defaultProject=secondary, got %+v err=%v", view, err)
	}
	evts := bus.List(1)
	if len(evts) != 1 || evts[0].Type != "io.kryton.settings.updated" {
		t.Fatalf("expected settings.updated event, got %#v", evts)
	}
}

func TestPutSettingsRejectsUnknownDefaultProject(t *testing.T) {
	h, _ := testServerMultiProject(t)
	body := bytes.NewBufferString(`{"defaultProject":"does-not-exist"}`)
	r := httptest.NewRequest(http.MethodPut, "/api/v1/settings", body)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPutSettingsRejectsEmptyDefaultProject(t *testing.T) {
	h, _ := testServerMultiProject(t)
	body := bytes.NewBufferString(`{"defaultProject":""}`)
	r := httptest.NewRequest(http.MethodPut, "/api/v1/settings", body)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPutSettingsUpdatesEventWebhookURL(t *testing.T) {
	h, _ := testServerWithSettingsStore(t)
	body := bytes.NewBufferString(`{"eventWebhookUrl":"https://hooks.example/kryton"}`)
	r := httptest.NewRequest(http.MethodPut, "/api/v1/settings", body)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var view struct {
		Runtime struct {
			EventWebhookURL string `json:"eventWebhookUrl"`
		} `json:"runtime"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil || view.Runtime.EventWebhookURL != "https://hooks.example/kryton" {
		t.Fatalf("unexpected view %+v err=%v", view, err)
	}
}

func TestPutSettingsForbiddenForViewerRole(t *testing.T) {
	h, viewerToken, adminToken := testServerAPIKeyRoles(t)

	body := bytes.NewBufferString(`{"eventWebhookUrl":"https://hooks.example/kryton"}`)
	r := httptest.NewRequest(http.MethodPut, "/api/v1/settings", body)
	r.Header.Set("Authorization", "Bearer "+viewerToken)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for viewer, got %d: %s", w.Code, w.Body.String())
	}

	body = bytes.NewBufferString(`{"eventWebhookUrl":"https://hooks.example/kryton"}`)
	r = httptest.NewRequest(http.MethodPut, "/api/v1/settings", body)
	r.Header.Set("Authorization", "Bearer "+adminToken)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for admin, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPostSettingsTestReturnsHealthyForDemo(t *testing.T) {
	h, _ := testServer(t)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/settings/test", nil))
	var resp struct {
		Healthy bool `json:"healthy"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v body=%s", err, w.Body.String())
	}
	if w.Code != http.StatusOK || !resp.Healthy {
		t.Fatalf("expected healthy 200 for demo provider, got status=%d healthy=%v body=%s", w.Code, resp.Healthy, w.Body.String())
	}
}

func TestPostAtlasTestDisabledReturnsOK(t *testing.T) {
	h, _ := testServer(t)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/integrations/atlas/test", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 when atlas is not configured/enabled, got %d: %s", w.Code, w.Body.String())
	}
}
