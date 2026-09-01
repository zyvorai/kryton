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
	_ "embed"
	"net/http"
	"strings"
)

//go:embed openapi.yaml
var openAPISpec []byte

type apiCatalog struct {
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	Description string            `json:"description"`
	OpenAPI     string            `json:"openapi"`
	BasePath    string            `json:"basePath"`
	Auth        apiCatalogAuth    `json:"auth"`
	Health      map[string]string `json:"health"`
	Endpoints   []apiEndpoint     `json:"endpoints"`
}

type apiCatalogAuth struct {
	Mode    string   `json:"mode"`
	Schemes []string `json:"schemes"`
	Header  string   `json:"header,omitempty"`
}

type apiEndpoint struct {
	Method      string `json:"method"`
	Path        string `json:"path"`
	Group       string `json:"group"`
	Description string `json:"description"`
}

func (s *Server) serveOpenAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.Header().Set("Access-Control-Allow-Origin", corsAllowOrigin(r, s.corsOrigins))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(openAPISpec)
}

func (s *Server) apiDiscovery(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, http.StatusOK, apiCatalog{
		Name:        "kryton",
		Version:     "1.0.0",
		Description: "Kryton Windows virtualization control plane",
		OpenAPI:     "/openapi.yaml",
		BasePath:    "/api/v1",
		Auth: apiCatalogAuth{
			Mode:    s.authMode,
			Schemes: []string{"bearer", "proxy"},
			Header:  "Authorization",
		},
		Health: map[string]string{
			"live":    "/healthz",
			"ready":   "/readyz",
			"metrics": "/metrics",
		},
		Endpoints: allAPIEndpoints(),
	})
}

func allAPIEndpoints() []apiEndpoint {
	return []apiEndpoint{
		{Method: "GET", Path: "/api/v1", Group: "meta", Description: "API discovery"},
		{Method: "GET", Path: "/api/v1/me", Group: "meta", Description: "Current identity"},
		{Method: "GET", Path: "/api/v1/projects", Group: "meta", Description: "Accessible projects"},
		{Method: "GET", Path: "/api/v1/capabilities", Group: "meta", Description: "Provider capabilities"},
		{Method: "GET", Path: "/api/v1/doctor", Group: "meta", Description: "Environment diagnostics"},
		{Method: "GET", Path: "/api/v1/settings", Group: "settings", Description: "Runtime settings"},
		{Method: "PUT", Path: "/api/v1/settings", Group: "settings", Description: "Update runtime settings"},
		{Method: "POST", Path: "/api/v1/settings/test", Group: "settings", Description: "Test control-plane connection"},
		{Method: "POST", Path: "/api/v1/integrations/atlas/test", Group: "integrations", Description: "Test Atlas storage control plane"},
		{Method: "GET", Path: "/api/v1/storage", Group: "storage", Description: "Storage inventory"},
		{Method: "GET", Path: "/api/v1/storage/config", Group: "storage", Description: "Default StorageClass"},
		{Method: "PUT", Path: "/api/v1/storage/config", Group: "storage", Description: "Set default StorageClass"},
		{Method: "GET", Path: "/api/v1/storage/setup", Group: "storage", Description: "Storage install status"},
		{Method: "POST", Path: "/api/v1/storage/setup", Group: "storage", Description: "Install Longhorn or Rook"},
		{Method: "GET", Path: "/api/v1/images", Group: "images", Description: "Image catalog"},
		{Method: "GET", Path: "/api/v1/golden", Group: "images", Description: "Golden builds"},
		{Method: "POST", Path: "/api/v1/golden", Group: "images", Description: "Start golden build"},
		{Method: "GET", Path: "/api/v1/golden/{id}", Group: "images", Description: "Golden build"},
		{Method: "POST", Path: "/api/v1/golden/{id}/bootstrap", Group: "images", Description: "Publish to CDI"},
		{Method: "GET", Path: "/api/v1/jobs", Group: "jobs", Description: "List jobs"},
		{Method: "GET", Path: "/api/v1/jobs/{id}", Group: "jobs", Description: "Get job"},
		{Method: "GET", Path: "/api/v1/summary", Group: "machines", Description: "Project summary"},
		{Method: "GET", Path: "/api/v1/machines", Group: "machines", Description: "List machines"},
		{Method: "POST", Path: "/api/v1/machines", Group: "machines", Description: "Create machine"},
		{Method: "GET", Path: "/api/v1/machines/{id}", Group: "machines", Description: "Get machine"},
		{Method: "DELETE", Path: "/api/v1/machines/{id}", Group: "machines", Description: "Delete machine"},
		{Method: "POST", Path: "/api/v1/machines/{id}/start", Group: "machines", Description: "Start machine"},
		{Method: "POST", Path: "/api/v1/machines/{id}/stop", Group: "machines", Description: "Stop machine"},
		{Method: "POST", Path: "/api/v1/machines/{id}/snapshot", Group: "machines", Description: "Create snapshot"},
		{Method: "GET", Path: "/api/v1/machines/{id}/snapshots", Group: "machines", Description: "List snapshots"},
		{Method: "POST", Path: "/api/v1/machines/{id}/snapshots/{sid}/restore", Group: "machines", Description: "Restore snapshot"},
		{Method: "DELETE", Path: "/api/v1/machines/{id}/snapshots/{sid}", Group: "machines", Description: "Delete snapshot"},
		{Method: "GET", Path: "/api/v1/machines/{id}/console", Group: "machines", Description: "Web console"},
		{Method: "GET", Path: "/api/v1/machines/{id}/vnc", Group: "machines", Description: "VNC proxy"},
		{Method: "GET", Path: "/api/v1/events", Group: "events", Description: "Event history"},
		{Method: "GET", Path: "/api/v1/events/stream", Group: "events", Description: "SSE event stream"},
		{Method: "GET", Path: "/healthz", Group: "health", Description: "Liveness"},
		{Method: "GET", Path: "/readyz", Group: "health", Description: "Readiness"},
		{Method: "GET", Path: "/metrics", Group: "health", Description: "Prometheus metrics"},
		{Method: "GET", Path: "/openapi.yaml", Group: "meta", Description: "OpenAPI 3.1 spec"},
	}
}

func corsAllowOrigin(r *http.Request, allowed []string) string {
	if len(allowed) == 0 {
		return ""
	}
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	for _, a := range allowed {
		a = strings.TrimSpace(a)
		if a == "*" {
			if origin != "" {
				return origin
			}
			return "*"
		}
		if origin != "" && strings.EqualFold(a, origin) {
			return origin
		}
	}
	return ""
}
