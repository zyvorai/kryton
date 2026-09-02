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
	"net/http"
	"net/http/httptest"
	"testing"
)

// The demo provider does not implement provider.ConsoleResolver, so
// console/vnc endpoints against it exercise the "unsupported provider"
// branch — the only console behavior reachable without a real
// dockur/kubevirt backend.

func TestMachineConsoleUnsupportedForDemoProviderJSON(t *testing.T) {
	h, _ := testServer(t)
	body := map[string]any{"project": "default", "name": "win-01", "image": "windows-server-2025", "compute": map[string]any{"cpu": 4, "memoryMiB": 8192}, "disk": map[string]any{"sizeGiB": 80}}
	b, _ := json.Marshal(body)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/machines", bytes.NewReader(b)))
	var m struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}

	w = httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/machines/"+m.ID+"/console?project=default", nil)
	h.ServeHTTP(w, r)
	if w.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501 for demo provider console, got %d: %s", w.Code, w.Body.String())
	}
}

func TestMachineConsoleUnsupportedForDemoProviderHTML(t *testing.T) {
	h, _ := testServer(t)
	body := map[string]any{"project": "default", "name": "win-02", "image": "windows-server-2025", "compute": map[string]any{"cpu": 4, "memoryMiB": 8192}, "disk": map[string]any{"sizeGiB": 80}}
	b, _ := json.Marshal(body)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/machines", bytes.NewReader(b)))
	var m struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}

	w = httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/machines/"+m.ID+"/console?project=default", nil)
	r.Header.Set("Accept", "text/html")
	h.ServeHTTP(w, r)
	// writeConsoleHTML is only reached once ConsoleTarget succeeds; the
	// demo provider fails the ConsoleResolver type assertion before that,
	// so even with an HTML Accept header this stays a JSON 501, not a
	// text/html error page — confirms the resolver-support check runs first.
	if w.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d: %s", w.Code, w.Body.String())
	}
}

func TestMachineVNCUnsupportedWithoutKubeClient(t *testing.T) {
	h, _ := testServer(t)
	body := map[string]any{"project": "default", "name": "win-03", "image": "windows-server-2025", "compute": map[string]any{"cpu": 4, "memoryMiB": 8192}, "disk": map[string]any{"sizeGiB": 80}}
	b, _ := json.Marshal(body)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/machines", bytes.NewReader(b)))
	var m struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}

	w = httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/machines/"+m.ID+"/vnc?project=default", nil)
	h.ServeHTTP(w, r)
	if w.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501 without a kube client, got %d: %s", w.Code, w.Body.String())
	}
}

func TestMachineConsoleRequiresViewerRole(t *testing.T) {
	h, _, _ := testServerAPIKeyRoles(t)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/machines/does-not-matter/console?project=default", nil)
	// No Authorization header at all in apikey mode: fails auth entirely (401),
	// distinct from the 403 a wrong-project/role principal would get.
	h.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without credentials, got %d: %s", w.Code, w.Body.String())
	}
}
