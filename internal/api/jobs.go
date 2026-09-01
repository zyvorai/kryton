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
