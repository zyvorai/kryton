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

// Package config loads krytond's runtime Config from KRYTON_* environment
// variables: provider selection, project list, auth mode, storage class,
// TLS/insecure flags, and file paths for images, API keys, and events.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/zyvorai/kryton/internal/kubeapi"
	"github.com/zyvorai/kryton/internal/model"
)

// Config is krytond's fully-resolved runtime configuration, produced by
// Load from KRYTON_* environment variables (see README.md's Configuration
// table for the env var each field maps to) and checked by Validate.
type Config struct {
	Addr               string
	Provider           string
	Projects           []string
	DefaultProject     string
	ImageNamespace     string
	NamespacePrefix    string
	StorageClass       string
	ImagesFile         string
	AuthMode           string
	APIKeysFile        string
	ProxySecretFile    string
	TrustProxy         bool
	AllowInsecure      bool
	LabAutoAuth        bool
	LabTokenFile       string
	EventWebhookURL    string
	EventWebhookSecret string
	EventsFile         string
	GoldenDir          string
	StorageConfigFile  string
	CORSOrigins        []string
	ReconcileInterval  time.Duration
	ShutdownTimeout    time.Duration
	// RateLimitRPS/RateLimitBurst configure the per-caller API token
	// bucket (internal/api's rateLimit middleware); RateLimitRPS <= 0
	// disables rate limiting entirely.
	RateLimitRPS   int
	RateLimitBurst int
	Kubernetes     Kubernetes
	TLS            TLS
	Dockur         Dockur
}

// Dockur configures the dockur provider: which container runtime to
// drive and the host-side port ranges to publish per machine.
type Dockur struct {
	Runtime    string
	DataDir    string
	PublicHost string
	HTTPBase   int
	RDPBase    int
}

// Kubernetes carries the credentials the kubevirt provider uses to reach
// the Kubernetes API. Load populates it explicitly from KRYTON_KUBERNETES_*
// vars, or falls back to in-cluster/kubeconfig discovery via resolveKubernetes.
type Kubernetes struct {
	Endpoint           string
	BearerToken        string
	TokenFile          string
	CAFile             string
	ClientCertFile     string
	ClientKeyFile      string
	InsecureSkipVerify bool
}

// TLS configures krytond's own listener certificate and, optionally,
// mutual-TLS client verification.
type TLS struct {
	CertFile     string
	KeyFile      string
	ClientCAFile string
}

// Load reads Config from the environment, fills in the kubevirt
// Kubernetes credentials when needed, and calls Validate before
// returning — cmd/krytond treats any returned error as fatal at startup.
func Load() (Config, error) {
	projects := splitCSV(getenv("KRYTON_PROJECTS", "default"))
	cfg := Config{
		Addr:               getenv("KRYTON_ADDR", ":8080"),
		Provider:           strings.ToLower(getenv("KRYTON_PROVIDER", "demo")),
		Projects:           projects,
		DefaultProject:     getenv("KRYTON_DEFAULT_PROJECT", first(projects, "default")),
		ImageNamespace:     getenv("KRYTON_IMAGE_NAMESPACE", "kryton-images"),
		NamespacePrefix:    getenv("KRYTON_NAMESPACE_PREFIX", ""),
		StorageClass:       getenv("KRYTON_STORAGE_CLASS", ""),
		ImagesFile:         os.Getenv("KRYTON_IMAGES_FILE"),
		AuthMode:           strings.ToLower(getenv("KRYTON_AUTH_MODE", "disabled")),
		APIKeysFile:        os.Getenv("KRYTON_API_KEYS_FILE"),
		ProxySecretFile:    os.Getenv("KRYTON_PROXY_SECRET_FILE"),
		TrustProxy:         boolEnv("KRYTON_TRUST_PROXY", false),
		AllowInsecure:      boolEnv("KRYTON_ALLOW_INSECURE", false),
		LabAutoAuth:        boolEnv("KRYTON_LAB_AUTO_AUTH", false),
		LabTokenFile:       os.Getenv("KRYTON_LAB_TOKEN_FILE"),
		EventWebhookURL:    os.Getenv("KRYTON_EVENT_WEBHOOK_URL"),
		EventWebhookSecret: os.Getenv("KRYTON_EVENT_WEBHOOK_SECRET"),
		EventsFile:         os.Getenv("KRYTON_EVENTS_FILE"),
		GoldenDir:          getenv("KRYTON_GOLDEN_DIR", ""),
		StorageConfigFile:  getenv("KRYTON_STORAGE_CONFIG_FILE", ""),
		CORSOrigins:        splitCSV(os.Getenv("KRYTON_CORS_ORIGINS")),
		ReconcileInterval:  durationEnv("KRYTON_RECONCILE_INTERVAL", 30*time.Second),
		ShutdownTimeout:    durationEnv("KRYTON_SHUTDOWN_TIMEOUT", 15*time.Second),
		RateLimitRPS:       intEnv("KRYTON_RATE_LIMIT_RPS", 0),
		RateLimitBurst:     intEnv("KRYTON_RATE_LIMIT_BURST", 0),
		Kubernetes: Kubernetes{
			Endpoint:           os.Getenv("KRYTON_KUBERNETES_ENDPOINT"),
			BearerToken:        os.Getenv("KRYTON_KUBERNETES_BEARER_TOKEN"),
			TokenFile:          getenv("KRYTON_KUBERNETES_TOKEN_FILE", "/var/run/secrets/kubernetes.io/serviceaccount/token"),
			CAFile:             getenv("KRYTON_KUBERNETES_CA_FILE", "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"),
			ClientCertFile:     os.Getenv("KRYTON_KUBERNETES_CLIENT_CERT_FILE"),
			ClientKeyFile:      os.Getenv("KRYTON_KUBERNETES_CLIENT_KEY_FILE"),
			InsecureSkipVerify: boolEnv("KRYTON_KUBERNETES_INSECURE_SKIP_VERIFY", false),
		},
		TLS: TLS{
			CertFile:     os.Getenv("KRYTON_TLS_CERT_FILE"),
			KeyFile:      os.Getenv("KRYTON_TLS_KEY_FILE"),
			ClientCAFile: os.Getenv("KRYTON_CLIENT_CA_FILE"),
		},
		Dockur: Dockur{
			Runtime:    getenv("KRYTON_DOCKUR_RUNTIME", "docker"),
			DataDir:    os.Getenv("KRYTON_DOCKUR_DATA_DIR"),
			PublicHost: getenv("KRYTON_DOCKUR_PUBLIC_HOST", "127.0.0.1"),
			HTTPBase:   intEnv("KRYTON_DOCKUR_HTTP_BASE", 18006),
			RDPBase:    intEnv("KRYTON_DOCKUR_RDP_BASE", 13389),
		},
	}
	if cfg.Provider == "kubevirt" {
		if err := cfg.resolveKubernetes(); err != nil {
			return Config{}, err
		}
	}
	if cfg.LabTokenFile == "" && cfg.APIKeysFile != "" {
		cfg.LabTokenFile = filepath.Join(filepath.Dir(cfg.APIKeysFile), "lab.token")
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Validate checks cross-field invariants Load cannot enforce
// per-variable alone: provider/auth-mode combinations, project list
// consistency, namespace validity, and the insecure-mode opt-in rules.
func (c Config) Validate() error {
	switch c.Provider {
	case "demo", "kubevirt", "dockur":
	default:
		return fmt.Errorf("unsupported KRYTON_PROVIDER %q (demo|kubevirt|dockur)", c.Provider)
	}
	if len(c.Projects) == 0 {
		return errors.New("at least one KRYTON_PROJECTS entry is required")
	}
	foundDefault := false
	for _, p := range c.Projects {
		if err := model.ValidateProject(p); err != nil {
			return fmt.Errorf("invalid project %q: %w", p, err)
		}
		if p == c.DefaultProject {
			foundDefault = true
		}
	}
	if !foundDefault {
		return errors.New("KRYTON_DEFAULT_PROJECT must be present in KRYTON_PROJECTS")
	}
	if c.Provider == "kubevirt" {
		if err := model.ValidateProject(c.ImageNamespace); err != nil {
			return fmt.Errorf("invalid KRYTON_IMAGE_NAMESPACE: %w", err)
		}
		for _, p := range c.Projects {
			if err := model.ValidateProject(c.NamespacePrefix + p); err != nil {
				return fmt.Errorf("namespace mapping for project %q is invalid: %w", p, err)
			}
		}
	}
	switch c.AuthMode {
	case "disabled":
		if c.Provider != "demo" && c.Provider != "dockur" && !c.AllowInsecure {
			return errors.New("authentication cannot be disabled with a production provider unless KRYTON_ALLOW_INSECURE=true")
		}
	case "apikey":
		if c.APIKeysFile == "" {
			return errors.New("KRYTON_API_KEYS_FILE is required for apikey authentication")
		}
	case "proxy":
		if !c.TrustProxy {
			return errors.New("proxy auth requires KRYTON_TRUST_PROXY=true")
		}
		if c.ProxySecretFile == "" {
			return errors.New("proxy auth requires KRYTON_PROXY_SECRET_FILE")
		}
	default:
		return fmt.Errorf("unsupported KRYTON_AUTH_MODE %q", c.AuthMode)
	}
	if c.LabAutoAuth {
		if !c.AllowInsecure {
			return errors.New("KRYTON_LAB_AUTO_AUTH requires KRYTON_ALLOW_INSECURE=true")
		}
		if c.AuthMode != "apikey" {
			return errors.New("KRYTON_LAB_AUTO_AUTH requires KRYTON_AUTH_MODE=apikey")
		}
	}
	if (c.TLS.CertFile == "") != (c.TLS.KeyFile == "") {
		return errors.New("both KRYTON_TLS_CERT_FILE and KRYTON_TLS_KEY_FILE must be set together")
	}
	return nil
}

func (c *Config) resolveKubernetes() error {
	if strings.TrimSpace(c.Kubernetes.Endpoint) != "" {
		return nil
	}
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		return nil
	}
	path := strings.TrimSpace(os.Getenv("KRYTON_KUBECONFIG"))
	if path == "" {
		path = strings.TrimSpace(os.Getenv("KUBECONFIG"))
	}
	kc, err := kubeapi.FromKubeconfig(path)
	if err != nil {
		return fmt.Errorf("kubernetes config: %w (set KRYTON_KUBERNETES_ENDPOINT or KRYTON_KUBECONFIG)", err)
	}
	if c.Kubernetes.BearerToken == "" {
		c.Kubernetes.BearerToken = kc.BearerToken
	}
	if c.Kubernetes.CAFile == "" || c.Kubernetes.CAFile == "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt" {
		if kc.CAFile != "" {
			c.Kubernetes.CAFile = kc.CAFile
		}
	}
	if c.Kubernetes.ClientCertFile == "" {
		c.Kubernetes.ClientCertFile = kc.ClientCertFile
	}
	if c.Kubernetes.ClientKeyFile == "" {
		c.Kubernetes.ClientKeyFile = kc.ClientKeyFile
	}
	c.Kubernetes.Endpoint = kc.Endpoint
	if kc.InsecureSkipVerify {
		c.Kubernetes.InsecureSkipVerify = true
	}
	return nil
}

func getenv(k, d string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return d
}

func splitCSV(v string) []string {
	var out []string
	seen := map[string]bool{}
	for _, s := range strings.Split(v, ",") {
		s = strings.TrimSpace(s)
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func first(v []string, d string) string {
	if len(v) > 0 {
		return v[0]
	}
	return d
}

func boolEnv(k string, d bool) bool {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return d
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return d
	}
	return b
}

func durationEnv(k string, d time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return d
	}
	x, err := time.ParseDuration(v)
	if err != nil {
		return d
	}
	return x
}

func intEnv(k string, d int) int {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return d
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return d
	}
	return n
}
