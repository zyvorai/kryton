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

package atlas

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const ProductID = "kryton"

// Config is how Kryton reaches Atlas (Zyvor storage control plane).
type Config struct {
	Enabled     bool   `json:"enabled"`
	BaseURL     string `json:"baseUrl"`
	Token       string `json:"token,omitempty"`
	Product     string `json:"product,omitempty"` // owner product id (default kryton)
	PreferAtlas bool   `json:"preferAtlas,omitempty"`
}

// Probe is one Atlas connectivity check.
type Probe struct {
	Name      string `json:"name"`
	Status    string `json:"status"` // pass | warn | fail
	Message   string `json:"message"`
	Hint      string `json:"hint,omitempty"`
	LatencyMs int64  `json:"latencyMs,omitempty"`
}

// TestResult is returned by POST /api/v1/integrations/atlas/test.
type TestResult struct {
	Healthy        bool     `json:"healthy"`
	Configured     bool     `json:"configured"`
	BaseURL        string   `json:"baseUrl,omitempty"`
	Product        string   `json:"product,omitempty"`
	Probes         []Probe  `json:"probes"`
	StorageClasses []string `json:"storageClasses,omitempty"`
	TestedAt       string   `json:"testedAt"`
}

// Client talks to Atlas REST (`/api/atlas/v1` + `/readyz`).
type Client struct {
	http    *http.Client
	baseURL string
	token   string
}

func NewClient(cfg Config) *Client {
	base := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	return &Client{
		http:    &http.Client{Timeout: 12 * time.Second},
		baseURL: base,
		token:   strings.TrimSpace(cfg.Token),
	}
}

func (c *Client) Configured() bool {
	return c != nil && c.baseURL != ""
}

func Test(ctx context.Context, cfg Config) TestResult {
	now := time.Now().UTC().Format(time.RFC3339)
	product := strings.TrimSpace(cfg.Product)
	if product == "" {
		product = ProductID
	}
	out := TestResult{
		Configured: cfg.Enabled && strings.TrimSpace(cfg.BaseURL) != "",
		BaseURL:    strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"),
		Product:    product,
		TestedAt:   now,
	}
	if !cfg.Enabled {
		out.Probes = []Probe{{Name: "atlas", Status: "warn", Message: "Atlas integration is disabled", Hint: "Enable Integrate Atlas in Settings"}}
		return out
	}
	if out.BaseURL == "" {
		out.Probes = []Probe{{Name: "atlas", Status: "fail", Message: "Atlas base URL is empty", Hint: "Set e.g. http://127.0.0.1:5110 or https://atlas.example"}}
		return out
	}

	cl := NewClient(cfg)
	out.Probes = append(out.Probes, cl.probeReadyz(ctx))
	out.Probes = append(out.Probes, cl.probeVersion(ctx))
	scProbe, classes := cl.probeStorageClasses(ctx)
	out.Probes = append(out.Probes, scProbe)
	out.StorageClasses = classes

	out.Healthy = true
	for _, p := range out.Probes {
		if p.Status == "fail" {
			out.Healthy = false
			break
		}
	}
	return out
}

func (c *Client) probeReadyz(ctx context.Context) Probe {
	t0 := time.Now()
	code, body, err := c.get(ctx, "/readyz")
	ms := time.Since(t0).Milliseconds()
	if err != nil {
		return Probe{Name: "readyz", Status: "fail", Message: err.Error(), Hint: "Check ATLAS_BIND_ADDR / network path from krytond", LatencyMs: ms}
	}
	if code >= 500 {
		return Probe{Name: "readyz", Status: "fail", Message: fmt.Sprintf("HTTP %d", code), Hint: truncate(body, 160), LatencyMs: ms}
	}
	if code >= 400 {
		return Probe{Name: "readyz", Status: "warn", Message: fmt.Sprintf("HTTP %d", code), Hint: truncate(body, 160), LatencyMs: ms}
	}
	return Probe{Name: "readyz", Status: "pass", Message: "Atlas ready", LatencyMs: ms}
}

func (c *Client) probeVersion(ctx context.Context) Probe {
	t0 := time.Now()
	code, body, err := c.get(ctx, "/version")
	ms := time.Since(t0).Milliseconds()
	if err != nil {
		// /version may be under /api/atlas/v1 on some builds — try both.
		code, body, err = c.get(ctx, "/api/atlas/v1/version")
		ms = time.Since(t0).Milliseconds()
		if err != nil {
			return Probe{Name: "version", Status: "warn", Message: "version endpoint unavailable", Hint: err.Error(), LatencyMs: ms}
		}
	}
	if code >= 400 {
		return Probe{Name: "version", Status: "warn", Message: fmt.Sprintf("HTTP %d", code), LatencyMs: ms}
	}
	var v map[string]any
	_ = json.Unmarshal([]byte(body), &v)
	msg := "Atlas responding"
	if ver, ok := v["version"].(string); ok && ver != "" {
		msg = "Atlas " + ver
	} else if ver, ok := v["git_version"].(string); ok && ver != "" {
		msg = "Atlas " + ver
	}
	return Probe{Name: "version", Status: "pass", Message: msg, LatencyMs: ms}
}

func (c *Client) probeStorageClasses(ctx context.Context) (Probe, []string) {
	t0 := time.Now()
	code, body, err := c.get(ctx, "/api/atlas/v1/storage-classes")
	ms := time.Since(t0).Milliseconds()
	if err != nil {
		return Probe{Name: "storage-classes", Status: "fail", Message: err.Error(), Hint: "Atlas needs a live Kubernetes driver for StorageClasses", LatencyMs: ms}, nil
	}
	if code == 401 || code == 403 {
		return Probe{Name: "storage-classes", Status: "fail", Message: "Unauthorized — set a valid Atlas bearer token", Hint: "Mint with POST /api/atlas/v1/auth/tokens (product.service.kryton)", LatencyMs: ms}, nil
	}
	if code == 502 {
		return Probe{Name: "storage-classes", Status: "warn", Message: "No Kubernetes driver on Atlas", Hint: "Atlas still usable for Ceph inventory; StorageClass sync unavailable", LatencyMs: ms}, nil
	}
	if code >= 400 {
		return Probe{Name: "storage-classes", Status: "warn", Message: fmt.Sprintf("HTTP %d", code), Hint: truncate(body, 160), LatencyMs: ms}, nil
	}
	names := parseStorageClassNames(body)
	return Probe{Name: "storage-classes", Status: "pass", Message: fmt.Sprintf("%d StorageClass(es) via Atlas", len(names)), LatencyMs: ms}, names
}

func parseStorageClassNames(body string) []string {
	var wrapped struct {
		Items []struct {
			Name string `json:"name"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(body), &wrapped); err == nil && len(wrapped.Items) > 0 {
		var out []string
		for _, it := range wrapped.Items {
			if it.Name != "" {
				out = append(out, it.Name)
			}
		}
		return out
	}
	var arr []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(body), &arr); err == nil {
		var out []string
		for _, it := range arr {
			if it.Name != "" {
				out = append(out, it.Name)
			}
		}
		return out
	}
	return nil
}

func (c *Client) get(ctx context.Context, path string) (int, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Accept", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	return res.StatusCode, string(b), nil
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
