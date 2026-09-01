package kubeapi

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// FromKubeconfig loads API settings from the current context in a kubeconfig file.
// It shells out to kubectl (already required for cluster operations).
func FromKubeconfig(path string) (Config, error) {
	if strings.TrimSpace(path) == "" {
		path = defaultKubeconfigPath()
	}
	if path == "" {
		return Config{}, fmt.Errorf("kubeconfig not found (set KRYTON_KUBECONFIG or KUBECONFIG)")
	}
	if _, err := os.Stat(path); err != nil {
		return Config{}, fmt.Errorf("kubeconfig %s: %w", path, err)
	}

	cmd := exec.Command("kubectl", "config", "view", "--minify", "--raw", "-o", "json")
	cmd.Env = append(os.Environ(), "KUBECONFIG="+path)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return Config{}, fmt.Errorf("kubectl config view: %s", strings.TrimSpace(string(ee.Stderr)))
		}
		return Config{}, fmt.Errorf("kubectl config view: %w", err)
	}

	var raw struct {
		Clusters []struct {
			Cluster struct {
				Server                   string `json:"server"`
				CertificateAuthorityData string `json:"certificate-authority-data"`
				InsecureSkipTLSVerify    bool   `json:"insecure-skip-tls-verify"`
			} `json:"cluster"`
		} `json:"clusters"`
		Users []struct {
			User struct {
				Token                 string `json:"token"`
				ClientCertificateData string `json:"client-certificate-data"`
				ClientKeyData         string `json:"client-key-data"`
				ClientCertificate     string `json:"client-certificate"`
				ClientKey             string `json:"client-key"`
			} `json:"user"`
		} `json:"users"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return Config{}, fmt.Errorf("parse kubeconfig json: %w", err)
	}
	if len(raw.Clusters) == 0 || raw.Clusters[0].Cluster.Server == "" {
		return Config{}, fmt.Errorf("kubeconfig has no cluster server")
	}
	cluster := raw.Clusters[0].Cluster
	cfg := Config{
		Endpoint:           cluster.Server,
		InsecureSkipVerify: cluster.InsecureSkipTLSVerify,
	}
	if cluster.CertificateAuthorityData != "" {
		caFile, err := writeTempPEM("kryton-k8s-ca-", cluster.CertificateAuthorityData)
		if err != nil {
			return Config{}, err
		}
		cfg.CAFile = caFile
	}
	if len(raw.Users) == 0 {
		return cfg, nil
	}
	user := raw.Users[0].User
	if user.Token != "" {
		cfg.BearerToken = user.Token
		return cfg, nil
	}
	certData := firstNonEmpty(user.ClientCertificateData, readFileB64(user.ClientCertificate))
	keyData := firstNonEmpty(user.ClientKeyData, readFileB64(user.ClientKey))
	if certData == "" || keyData == "" {
		return cfg, fmt.Errorf("kubeconfig user has no token or client certificate")
	}
	certFile, err := writeTempPEM("kryton-k8s-cert-", certData)
	if err != nil {
		return Config{}, err
	}
	keyFile, err := writeTempPEM("kryton-k8s-key-", keyData)
	if err != nil {
		return Config{}, err
	}
	cfg.ClientCertFile = certFile
	cfg.ClientKeyFile = keyFile
	return cfg, nil
}

func defaultKubeconfigPath() string {
	for _, k := range []string{"KRYTON_KUBECONFIG", "KUBECONFIG"} {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	home, _ := os.UserHomeDir()
	if home != "" {
		p := filepath.Join(home, ".kube", "config")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	for _, p := range []string{"/etc/rancher/k3s/k3s.yaml"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func writeTempPEM(prefix, data string) (string, error) {
	pem, err := decodePEM(data)
	if err != nil {
		return "", err
	}
	f, err := os.CreateTemp("", prefix+"*.pem")
	if err != nil {
		return "", err
	}
	if _, err := f.Write(pem); err != nil {
		_ = f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	return f.Name(), nil
}

func decodePEM(v string) ([]byte, error) {
	if strings.Contains(v, "-----BEGIN") {
		return []byte(v), nil
	}
	b, err := base64.StdEncoding.DecodeString(v)
	if err != nil {
		return nil, fmt.Errorf("decode PEM data: %w", err)
	}
	return b, nil
}

func readFileB64(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}
