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
	"strings"

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

// goldenPassport serves the raw guestkit Cutover Passport JSON recorded for
// a build (schema: guestkit's src/assurance/passport.rs) — full evidence
// (BitLocker, VirtIO drivers, activation, hotfixes, blockers, fix plan)
// behind the certified/validationScore summary already on GET
// /api/v1/golden/{id}.
func (s *Server) goldenPassport(w http.ResponseWriter, r *http.Request) {
	if s.golden == nil {
		s.writeAPIError(w, r, http.StatusNotFound, "not_found", "golden image builder not available on this instance")
		return
	}
	b, err := s.golden.Get(r.PathValue("id"))
	if err != nil {
		s.writeAPIError(w, r, http.StatusNotFound, "not_found", "golden build not found")
		return
	}
	path := strings.TrimSpace(b.PassportPath)
	if path == "" {
		s.writeAPIError(w, r, http.StatusNotFound, "not_found", "no guestkit passport recorded for this build (guestkit may not have been installed on the build host)")
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		s.writeAPIError(w, r, http.StatusNotFound, "not_found", "passport file missing on disk: "+path)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
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

func (s *Server) goldenBootstrap(w http.ResponseWriter, r *http.Request) {
	if s.golden == nil {
		s.writeAPIError(w, r, http.StatusServiceUnavailable, "unavailable", "golden image builder is not available on this instance")
		return
	}
	if _, ok := s.requireProject(w, r, auth.Operator); !ok {
		return
	}
	b, err := s.golden.Bootstrap(r.PathValue("id"))
	if err != nil {
		switch {
		case errors.Is(err, golden.ErrNotFound):
			s.writeAPIError(w, r, http.StatusNotFound, "not_found", "golden build not found")
		case errors.Is(err, golden.ErrNotReady):
			s.badRequest(w, r, "golden image must be captured (state=ready) before CDI bootstrap")
		case errors.Is(err, golden.ErrBootstrapRunning):
			s.writeAPIError(w, r, http.StatusConflict, "conflict", "CDI bootstrap is already running for this build")
		case errors.Is(err, golden.ErrBootstrapDisabled):
			s.writeAPIError(w, r, http.StatusServiceUnavailable, "unavailable", "CDI bootstrap script is not available on this instance")
		default:
			s.writeErr(w, r, err)
		}
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
