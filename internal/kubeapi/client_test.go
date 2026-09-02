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

package kubeapi

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeTempCertKeyPair generates a self-signed ECDSA cert/key pair and
// writes both PEM files under t.TempDir(), returning their paths and the
// certificate PEM bytes (usable as a CA bundle for TestNew's CAFile cases).
func writeTempCertKeyPair(t *testing.T) (certPath, keyPath string, certPEM []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: bigOne(),
		Subject:      pkix.Name{CommonName: "kryton-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	dir := t.TempDir()
	certPath = filepath.Join(dir, "cert.pem")
	keyPath = filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certPath, certPEM, 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	return certPath, keyPath, certPEM
}

func bigOne() *big.Int { return big.NewInt(1) }

func TestNewRequiresEndpointOrInCluster(t *testing.T) {
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	t.Setenv("KUBERNETES_SERVICE_PORT_HTTPS", "")
	t.Setenv("KUBERNETES_SERVICE_PORT", "")
	if _, err := New(Config{}); err == nil {
		t.Fatal("expected error when Endpoint is empty and no in-cluster env is set")
	}
}

func TestNewInClusterDiscovery(t *testing.T) {
	t.Setenv("KUBERNETES_SERVICE_HOST", "10.0.0.1")
	t.Setenv("KUBERNETES_SERVICE_PORT_HTTPS", "6443")
	c, err := New(Config{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c.Endpoint() != "https://10.0.0.1:6443" {
		t.Fatalf("expected in-cluster endpoint, got %q", c.Endpoint())
	}
}

func TestNewValidClientCertKeyPair(t *testing.T) {
	certPath, keyPath, _ := writeTempCertKeyPair(t)
	c, err := New(Config{Endpoint: "https://k8s.example:6443", ClientCertFile: certPath, ClientKeyFile: keyPath})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c.Endpoint() != "https://k8s.example:6443" {
		t.Fatalf("unexpected endpoint %q", c.Endpoint())
	}
}

func TestNewBadClientCertPath(t *testing.T) {
	_, err := New(Config{Endpoint: "https://k8s.example:6443", ClientCertFile: "/nonexistent/cert.pem", ClientKeyFile: "/nonexistent/key.pem"})
	if err == nil {
		t.Fatal("expected error for missing client cert/key files")
	}
}

func TestNewMissingCAFileHTTPSNotInsecure(t *testing.T) {
	_, err := New(Config{Endpoint: "https://k8s.example:6443", CAFile: "/nonexistent/ca.pem"})
	if err == nil {
		t.Fatal("expected error when CAFile is missing, scheme is https, and InsecureSkipVerify is false")
	}
}

func TestNewMissingCAFileInsecureSkipVerify(t *testing.T) {
	_, err := New(Config{Endpoint: "https://k8s.example:6443", CAFile: "/nonexistent/ca.pem", InsecureSkipVerify: true})
	if err != nil {
		t.Fatalf("expected no error for missing CAFile when InsecureSkipVerify is true, got %v", err)
	}
}

func TestNewValidCAFile(t *testing.T) {
	_, _, certPEM := writeTempCertKeyPair(t)
	dir := t.TempDir()
	caPath := filepath.Join(dir, "ca.pem")
	if err := os.WriteFile(caPath, certPEM, 0o600); err != nil {
		t.Fatalf("write CA file: %v", err)
	}
	c, err := New(Config{Endpoint: "https://k8s.example:6443", CAFile: caPath})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c == nil {
		t.Fatal("expected non-nil client")
	}
}

func newTestClient(t *testing.T, srv *httptest.Server, token string) *Client {
	t.Helper()
	c, err := New(Config{Endpoint: srv.URL, BearerToken: token})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func TestJSONDecodesSuccessResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"kind":"Pod","apiVersion":"v1"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv, "")
	var out struct{ Kind, APIVersion string }
	if err := c.JSON(context.Background(), http.MethodGet, "/api/v1/pods/foo", "", nil, &out); err != nil {
		t.Fatalf("JSON: %v", err)
	}
	if out.Kind != "Pod" {
		t.Fatalf("expected decoded Kind=Pod, got %+v", out)
	}
}

func TestJSONSetsBearerTokenHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := newTestClient(t, srv, "s3cr3t")
	if err := c.JSON(context.Background(), http.MethodGet, "/version", "", nil, nil); err != nil {
		t.Fatalf("JSON: %v", err)
	}
	if gotAuth != "Bearer s3cr3t" {
		t.Fatalf("expected Bearer token header, got %q", gotAuth)
	}
}

func TestJSONEncodesRequestBody(t *testing.T) {
	var gotContentType, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		gotBody = string(buf)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := newTestClient(t, srv, "")
	if err := c.JSON(context.Background(), http.MethodPost, "/api/v1/pods", "", map[string]string{"name": "foo"}, nil); err != nil {
		t.Fatalf("JSON: %v", err)
	}
	if gotContentType != "application/json" {
		t.Fatalf("expected application/json content type, got %q", gotContentType)
	}
	if gotBody == "" {
		t.Fatal("expected a non-empty request body")
	}
}

func TestJSONNotFoundReturnsAPIErrorWithIsNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"reason":"NotFound","message":"pods \"foo\" not found"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv, "")
	err := c.JSON(context.Background(), http.MethodGet, "/api/v1/pods/foo", "", nil, nil)
	if err == nil {
		t.Fatal("expected an error for 404 response")
	}
	if !IsNotFound(err) {
		t.Fatalf("expected IsNotFound to be true, got %v", err)
	}
	if IsConflict(err) {
		t.Fatal("expected IsConflict to be false for a 404")
	}
	if err.Error() != `pods "foo" not found` {
		t.Fatalf("expected decoded message in Error(), got %q", err.Error())
	}
}

func TestJSONConflictReturnsAPIErrorWithIsConflict(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"reason":"AlreadyExists","message":"already exists"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv, "")
	err := c.JSON(context.Background(), http.MethodPost, "/api/v1/pods", "", nil, nil)
	if !IsConflict(err) {
		t.Fatalf("expected IsConflict to be true, got %v", err)
	}
}

func TestJSONErrorFallsBackToRawBodyWhenNotJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom, not json"))
	}))
	defer srv.Close()

	c := newTestClient(t, srv, "")
	err := c.JSON(context.Background(), http.MethodGet, "/version", "", nil, nil)
	if err == nil {
		t.Fatal("expected an error for 500 response")
	}
	if err.Error() != "boom, not json" {
		t.Fatalf("expected raw body fallback in Error(), got %q", err.Error())
	}
}

func TestHealthHitsVersionEndpoint(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"major":"1","minor":"29"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv, "")
	if err := c.Health(context.Background()); err != nil {
		t.Fatalf("Health: %v", err)
	}
	if gotPath != "/version" {
		t.Fatalf("expected Health to GET /version, got %q", gotPath)
	}
}

func TestEndpointOnNilOrZeroClient(t *testing.T) {
	var c *Client
	if got := c.Endpoint(); got != "" {
		t.Fatalf("expected empty string for nil client, got %q", got)
	}
	if got := (&Client{}).Endpoint(); got != "" {
		t.Fatalf("expected empty string for zero-value client, got %q", got)
	}
}
