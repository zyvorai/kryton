package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLabBootstrapDisabled(t *testing.T) {
	s := &Server{authMode: "apikey", allowInsecure: true, labAutoAuth: false}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/lab/bootstrap", nil)
	rec := httptest.NewRecorder()
	s.labBootstrap(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"autoAuth":false`) {
		t.Fatalf("body %s", rec.Body.String())
	}
}

func TestLabBootstrapEnabled(t *testing.T) {
	dir := t.TempDir()
	tokenFile := filepath.Join(dir, "lab.token")
	if err := os.WriteFile(tokenFile, []byte("lab-secret-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &Server{authMode: "apikey", allowInsecure: true, labAutoAuth: true, labTokenFile: tokenFile}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/lab/bootstrap", nil)
	rec := httptest.NewRecorder()
	s.labBootstrap(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"autoAuth":true`) || !strings.Contains(body, "lab-secret-token") {
		t.Fatalf("body %s", body)
	}
}
