package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/zyvorai/kryton/internal/atlas"
)

// Runtime is operator-tunable without restarting krytond.
type Runtime struct {
	DefaultProject  string       `json:"defaultProject,omitempty"`
	ImageNamespace  string       `json:"imageNamespace,omitempty"`
	EventWebhookURL string       `json:"eventWebhookUrl,omitempty"`
	Atlas           atlas.Config `json:"atlas,omitempty"`
}

// Store persists runtime settings under ~/.kryton/settings.json.
type Store struct {
	mu   sync.RWMutex
	path string
	cfg  Runtime
}

func NewStore(path string, initial Runtime) (*Store, error) {
	s := &Store{path: strings.TrimSpace(path), cfg: trimRuntime(initial)}
	if s.path == "" {
		return s, nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			if !emptyRuntime(s.cfg) {
				_ = s.Save(s.cfg)
			}
			return s, nil
		}
		return nil, err
	}
	var loaded Runtime
	if err := json.Unmarshal(b, &loaded); err != nil {
		return nil, err
	}
	s.cfg = mergeRuntime(s.cfg, trimRuntime(loaded))
	return s, nil
}

func (s *Store) Get() Runtime {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

func (s *Store) Save(cfg Runtime) error {
	cfg = trimRuntime(cfg)
	s.mu.Lock()
	s.cfg = cfg
	path := s.path
	s.mu.Unlock()
	if path == "" {
		return nil
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func trimRuntime(r Runtime) Runtime {
	a := r.Atlas
	a.BaseURL = strings.TrimRight(strings.TrimSpace(a.BaseURL), "/")
	a.Token = strings.TrimSpace(a.Token)
	a.Product = strings.TrimSpace(a.Product)
	if a.Product == "" && (a.Enabled || a.BaseURL != "") {
		a.Product = atlas.ProductID
	}
	return Runtime{
		DefaultProject:  strings.TrimSpace(r.DefaultProject),
		ImageNamespace:  strings.TrimSpace(r.ImageNamespace),
		EventWebhookURL: strings.TrimSpace(r.EventWebhookURL),
		Atlas:           a,
	}
}

func mergeRuntime(base, loaded Runtime) Runtime {
	out := base
	if loaded.DefaultProject != "" {
		out.DefaultProject = loaded.DefaultProject
	}
	if loaded.ImageNamespace != "" {
		out.ImageNamespace = loaded.ImageNamespace
	}
	if loaded.EventWebhookURL != "" {
		out.EventWebhookURL = loaded.EventWebhookURL
	}
	// Atlas: loaded file wins when present (including enabled=false with empty URL).
	if loaded.Atlas.BaseURL != "" || loaded.Atlas.Enabled || loaded.Atlas.Token != "" {
		out.Atlas = loaded.Atlas
	}
	return out
}

func emptyRuntime(r Runtime) bool {
	return r.DefaultProject == "" && r.ImageNamespace == "" && r.EventWebhookURL == "" &&
		!r.Atlas.Enabled && r.Atlas.BaseURL == "" && r.Atlas.Token == ""
}
