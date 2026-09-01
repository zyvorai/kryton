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
	"bytes"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"
)

func (s *Server) staticHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p == "." || p == "" {
			p = "index.html"
		}
		data, err := fs.ReadFile(s.web, p)
		if err != nil {
			p = "index.html"
			data, err = fs.ReadFile(s.web, p)
			if err != nil {
				http.NotFound(w, r)
				return
			}
		}
		if ct := mime.TypeByExtension(path.Ext(p)); ct != "" {
			w.Header().Set("Content-Type", ct)
		}
		if p == "index.html" {
			w.Header().Set("Cache-Control", "no-store")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=3600")
		}
		http.ServeContent(w, r, p, time.Time{}, bytes.NewReader(data))
	})
}
