package api

import (
	"errors"
	"net/http"
	"os"

	"github.com/zyvorai/kryton/internal/model"
)

func (s *Server) listJobs(w http.ResponseWriter, r *http.Request) {
	if s.jobs == nil {
		jsonResponse(w, http.StatusOK, listResponse[model.Job]{Items: []model.Job{}})
		return
	}
	items, err := s.jobs.List(r.Context())
	if err != nil {
		s.writeErr(w, r, err)
		return
	}
	if items == nil {
		items = []model.Job{}
	}
	jsonResponse(w, http.StatusOK, listResponse[model.Job]{Items: items})
}

func (s *Server) getJob(w http.ResponseWriter, r *http.Request) {
	if s.jobs == nil {
		s.writeAPIError(w, r, http.StatusNotFound, "not_found", "jobs not available")
		return
	}
	j, err := s.jobs.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			s.writeAPIError(w, r, http.StatusNotFound, "not_found", "job not found")
			return
		}
		s.writeErr(w, r, err)
		return
	}
	jsonResponse(w, http.StatusOK, j)
}
