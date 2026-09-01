package api

import (
	"net/http"
	"strings"

	"github.com/zyvorai/kryton/internal/atlas"
	"github.com/zyvorai/kryton/internal/auth"
	"github.com/zyvorai/kryton/internal/kubevirt"
	"github.com/zyvorai/kryton/internal/storage"
)

type storageConfigRequest struct {
	StorageClass string `json:"storageClass"`
}

func (s *Server) getStorage(w http.ResponseWriter, r *http.Request) {
	inv, err := s.loadStorageInventory(r)
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	jsonResponse(w, http.StatusOK, inv)
}

func (s *Server) loadStorageInventory(r *http.Request) (storage.Inventory, error) {
	cfg := s.currentStorageConfig()
	inv, err := storage.LoadInventory(r.Context(), s.kubeClient, cfg, s.p.Name())
	if err != nil {
		return inv, err
	}
	inv.BackendsInstalled = map[string]bool{}
	for _, c := range inv.StorageClasses {
		if c.Backend == "rook-ceph" || c.Backend == "longhorn" {
			inv.BackendsInstalled[c.Backend] = true
		}
	}
	if s.storageSetup != nil {
		inv.ScriptsAvailable = s.storageSetup.Available()
		if st, err := s.storageSetup.Get(); err == nil && st != nil {
			inv.Setup = st
		}
		if inv.ScriptsAvailable && s.p.Name() == "kubevirt" {
			inv.BlockDevices = storage.ListBlockDevices(r.Context())
		}
	}
	rt := s.currentRuntimeSettings()
	if rt.Atlas.Enabled && rt.Atlas.PreferAtlas && strings.TrimSpace(rt.Atlas.BaseURL) != "" {
		res := atlas.Test(r.Context(), rt.Atlas)
		inv.Atlas = &storage.AtlasHint{
			Enabled:        true,
			BaseURL:        rt.Atlas.BaseURL,
			Healthy:        res.Healthy,
			StorageClasses: res.StorageClasses,
			Product:        firstNonEmptyStr(rt.Atlas.Product, atlas.ProductID),
		}
		if len(res.StorageClasses) > 0 {
			prefer := map[string]bool{}
			for _, n := range res.StorageClasses {
				prefer[n] = true
			}
			for i := range inv.StorageClasses {
				if prefer[inv.StorageClasses[i].Name] {
					inv.StorageClasses[i].Recommended = true
				}
			}
		}
	}
	return inv, nil
}

func (s *Server) getStorageConfig(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, http.StatusOK, s.currentStorageConfig())
}

func (s *Server) putStorageConfig(w http.ResponseWriter, r *http.Request) {
	p := auth.FromContext(r.Context())
	allowed := p.Role == auth.Admin || p.Role == auth.Operator
	if !allowed {
		s.forbidden(w, r)
		return
	}
	if s.p.Name() != "kubevirt" {
		s.writeAPIError(w, r, http.StatusNotImplemented, "UNSUPPORTED", "storage configuration requires the kubevirt provider")
		return
	}
	var req storageConfigRequest
	if err := decodeJSON(w, r, &req); err != nil {
		s.badRequest(w, r, err.Error())
		return
	}
	req.StorageClass = strings.TrimSpace(req.StorageClass)
	if err := s.updateStorageClass(r.Context(), req.StorageClass); err != nil {
		s.badRequest(w, r, err.Error())
		return
	}
	s.metrics.Operation("storage-config")
	s.events.Publish(r.Context(), "io.kryton.storage.configured", "storage/config", map[string]any{"storageClass": req.StorageClass})
	jsonResponse(w, http.StatusOK, storage.Config{StorageClass: req.StorageClass})
}

func (s *Server) getStorageSetup(w http.ResponseWriter, r *http.Request) {
	if s.storageSetup == nil {
		jsonResponse(w, http.StatusOK, map[string]any{"state": "idle", "logs": []string{}})
		return
	}
	st, err := s.storageSetup.Get()
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{
		"setup": st,
		"logs":  s.storageSetup.Logs(200),
	})
}

func (s *Server) postStorageSetup(w http.ResponseWriter, r *http.Request) {
	p := auth.FromContext(r.Context())
	if p.Role != auth.Admin && p.Role != auth.Operator {
		s.forbidden(w, r)
		return
	}
	if s.p.Name() != "kubevirt" {
		s.writeAPIError(w, r, http.StatusNotImplemented, "UNSUPPORTED", "storage setup requires the kubevirt provider")
		return
	}
	if s.storageSetup == nil || !s.storageSetup.Available() {
		s.badRequest(w, r, "storage setup scripts are not available on this host (run krytond on the cluster node with scripts/)")
		return
	}
	var req storage.SetupRequest
	if err := decodeJSON(w, r, &req); err != nil {
		s.badRequest(w, r, err.Error())
		return
	}
	if !req.SetDefault {
		req.SetDefault = true
	}
	st, err := s.storageSetup.Start(req)
	if err != nil {
		s.badRequest(w, r, err.Error())
		return
	}
	s.metrics.Operation("storage-setup")
	s.events.Publish(r.Context(), "io.kryton.storage.setup.started", "storage/setup", map[string]any{
		"backend": req.Backend, "rookMode": req.RookMode, "device": req.Device,
	})
	jsonResponse(w, http.StatusAccepted, st)
}

func (s *Server) currentStorageConfig() storage.Config {
	if s.storageStore != nil {
		return s.storageStore.Get()
	}
	if kv, ok := s.p.(*kubevirt.Provider); ok {
		return storage.Config{StorageClass: kv.StorageClass()}
	}
	return storage.Config{StorageClass: s.storageClass}
}

func (s *Server) applyStorageClass(name string) {
	s.storageClass = name
	if kv, ok := s.p.(*kubevirt.Provider); ok {
		kv.SetStorageClass(name)
	}
}

func (s *Server) onStorageSetupComplete(storageClass string, setDefault bool) {
	if !setDefault || storageClass == "" {
		return
	}
	cfg := storage.Config{StorageClass: storageClass}
	if s.storageStore != nil {
		_ = s.storageStore.Save(cfg)
	}
	s.applyStorageClass(storageClass)
}
