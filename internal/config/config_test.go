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

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func validConfig() Config {
	return Config{
		Provider:       "demo",
		Projects:       []string{"default"},
		DefaultProject: "default",
		ImageNamespace: "kryton-images",
		AuthMode:       "disabled",
	}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(c *Config)
		wantErr string
	}{
		{
			name:   "valid demo config",
			mutate: func(c *Config) {},
		},
		{
			name:    "unsupported provider",
			mutate:  func(c *Config) { c.Provider = "vmware" },
			wantErr: "unsupported KRYTON_PROVIDER",
		},
		{
			name:    "no projects",
			mutate:  func(c *Config) { c.Projects = nil },
			wantErr: "at least one KRYTON_PROJECTS entry",
		},
		{
			name: "invalid project name",
			mutate: func(c *Config) {
				c.Projects = []string{"Not_Valid"}
				c.DefaultProject = "Not_Valid"
			},
			wantErr: "invalid project",
		},
		{
			name: "default project not in list",
			mutate: func(c *Config) {
				c.Projects = []string{"default"}
				c.DefaultProject = "other"
			},
			wantErr: "KRYTON_DEFAULT_PROJECT must be present",
		},
		{
			name: "kubevirt invalid image namespace",
			mutate: func(c *Config) {
				c.Provider = "kubevirt"
				c.AllowInsecure = true
				c.ImageNamespace = "Not_Valid"
			},
			wantErr: "invalid KRYTON_IMAGE_NAMESPACE",
		},
		{
			name: "kubevirt namespace prefix collision",
			mutate: func(c *Config) {
				c.Provider = "kubevirt"
				c.AllowInsecure = true
				c.NamespacePrefix = "UPPER-"
			},
			wantErr: "namespace mapping for project",
		},
		{
			name: "disabled auth with production provider requires allow insecure",
			mutate: func(c *Config) {
				c.Provider = "kubevirt"
				c.AuthMode = "disabled"
				c.AllowInsecure = false
			},
			wantErr: "KRYTON_ALLOW_INSECURE=true",
		},
		{
			name: "disabled auth with production provider and allow insecure passes",
			mutate: func(c *Config) {
				c.Provider = "kubevirt"
				c.AuthMode = "disabled"
				c.AllowInsecure = true
			},
		},
		{
			name: "disabled auth with demo provider never requires allow insecure",
			mutate: func(c *Config) {
				c.Provider = "demo"
				c.AuthMode = "disabled"
				c.AllowInsecure = false
			},
		},
		{
			name: "apikey without keys file",
			mutate: func(c *Config) {
				c.AuthMode = "apikey"
				c.APIKeysFile = ""
			},
			wantErr: "KRYTON_API_KEYS_FILE is required",
		},
		{
			name: "apikey with keys file passes",
			mutate: func(c *Config) {
				c.AuthMode = "apikey"
				c.APIKeysFile = "/etc/kryton/keys.json"
			},
		},
		{
			name: "proxy without trust proxy",
			mutate: func(c *Config) {
				c.AuthMode = "proxy"
				c.TrustProxy = false
			},
			wantErr: "KRYTON_TRUST_PROXY=true",
		},
		{
			name: "proxy without secret file",
			mutate: func(c *Config) {
				c.AuthMode = "proxy"
				c.TrustProxy = true
				c.ProxySecretFile = ""
			},
			wantErr: "KRYTON_PROXY_SECRET_FILE",
		},
		{
			name: "proxy fully configured passes",
			mutate: func(c *Config) {
				c.AuthMode = "proxy"
				c.TrustProxy = true
				c.ProxySecretFile = "/etc/kryton/proxy-secret"
			},
		},
		{
			name:    "unsupported auth mode",
			mutate:  func(c *Config) { c.AuthMode = "kerberos" },
			wantErr: "unsupported KRYTON_AUTH_MODE",
		},
		{
			name: "lab auto auth requires allow insecure",
			mutate: func(c *Config) {
				c.AuthMode = "apikey"
				c.APIKeysFile = "/etc/kryton/keys.json"
				c.LabAutoAuth = true
				c.AllowInsecure = false
			},
			wantErr: "KRYTON_LAB_AUTO_AUTH requires KRYTON_ALLOW_INSECURE=true",
		},
		{
			name: "lab auto auth requires apikey mode",
			mutate: func(c *Config) {
				c.AuthMode = "disabled"
				c.AllowInsecure = true
				c.LabAutoAuth = true
			},
			wantErr: "KRYTON_LAB_AUTO_AUTH requires KRYTON_AUTH_MODE=apikey",
		},
		{
			name: "lab auto auth fully configured passes",
			mutate: func(c *Config) {
				c.AuthMode = "apikey"
				c.APIKeysFile = "/etc/kryton/keys.json"
				c.AllowInsecure = true
				c.LabAutoAuth = true
			},
		},
		{
			name:    "tls cert without key",
			mutate:  func(c *Config) { c.TLS.CertFile = "/etc/kryton/tls.crt" },
			wantErr: "both KRYTON_TLS_CERT_FILE and KRYTON_TLS_KEY_FILE must be set together",
		},
		{
			name:    "tls key without cert",
			mutate:  func(c *Config) { c.TLS.KeyFile = "/etc/kryton/tls.key" },
			wantErr: "both KRYTON_TLS_CERT_FILE and KRYTON_TLS_KEY_FILE must be set together",
		},
		{
			name: "tls cert and key together passes",
			mutate: func(c *Config) {
				c.TLS.CertFile = "/etc/kryton/tls.crt"
				c.TLS.KeyFile = "/etc/kryton/tls.key"
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := validConfig()
			tc.mutate(&c)
			err := c.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %q", tc.wantErr, err.Error())
			}
		})
	}
}

func TestResolveKubernetesInCluster(t *testing.T) {
	c := &Config{}
	t.Setenv("KUBERNETES_SERVICE_HOST", "10.0.0.1")
	if err := c.resolveKubernetes(); err != nil {
		t.Fatalf("expected no error when KUBERNETES_SERVICE_HOST is set, got %v", err)
	}
	if c.Kubernetes.Endpoint != "" {
		t.Fatalf("expected resolveKubernetes to leave Endpoint alone for in-cluster discovery, got %q", c.Kubernetes.Endpoint)
	}
}

func TestResolveKubernetesEndpointAlreadySet(t *testing.T) {
	c := &Config{Kubernetes: Kubernetes{Endpoint: "https://k8s.example:6443"}}
	if err := c.resolveKubernetes(); err != nil {
		t.Fatalf("expected no error when Endpoint already set, got %v", err)
	}
	if c.Kubernetes.Endpoint != "https://k8s.example:6443" {
		t.Fatalf("expected Endpoint to be left untouched, got %q", c.Kubernetes.Endpoint)
	}
}

func TestResolveKubernetesFromKubeconfig(t *testing.T) {
	dir := t.TempDir()
	kubeconfig := filepath.Join(dir, "kubeconfig")
	const kubeconfigYAML = `
apiVersion: v1
kind: Config
clusters:
- name: test
  cluster:
    server: https://k8s.example:6443
    insecure-skip-tls-verify: true
contexts:
- name: test
  context:
    cluster: test
    user: test
current-context: test
users:
- name: test
  user:
    token: fake-token
`
	if err := os.WriteFile(kubeconfig, []byte(kubeconfigYAML), 0o600); err != nil {
		t.Fatalf("write kubeconfig fixture: %v", err)
	}

	c := &Config{}
	t.Setenv("KRYTON_KUBECONFIG", kubeconfig)
	if err := c.resolveKubernetes(); err != nil {
		t.Fatalf("resolveKubernetes: %v", err)
	}
	if c.Kubernetes.Endpoint != "https://k8s.example:6443" {
		t.Fatalf("expected endpoint from kubeconfig, got %q", c.Kubernetes.Endpoint)
	}
	if c.Kubernetes.BearerToken != "fake-token" {
		t.Fatalf("expected bearer token from kubeconfig, got %q", c.Kubernetes.BearerToken)
	}
	if !c.Kubernetes.InsecureSkipVerify {
		t.Fatal("expected InsecureSkipVerify to propagate from kubeconfig")
	}
}

func TestResolveKubernetesMissingKubeconfig(t *testing.T) {
	c := &Config{}
	t.Setenv("KRYTON_KUBECONFIG", filepath.Join(t.TempDir(), "does-not-exist"))
	if err := c.resolveKubernetes(); err == nil {
		t.Fatal("expected error for missing kubeconfig file")
	}
}

func TestLoadDefaultsToDemoDisabled(t *testing.T) {
	for _, k := range []string{
		"KRYTON_PROVIDER", "KRYTON_PROJECTS", "KRYTON_DEFAULT_PROJECT", "KRYTON_AUTH_MODE",
		"KRYTON_ALLOW_INSECURE", "KRYTON_API_KEYS_FILE", "KRYTON_TLS_CERT_FILE", "KRYTON_TLS_KEY_FILE",
	} {
		t.Setenv(k, "")
	}
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Provider != "demo" {
		t.Fatalf("expected default provider demo, got %q", cfg.Provider)
	}
	if cfg.AuthMode != "disabled" {
		t.Fatalf("expected default auth mode disabled, got %q", cfg.AuthMode)
	}
	if len(cfg.Projects) != 1 || cfg.Projects[0] != "default" {
		t.Fatalf("expected default project list [default], got %v", cfg.Projects)
	}
}

func TestLoadRejectsInvalidCombination(t *testing.T) {
	t.Setenv("KRYTON_PROVIDER", "kubevirt")
	t.Setenv("KRYTON_AUTH_MODE", "disabled")
	t.Setenv("KRYTON_ALLOW_INSECURE", "false")
	if _, err := Load(); err == nil {
		t.Fatal("expected Load to fail for kubevirt+disabled auth without allow-insecure")
	}
}
