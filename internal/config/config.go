package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/zyvorai/kryton/internal/model"
)

type Config struct {
	Addr              string
	Provider          string
	Projects          []string
	DefaultProject    string
	ImageNamespace    string
	NamespacePrefix   string
	ImagesFile        string
	AuthMode          string
	APIKeysFile       string
	ProxySecretFile   string
	TrustProxy        bool
	AllowInsecure     bool
	EventWebhookURL   string
	ReconcileInterval time.Duration
	ShutdownTimeout   time.Duration
	Kubernetes        Kubernetes
	TLS               TLS
	Dockur            Dockur
}

type Dockur struct {
	Runtime    string
	DataDir    string
	PublicHost string
	HTTPBase   int
	RDPBase    int
}

type Kubernetes struct {
	Endpoint           string
	BearerToken        string
	TokenFile          string
	CAFile             string
	InsecureSkipVerify bool
}

type TLS struct {
	CertFile     string
	KeyFile      string
	ClientCAFile string
}

func Load() (Config, error) {
	projects := splitCSV(getenv("KRYTON_PROJECTS", "default"))
	cfg := Config{
		Addr:              getenv("KRYTON_ADDR", ":8080"),
		Provider:          strings.ToLower(getenv("KRYTON_PROVIDER", "demo")),
		Projects:          projects,
		DefaultProject:    getenv("KRYTON_DEFAULT_PROJECT", first(projects, "default")),
		ImageNamespace:    getenv("KRYTON_IMAGE_NAMESPACE", "kryton-images"),
		NamespacePrefix:   getenv("KRYTON_NAMESPACE_PREFIX", ""),
		ImagesFile:        os.Getenv("KRYTON_IMAGES_FILE"),
		AuthMode:          strings.ToLower(getenv("KRYTON_AUTH_MODE", "disabled")),
		APIKeysFile:       os.Getenv("KRYTON_API_KEYS_FILE"),
		ProxySecretFile:   os.Getenv("KRYTON_PROXY_SECRET_FILE"),
		TrustProxy:        boolEnv("KRYTON_TRUST_PROXY", false),
		AllowInsecure:     boolEnv("KRYTON_ALLOW_INSECURE", false),
		EventWebhookURL:   os.Getenv("KRYTON_EVENT_WEBHOOK_URL"),
		ReconcileInterval: durationEnv("KRYTON_RECONCILE_INTERVAL", 30*time.Second),
		ShutdownTimeout:   durationEnv("KRYTON_SHUTDOWN_TIMEOUT", 15*time.Second),
		Kubernetes: Kubernetes{
			Endpoint:           os.Getenv("KRYTON_KUBERNETES_ENDPOINT"),
			BearerToken:        os.Getenv("KRYTON_KUBERNETES_BEARER_TOKEN"),
			TokenFile:          getenv("KRYTON_KUBERNETES_TOKEN_FILE", "/var/run/secrets/kubernetes.io/serviceaccount/token"),
			CAFile:             getenv("KRYTON_KUBERNETES_CA_FILE", "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"),
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
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

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
	if (c.TLS.CertFile == "") != (c.TLS.KeyFile == "") {
		return errors.New("both KRYTON_TLS_CERT_FILE and KRYTON_TLS_KEY_FILE must be set together")
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
