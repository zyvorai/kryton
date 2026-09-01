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

// Package auth implements krytond's authentication and role model:
// disabled, apikey (hashed keys loaded from a JSON file), and proxy
// (trusting X-Kryton-User/-Role/-Projects headers from a reverse proxy).
// It resolves each request to a Principal scoped to a Role (viewer,
// operator, admin) and a set of allowed projects.
package auth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
)

// Role is a Kryton access level; Can and rank treat these as a strict
// hierarchy Viewer < Operator < Admin, so holding Admin satisfies any
// lower requirement too.
type Role string

const (
	Viewer   Role = "viewer"
	Operator Role = "operator"
	Admin    Role = "admin"
)

// Principal is the caller identity resolved from a request by
// Authenticator.Middleware, retrievable in a handler via FromContext.
type Principal struct {
	Name     string   `json:"name"`
	Role     Role     `json:"role"`
	Projects []string `json:"projects"` // "*" grants every project
}

// Config selects and configures an auth Mode ("disabled", "apikey", or "proxy") for New.
type Config struct {
	Mode        string
	APIKeysFile string
	TrustProxy  bool
	// ProxySecretFile holds the shared secret the reverse proxy must
	// present in X-Kryton-Proxy-Secret; required when Mode is "proxy".
	ProxySecretFile string
}

type keyRecord struct {
	Name     string   `json:"name"`
	SHA256   string   `json:"sha256,omitempty"`
	Role     Role     `json:"role"`
	Projects []string `json:"projects"`
	hash     [32]byte
}

type keyFile struct {
	Keys []keyRecord `json:"keys"`
}

// Authenticator implements one auth Mode for the lifetime of a krytond
// process; build it once with New and install it via Middleware. It is
// safe for concurrent use — all state is read-only after New returns.
type Authenticator struct {
	mode            string
	trustProxy      bool
	keys            []keyRecord
	proxySecretHash [32]byte
	hasProxySecret  bool
}

type contextKey struct{}

// New builds an Authenticator for cfg.Mode. For "apikey" it loads and
// validates cfg.APIKeysFile (every key needs a sha256 digest — see
// HashToken); for "proxy" it loads cfg.ProxySecretFile. Either read
// failure, or an apikey file with zero valid keys, is returned as an error.
func New(cfg Config) (*Authenticator, error) {
	a := &Authenticator{mode: cfg.Mode, trustProxy: cfg.TrustProxy}
	if cfg.Mode == "proxy" {
		b, err := os.ReadFile(cfg.ProxySecretFile)
		if err != nil {
			return nil, fmt.Errorf("read proxy secret file: %w", err)
		}
		secret := strings.TrimSpace(string(b))
		if secret == "" {
			return nil, errors.New("proxy secret file is empty")
		}
		a.proxySecretHash = sha256.Sum256([]byte(secret))
		a.hasProxySecret = true
		return a, nil
	}
	if cfg.Mode != "apikey" {
		return a, nil
	}
	b, err := os.ReadFile(cfg.APIKeysFile)
	if err != nil {
		return nil, fmt.Errorf("read api keys file: %w", err)
	}
	var f keyFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("parse api keys file: %w", err)
	}
	if len(f.Keys) == 0 {
		return nil, errors.New("api keys file contains no keys")
	}
	for i := range f.Keys {
		k := &f.Keys[i]
		if k.Name == "" || !validRole(k.Role) || len(k.Projects) == 0 {
			return nil, fmt.Errorf("invalid api key entry %d", i)
		}
		if k.SHA256 == "" {
			return nil, fmt.Errorf("api key %q requires a sha256 digest", k.Name)
		}
		raw, err := hex.DecodeString(k.SHA256)
		if err != nil || len(raw) != sha256.Size {
			return nil, fmt.Errorf("api key %q has invalid sha256", k.Name)
		}
		copy(k.hash[:], raw)
	}
	a.keys = f.Keys
	return a, nil
}

// HashToken returns the hex-encoded SHA-256 digest of token — the value
// that belongs in an API keys file's "sha256" field. Kryton never stores
// or logs the raw token, only this digest.
func HashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// Middleware authenticates every request before it reaches next,
// rejecting with 401 on failure and otherwise storing the resolved
// Principal in the request context for FromContext to retrieve.
func (a *Authenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, err := a.authenticate(r)
		if err != nil {
			w.Header().Set("WWW-Authenticate", `Bearer realm="kryton"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), contextKey{}, p)))
	})
}

func (a *Authenticator) authenticate(r *http.Request) (Principal, error) {
	switch a.mode {
	case "disabled":
		return Principal{Name: "local-demo", Role: Admin, Projects: []string{"*"}}, nil
	case "proxy":
		if !a.trustProxy || !a.hasProxySecret {
			return Principal{}, errors.New("proxy is not trusted")
		}
		presented := sha256.Sum256([]byte(strings.TrimSpace(r.Header.Get("X-Kryton-Proxy-Secret"))))
		if subtle.ConstantTimeCompare(presented[:], a.proxySecretHash[:]) != 1 {
			return Principal{}, errors.New("invalid proxy secret")
		}
		name := strings.TrimSpace(r.Header.Get("X-Kryton-User"))
		role := Role(strings.ToLower(strings.TrimSpace(r.Header.Get("X-Kryton-Role"))))
		projects := splitCSV(r.Header.Get("X-Kryton-Projects"))
		if name == "" || !validRole(role) || len(projects) == 0 {
			return Principal{}, errors.New("missing trusted proxy identity headers")
		}
		return Principal{Name: name, Role: role, Projects: projects}, nil
	case "apikey":
		h := r.Header.Get("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			return Principal{}, errors.New("missing bearer token")
		}
		tokenHash := sha256.Sum256([]byte(strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))))
		for _, k := range a.keys {
			if subtle.ConstantTimeCompare(tokenHash[:], k.hash[:]) == 1 {
				return Principal{Name: k.Name, Role: k.Role, Projects: append([]string(nil), k.Projects...)}, nil
			}
		}
		return Principal{}, errors.New("invalid bearer token")
	default:
		return Principal{}, errors.New("unsupported auth mode")
	}
}

// FromContext retrieves the Principal Middleware attached to ctx,
// or the zero Principal (no role, no projects) if none was set.
func FromContext(ctx context.Context) Principal {
	if p, ok := ctx.Value(contextKey{}).(Principal); ok {
		return p
	}
	return Principal{}
}

// Can reports whether p holds at least the required Role and is scoped
// to project (directly, or via a "*" wildcard project).
func Can(p Principal, project string, required Role) bool {
	if rank(p.Role) < rank(required) {
		return false
	}
	for _, allowed := range p.Projects {
		if allowed == "*" || allowed == project {
			return true
		}
	}
	return false
}

// FilterProjects returns the subset of configured that p can at least
// view, preserving order — the list GET /api/v1/projects returns.
func FilterProjects(p Principal, configured []string) []string {
	var out []string
	for _, project := range configured {
		if Can(p, project, Viewer) {
			out = append(out, project)
		}
	}
	return out
}

func validRole(r Role) bool { return r == Viewer || r == Operator || r == Admin }
func rank(r Role) int {
	switch r {
	case Admin:
		return 3
	case Operator:
		return 2
	case Viewer:
		return 1
	default:
		return 0
	}
}

func splitCSV(v string) []string {
	var out []string
	for _, x := range strings.Split(v, ",") {
		x = strings.TrimSpace(x)
		if x != "" {
			out = append(out, x)
		}
	}
	return out
}
