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

type Role string

const (
	Viewer   Role = "viewer"
	Operator Role = "operator"
	Admin    Role = "admin"
)

type Principal struct {
	Name     string   `json:"name"`
	Role     Role     `json:"role"`
	Projects []string `json:"projects"`
}

type Config struct {
	Mode            string
	APIKeysFile     string
	TrustProxy      bool
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

type Authenticator struct {
	mode            string
	trustProxy      bool
	keys            []keyRecord
	proxySecretHash [32]byte
	hasProxySecret  bool
}

type contextKey struct{}

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

func HashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

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

func FromContext(ctx context.Context) Principal {
	if p, ok := ctx.Value(contextKey{}).(Principal); ok {
		return p
	}
	return Principal{}
}

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
