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
