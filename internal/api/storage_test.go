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

	"github.com/zyvorai/kryton/internal/storage"
)

func TestGetStorageWithoutKubeClient(t *testing.T) {
	h, _ := testServer(t)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/storage", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var inv storage.Inventory
	if err := json.Unmarshal(w.Body.Bytes(), &inv); err != nil {
		t.Fatalf("decode: %v body=%s", err, w.Body.String())
	}
	if len(inv.StorageClasses) != 0 || inv.Provider != "demo" {
		t.Fatalf("expected empty inventory for demo provider with no kube client, got %+v", inv)
	}
}

func TestGetStorageConfigDefault(t *testing.T) {
	h, _ := testServer(t)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/storage/config", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var cfg storage.Config
	if err := json.Unmarshal(w.Body.Bytes(), &cfg); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if cfg.StorageClass != "" {
		t.Fatalf("expected empty default storage class, got %q", cfg.StorageClass)
	}
}

func TestPutStorageConfigRejectedForNonKubevirtProvider(t *testing.T) {
	h, _ := testServer(t)
	body := bytes.NewBufferString(`{"storageClass":"rook-ceph-block"}`)
	r := httptest.NewRequest(http.MethodPut, "/api/v1/storage/config", body)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501 for demo provider, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPutStorageConfigForbiddenForViewerRole(t *testing.T) {
	h, viewerToken, _ := testServerAPIKeyRoles(t)
	body := bytes.NewBufferString(`{"storageClass":"rook-ceph-block"}`)
	r := httptest.NewRequest(http.MethodPut, "/api/v1/storage/config", body)
	r.Header.Set("Authorization", "Bearer "+viewerToken)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for viewer, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetStorageSetupIdleWhenNoManager(t *testing.T) {
	h, _ := testServer(t)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/storage/setup", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		State string   `json:"state"`
		Logs  []string `json:"logs"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil || resp.State != "idle" {
		t.Fatalf("expected idle state, got %+v err=%v", resp, err)
	}
}

func TestPostStorageSetupRejectedForNonKubevirtProvider(t *testing.T) {
	h, _ := testServer(t)
	body := bytes.NewBufferString(`{"backend":"longhorn"}`)
	r := httptest.NewRequest(http.MethodPost, "/api/v1/storage/setup", body)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501 for demo provider, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPostStorageSetupForbiddenForViewerRole(t *testing.T) {
	h, viewerToken, _ := testServerAPIKeyRoles(t)
	body := bytes.NewBufferString(`{"backend":"longhorn"}`)
	r := httptest.NewRequest(http.MethodPost, "/api/v1/storage/setup", body)
	r.Header.Set("Authorization", "Bearer "+viewerToken)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for viewer, got %d: %s", w.Code, w.Body.String())
	}
}
