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
	"context"
	"net/http"
	"strings"

	"github.com/zyvorai/kryton/internal/atlas"
	"github.com/zyvorai/kryton/internal/auth"
	"github.com/zyvorai/kryton/internal/connection"
	"github.com/zyvorai/kryton/internal/doctor"
	"github.com/zyvorai/kryton/internal/kubevirt"
	"github.com/zyvorai/kryton/internal/model"
	"github.com/zyvorai/kryton/internal/settings"
	"github.com/zyvorai/kryton/internal/storage"
)

type settingsView struct {
	Runtime  settingsRuntimeView `json:"runtime"`
	System   settingsSystemView  `json:"system"`
	Doctor   model.DoctorReport  `json:"doctor"`
	Editable []string            `json:"editable"`
}

type settingsRuntimeView struct {
	DefaultProject  string       `json:"defaultProject"`
	ImageNamespace  string       `json:"imageNamespace"`
	EventWebhookURL string       `json:"eventWebhookUrl"`
	StorageClass    string       `json:"storageClass"`
	Atlas           atlas.Config `json:"atlas"`
	AtlasTokenSet   bool         `json:"atlasTokenSet"`
}

type settingsSystemView struct {
	Provider            string   `json:"provider"`
	AuthMode            string   `json:"authMode"`
	Projects            []string `json:"projects"`
	DefaultProjectEnv   string   `json:"defaultProjectEnv"`
	ImageNamespaceEnv   string   `json:"imageNamespaceEnv"`
	NamespacePrefix     string   `json:"namespacePrefix"`
	KubernetesEndpoint  string   `json:"kubernetesEndpoint,omitempty"`
	KubernetesConnected bool     `json:"kubernetesConnected"`
	ScriptsAvailable    bool     `json:"scriptsAvailable"`
	AllowInsecure       bool     `json:"allowInsecure"`
	StorageConfigFile   string   `json:"storageConfigFile,omitempty"`
	SettingsConfigFile  string   `json:"settingsConfigFile,omitempty"`
}

type settingsUpdateRequest struct {
	DefaultProject  *string       `json:"defaultProject,omitempty"`
	ImageNamespace  *string       `json:"imageNamespace,omitempty"`
	EventWebhookURL *string       `json:"eventWebhookUrl,omitempty"`
	StorageClass    *string       `json:"storageClass,omitempty"`
	Atlas           *atlas.Config `json:"atlas,omitempty"`
}

type settingsTestResponse struct {
	Healthy    bool               `json:"healthy"`
	Doctor     model.DoctorReport `json:"doctor"`
	Connection connection.Result  `json:"connection"`
}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, http.StatusOK, s.buildSettingsView(r))
}

func (s *Server) putSettings(w http.ResponseWriter, r *http.Request) {
	if p := auth.FromContext(r.Context()); p.Role != auth.Admin && p.Role != auth.Operator {
		s.forbidden(w, r)
		return
	}
	var req settingsUpdateRequest
	if err := decodeJSON(w, r, &req); err != nil {
		s.badRequest(w, r, err.Error())
		return
	}
	if req.StorageClass != nil {
		if err := s.updateStorageClass(r.Context(), strings.TrimSpace(*req.StorageClass)); err != nil {
			s.badRequest(w, r, err.Error())
			return
		}
	}
	rt := s.currentRuntimeSettings()
	if req.DefaultProject != nil {
		p := strings.TrimSpace(*req.DefaultProject)
		if p == "" {
			s.badRequest(w, r, "defaultProject cannot be empty")
			return
		}
		if !containsString(s.projects, p) {
			s.badRequest(w, r, "defaultProject must be one of: "+strings.Join(s.projects, ", "))
			return
		}
		rt.DefaultProject = p
		s.defaultProject = p
	}
	if req.ImageNamespace != nil {
		ns := strings.TrimSpace(*req.ImageNamespace)
		if ns == "" {
			s.badRequest(w, r, "imageNamespace cannot be empty")
			return
		}
		if err := model.ValidateProject(ns); err != nil {
			s.badRequest(w, r, "invalid imageNamespace: "+err.Error())
			return
		}
		rt.ImageNamespace = ns
		s.imageNamespace = ns
		if kv, ok := s.p.(*kubevirt.Provider); ok {
			kv.SetImageNamespace(ns)
		}
	}
	if req.EventWebhookURL != nil {
		rt.EventWebhookURL = strings.TrimSpace(*req.EventWebhookURL)
		if s.events != nil {
			s.events.SetWebhookURL(rt.EventWebhookURL)
		}
	}
	if req.Atlas != nil {
		a := *req.Atlas
		a.BaseURL = strings.TrimRight(strings.TrimSpace(a.BaseURL), "/")
		a.Token = strings.TrimSpace(a.Token)
		a.Product = strings.TrimSpace(a.Product)
		if a.Product == "" {
			a.Product = atlas.ProductID
		}
		if a.Enabled && a.BaseURL == "" {
			s.badRequest(w, r, "atlas.baseUrl is required when Atlas integration is enabled")
			return
		}
		// Keep previous token when client sends empty (UI password field leave-blank).
		if a.Token == "" && rt.Atlas.Token != "" {
			a.Token = rt.Atlas.Token
		}
		rt.Atlas = a
	}
	if s.settingsStore != nil {
		if err := s.settingsStore.Save(rt); err != nil {
			s.writeErr(w, r, err)
			return
		}
	}
	s.metrics.Operation("settings-config")
	s.events.Publish(r.Context(), "io.kryton.settings.updated", "settings", map[string]any{
		"defaultProject": rt.DefaultProject, "imageNamespace": rt.ImageNamespace,
	})
	jsonResponse(w, http.StatusOK, s.buildSettingsView(r))
}

func (s *Server) postSettingsTest(w http.ResponseWriter, r *http.Request) {
	report := s.runDoctor(r)
	script := ""
	if s.storageSetup != nil {
		script = s.storageSetup.SnapshotsScript()
	}
	conn := connection.Test(r.Context(), connection.Input{
		Provider:        s.p,
		KubeClient:      s.kubeClient,
		StorageClass:    s.currentStorageConfig().StorageClass,
		ScriptsOK:       s.storageSetup != nil && s.storageSetup.Available(),
		SnapshotsScript: script,
	})
	resp := settingsTestResponse{Doctor: report, Connection: conn, Healthy: report.Healthy && conn.Healthy}
	status := http.StatusOK
	if !resp.Healthy {
		status = http.StatusServiceUnavailable
	}
	jsonResponse(w, status, resp)
}

func (s *Server) postAtlasTest(w http.ResponseWriter, r *http.Request) {
	cfg := s.currentRuntimeSettings().Atlas
	var override atlas.Config
	err := decodeJSON(w, r, &override)
	if err == nil {
		if strings.TrimSpace(override.BaseURL) != "" || override.Enabled || strings.TrimSpace(override.Token) != "" {
			if strings.TrimSpace(override.BaseURL) == "" {
				override.BaseURL = cfg.BaseURL
			}
			if strings.TrimSpace(override.Token) == "" {
				override.Token = cfg.Token
			}
			if strings.TrimSpace(override.Product) == "" {
				override.Product = cfg.Product
			}
			override.Enabled = true
			cfg = override
		}
	} else if r.ContentLength > 0 {
		s.badRequest(w, r, err.Error())
		return
	}
	res := atlas.Test(r.Context(), cfg)
	status := http.StatusOK
	if cfg.Enabled && !res.Healthy {
		status = http.StatusServiceUnavailable
	}
	jsonResponse(w, status, res)
}

func (s *Server) buildSettingsView(r *http.Request) settingsView {
	rt := s.currentRuntimeSettings()
	atlasCfg := rt.Atlas
	tokenSet := atlasCfg.Token != ""
	if atlasCfg.Product == "" {
		atlasCfg.Product = atlas.ProductID
	}
	atlasView := atlasCfg
	atlasView.Token = ""
	return settingsView{
		Runtime: settingsRuntimeView{
			DefaultProject:  firstNonEmptyStr(rt.DefaultProject, s.defaultProject),
			ImageNamespace:  firstNonEmptyStr(rt.ImageNamespace, s.imageNamespace),
			EventWebhookURL: rt.EventWebhookURL,
			StorageClass:    s.currentStorageConfig().StorageClass,
			Atlas:           atlasView,
			AtlasTokenSet:   tokenSet,
		},
		System: settingsSystemView{
			Provider:            s.p.Name(),
			AuthMode:            s.authMode,
			Projects:            s.projects,
			DefaultProjectEnv:   s.defaultProjectEnv,
			ImageNamespaceEnv:   s.imageNamespaceEnv,
			NamespacePrefix:     s.namespacePrefix,
			KubernetesEndpoint:  s.kubernetesEndpointDisplay(),
			KubernetesConnected: s.kubeClient != nil,
			ScriptsAvailable:    s.storageSetup != nil && s.storageSetup.Available(),
			AllowInsecure:       s.allowInsecure,
			StorageConfigFile:   s.storageConfigPath,
			SettingsConfigFile:  s.settingsConfigPath,
		},
		Doctor:   s.runDoctor(r),
		Editable: s.settingsEditableFields(),
	}
}

func (s *Server) runDoctor(r *http.Request) model.DoctorReport {
	return doctor.Run(r.Context(), doctor.Input{
		Provider:        s.p,
		Catalog:         s.catalog,
		AuthMode:        s.authMode,
		Projects:        s.projects,
		DockurDir:       s.dockurDataDir,
		Runtime:         s.dockurRuntime,
		KubeClient:      s.kubeClient,
		ImageNamespace:  firstNonEmptyStr(s.currentRuntimeSettings().ImageNamespace, s.imageNamespace),
		NamespacePrefix: s.namespacePrefix,
		StorageClass:    s.currentStorageConfig().StorageClass,
	})
}

func (s *Server) currentRuntimeSettings() settings.Runtime {
	if s.settingsStore != nil {
		return s.settingsStore.Get()
	}
	return settings.Runtime{}
}

func (s *Server) settingsEditableFields() []string {
	out := []string{"defaultProject", "eventWebhookUrl", "storageClass", "atlas"}
	if s.p.Name() == "kubevirt" {
		out = append(out, "imageNamespace")
	}
	return out
}

func (s *Server) updateStorageClass(ctx context.Context, sc string) error {
	if sc != "" {
		if s.kubeClient == nil {
			return errString("kubernetes client is not available")
		}
		if s.p.Name() != "kubevirt" {
			return errString("storage class applies to kubevirt provider only")
		}
		inv, err := storage.LoadInventory(ctx, s.kubeClient, storage.Config{}, s.p.Name())
		if err != nil {
			return err
		}
		found := false
		var chosen storage.Class
		for _, c := range inv.StorageClasses {
			if c.Name == sc {
				found, chosen = true, c
				break
			}
		}
		if !found {
			return errString("storage class " + sc + " not found in cluster")
		}
		if chosen.Backend == "local-path" {
			return errString("local-path cannot CSI-snapshot VM disks; pick rook-ceph-block or longhorn")
		}
	}
	cfg := storage.Config{StorageClass: sc}
	if s.storageStore != nil {
		if err := s.storageStore.Save(cfg); err != nil {
			return err
		}
	}
	s.applyStorageClass(sc)
	return nil
}

func (s *Server) kubernetesEndpointDisplay() string {
	if s.kubeClient == nil {
		return ""
	}
	ep := s.kubeClient.Endpoint()
	if ep == "" {
		return "kubeconfig"
	}
	return ep
}

func containsString(items []string, v string) bool {
	for _, x := range items {
		if x == v {
			return true
		}
	}
	return false
}

func firstNonEmptyStr(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

type errString string

func (e errString) Error() string { return string(e) }
