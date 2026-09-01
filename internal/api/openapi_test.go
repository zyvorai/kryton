package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAPIDiscoveryAndOpenAPI(t *testing.T) {
	h, _ := testServer(t)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1", nil))
	if w.Code != 200 {
		t.Fatalf("discovery status=%d body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, `"openapi":"/openapi.yaml"`) || !strings.Contains(body, `/api/v1/machines`) {
		t.Fatalf("unexpected discovery: %s", body)
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/openapi.yaml", nil))
	if w.Code != 200 {
		t.Fatalf("openapi status=%d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "yaml") {
		t.Fatalf("content-type=%s", ct)
	}
	if !strings.Contains(w.Body.String(), "openapi:") || !strings.Contains(w.Body.String(), "/api/v1/settings/test") {
		t.Fatalf("openapi missing expected paths")
	}

	// Doctor must not be swallowed by discovery.
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/doctor", nil))
	if w.Code != 200 && w.Code != 503 {
		t.Fatalf("doctor status=%d body=%s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), `"endpoints"`) {
		t.Fatalf("doctor returned discovery catalog")
	}
}

func TestCORSPreflight(t *testing.T) {
	h, _ := testServerCORS(t, []string{"https://axiom.example"})
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/machines", nil)
	req.Header.Set("Origin", "https://axiom.example")
	req.Header.Set("Access-Control-Request-Method", "GET")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status=%d", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://axiom.example" {
		t.Fatalf("allow-origin=%q", got)
	}
}
