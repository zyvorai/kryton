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

// Package kubeapi is a minimal Kubernetes REST API client used by the
// kubevirt provider and doctor checks: it talks directly to the
// apiserver over HTTP(S) with bearer-token, client-cert, or in-cluster
// service-account auth, without depending on client-go.
package kubeapi

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Config points New at a Kubernetes API server. Leaving Endpoint empty
// makes New try in-cluster discovery (KUBERNETES_SERVICE_HOST/_PORT);
// BearerToken takes priority over TokenFile when both are set.
type Config struct {
	Endpoint           string
	BearerToken        string
	TokenFile          string
	CAFile             string
	ClientCertFile     string
	ClientKeyFile      string
	InsecureSkipVerify bool
}

// Client is a thin, dependency-free Kubernetes API client: one base URL,
// one bearer token, one http.Client. Safe for concurrent use.
type Client struct {
	base  *url.URL
	token string
	http  *http.Client
}

// APIError is returned by JSON for any non-2xx Kubernetes API response,
// carrying the raw StatusCode plus the decoded Status.reason/message
// when the body was a Kubernetes Status object. Use IsNotFound/IsConflict
// to test for the common cases.
type APIError struct {
	StatusCode int
	Reason     string
	Message    string
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("kubernetes API returned HTTP %d", e.StatusCode)
}

// New builds a Client for cfg. With cfg.Endpoint empty it falls back to
// in-cluster discovery via the KUBERNETES_SERVICE_HOST/_PORT_HTTPS
// environment, returning an error if neither is set. TLS client
// certificates and a custom CA are loaded eagerly, so a bad cert/key/CA
// path fails here rather than on the first request.
func New(cfg Config) (*Client, error) {
	endpoint := strings.TrimRight(strings.TrimSpace(cfg.Endpoint), "/")
	if endpoint == "" {
		host := os.Getenv("KUBERNETES_SERVICE_HOST")
		port := os.Getenv("KUBERNETES_SERVICE_PORT_HTTPS")
		if port == "" {
			port = os.Getenv("KUBERNETES_SERVICE_PORT")
		}
		if host == "" {
			return nil, errors.New("Kubernetes endpoint is unset and in-cluster service environment is unavailable")
		}
		if port == "" {
			port = "443"
		}
		endpoint = "https://" + host + ":" + port
	}
	base, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("parse Kubernetes endpoint: %w", err)
	}

	token := strings.TrimSpace(cfg.BearerToken)
	if token == "" && cfg.TokenFile != "" {
		if b, err := os.ReadFile(cfg.TokenFile); err == nil {
			token = strings.TrimSpace(string(b))
		}
	}

	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: cfg.InsecureSkipVerify} //nolint:gosec -- explicit operator setting
	if cfg.ClientCertFile != "" && cfg.ClientKeyFile != "" {
		cert, err := tls.LoadX509KeyPair(cfg.ClientCertFile, cfg.ClientKeyFile)
		if err != nil {
			return nil, fmt.Errorf("load Kubernetes client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}
	if cfg.CAFile != "" {
		if pem, err := os.ReadFile(cfg.CAFile); err == nil {
			pool, err := x509.SystemCertPool()
			if err != nil || pool == nil {
				pool = x509.NewCertPool()
			}
			if !pool.AppendCertsFromPEM(pem) {
				return nil, fmt.Errorf("no certificates found in %s", cfg.CAFile)
			}
			tlsConfig.RootCAs = pool
		} else if base.Scheme == "https" && !cfg.InsecureSkipVerify {
			return nil, fmt.Errorf("read Kubernetes CA file: %w", err)
		}
	}
	transport := &http.Transport{Proxy: http.ProxyFromEnvironment, TLSClientConfig: tlsConfig, MaxIdleConns: 100, IdleConnTimeout: 90 * time.Second, TLSHandshakeTimeout: 10 * time.Second}
	return &Client{base: base, token: token, http: &http.Client{Transport: transport, Timeout: 20 * time.Second}}, nil
}

// Endpoint returns the API server base URL c talks to, or "" for a nil
// or zero-value Client — used to display the configured cluster in Settings.
func (c *Client) Endpoint() string {
	if c == nil || c.base == nil {
		return ""
	}
	return c.base.String()
}

// Health reports whether the API server responds to GET /version.
func (c *Client) Health(ctx context.Context) error {
	var out map[string]any
	return c.JSON(ctx, http.MethodGet, "/version", "", nil, &out)
}

// JSON performs one Kubernetes API request: path may include a query
// string (split automatically), body — if non-nil — is JSON-encoded as
// the request payload (contentType defaults to application/json), and a
// non-nil out receives the JSON-decoded response body. A non-2xx status
// is returned as *APIError rather than a decode attempt.
func (c *Client) JSON(ctx context.Context, method, path, contentType string, body, out any) error {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		r = bytes.NewReader(b)
	}
	u := *c.base
	u.Path = strings.TrimRight(c.base.Path, "/") + pathOnly(path)
	u.RawQuery = queryOnly(path)
	req, err := http.NewRequestWithContext(ctx, method, u.String(), r)
	if err != nil {
		return err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if body != nil {
		if contentType == "" {
			contentType = "application/json"
		}
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "kryton/1.0")
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		apiErr := &APIError{StatusCode: res.StatusCode}
		var status struct{ Reason, Message string }
		if json.Unmarshal(payload, &status) == nil {
			apiErr.Reason, apiErr.Message = status.Reason, status.Message
		}
		if apiErr.Message == "" {
			apiErr.Message = strings.TrimSpace(string(payload))
		}
		return apiErr
	}
	if out != nil && len(payload) > 0 {
		if err := json.Unmarshal(payload, out); err != nil {
			return fmt.Errorf("decode Kubernetes response: %w", err)
		}
	}
	return nil
}

// IsNotFound reports whether err is an *APIError for HTTP 404.
func IsNotFound(err error) bool {
	var e *APIError
	return errors.As(err, &e) && e.StatusCode == http.StatusNotFound
}

// IsConflict reports whether err is an *APIError for HTTP 409.
func IsConflict(err error) bool {
	var e *APIError
	return errors.As(err, &e) && e.StatusCode == http.StatusConflict
}

func pathOnly(v string) string {
	if i := strings.IndexByte(v, '?'); i >= 0 {
		return v[:i]
	}
	return v
}
func queryOnly(v string) string {
	if i := strings.IndexByte(v, '?'); i >= 0 {
		return v[i+1:]
	}
	return ""
}
