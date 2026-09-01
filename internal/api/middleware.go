package api

import (
	"context"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/zyvorai/kryton/internal/id"
)

type requestIDKey struct{}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) { w.status = code; w.ResponseWriter.WriteHeader(code) }
func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}
func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *Server) requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rid := r.Header.Get("X-Request-ID")
		if rid == "" {
			rid = id.New()
		}
		w.Header().Set("X-Request-ID", rid)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey{}, rid)))
	})
}
func requestID(ctx context.Context) string { v, _ := ctx.Value(requestIDKey{}).(string); return v }
func (s *Server) security(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		next.ServeHTTP(w, r)
	})
}

// cors enables browser clients from other Zyvor products (Axiom, Haven, marketing) to call Kryton.
// Configure with KRYTON_CORS_ORIGINS (comma-separated; use * for lab/dev).
func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if allow := corsAllowOrigin(r, s.corsOrigins); allow != "" {
			w.Header().Set("Access-Control-Allow-Origin", allow)
			w.Header().Set("Vary", "Origin")
			if allow != "*" {
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Accept, X-Request-ID, X-Kryton-User, X-Kryton-Role, X-Kryton-Projects, X-Kryton-Proxy-Secret")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID")
			w.Header().Set("Access-Control-Max-Age", "600")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func ParseCORSOrigins(v string) []string {
	var out []string
	for _, p := range strings.Split(v, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
func (s *Server) accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(sw, r)
		status := sw.status
		if status == 0 {
			status = 200
		}
		s.metrics.Request(status >= 400)
		level := slog.LevelInfo
		if status >= 500 {
			level = slog.LevelError
		}
		s.log.Log(r.Context(), level, "http request", "method", r.Method, "path", r.URL.Path, "status", status, "duration_ms", time.Since(start).Milliseconds(), "request_id", requestID(r.Context()))
	})
}
func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if x := recover(); x != nil {
				s.log.Error("panic recovered", "panic", x, "stack", string(debug.Stack()), "request_id", requestID(r.Context()))
				s.writeAPIError(w, r, http.StatusInternalServerError, "INTERNAL", "internal server error", "Unexpected panic — check krytond logs with the request id.")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
