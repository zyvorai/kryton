package api

import (
	"net/http"

	"github.com/zyvorai/kryton/internal/auth"
	"github.com/zyvorai/kryton/internal/golden"
	"github.com/zyvorai/kryton/internal/model"
)

func (s *Server) goldenList(w http.ResponseWriter, r *http.Request) {
	if s.golden == nil {
		jsonResponse(w, http.StatusOK, listResponse[model.GoldenBuild]{Items: []model.GoldenBuild{}})
		return
	}
	items, err := s.golden.List()
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	if items == nil {
		items = []model.GoldenBuild{}
	}
	jsonResponse(w, http.StatusOK, listResponse[model.GoldenBuild]{Items: items})
}

func (s *Server) goldenGet(w http.ResponseWriter, r *http.Request) {
	if s.golden == nil {
		s.writeAPIError(w, r, http.StatusNotFound, "not_found", "golden image builder not available on this instance")
		return
	}
	b, err := s.golden.Get(r.PathValue("id"))
	if err != nil {
		s.writeAPIError(w, r, http.StatusNotFound, "not_found", "golden build not found")
		return
	}
	jsonResponse(w, http.StatusOK, b)
}

func (s *Server) goldenStart(w http.ResponseWriter, r *http.Request) {
	if s.golden == nil {
		s.writeAPIError(w, r, http.StatusServiceUnavailable, "unavailable", "golden image builder requires docker and /dev/kvm on the krytond host")
		return
	}
	if _, ok := s.requireProject(w, r, auth.Operator); !ok {
		return
	}
	var req model.GoldenStartRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	if !req.Auto {
		req.Auto = true
	}
	b, err := s.golden.Start(r.Context(), req)
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	jsonResponse(w, http.StatusAccepted, b)
}

func goldenEnabled(m *golden.Manager) bool {
	if m == nil {
		return false
	}
	return m.Available() == nil
}
