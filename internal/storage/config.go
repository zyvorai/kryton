package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/zyvorai/kryton/internal/kubeapi"
)

// Config is the Kryton-selected default for new VM disks.
type Config struct {
	StorageClass string `json:"storageClass"`
}

// Class describes a cluster StorageClass for the Settings UI.
type Class struct {
	Name             string `json:"name"`
	Provisioner      string `json:"provisioner"`
	Default          bool   `json:"default,omitempty"`
	AllowExpansion   bool   `json:"allowVolumeExpansion,omitempty"`
	BindingMode      string `json:"volumeBindingMode,omitempty"`
	Backend          string `json:"backend"` // rook-ceph | longhorn | local-path | other
	SnapshotCapable  bool   `json:"snapshotCapable"`
	SnapshotClass    string `json:"snapshotClass,omitempty"`
	Recommended      bool   `json:"recommended,omitempty"`
}

// SnapshotClass is a CSI VolumeSnapshotClass.
type SnapshotClass struct {
	Name    string `json:"name"`
	Driver  string `json:"driver"`
	Default bool   `json:"default,omitempty"`
}

// Inventory is what the Settings UX needs to configure existing Rook/Longhorn.
type Inventory struct {
	StorageClasses    []Class         `json:"storageClasses"`
	SnapshotClasses   []SnapshotClass `json:"snapshotClasses"`
	Config            Config          `json:"config"`
	Provider          string          `json:"provider"`
	Setup             *SetupState     `json:"setup,omitempty"`
	BlockDevices      []BlockDevice   `json:"blockDevices,omitempty"`
	ScriptsAvailable  bool            `json:"scriptsAvailable"`
	BackendsInstalled map[string]bool `json:"backendsInstalled,omitempty"`
	Atlas             *AtlasHint      `json:"atlas,omitempty"`
}

// AtlasHint surfaces Atlas-discovered StorageClasses when integration is enabled.
type AtlasHint struct {
	Enabled        bool     `json:"enabled"`
	BaseURL        string   `json:"baseUrl,omitempty"`
	Healthy        bool     `json:"healthy"`
	Product        string   `json:"product,omitempty"`
	StorageClasses []string `json:"storageClasses,omitempty"`
}

// Store persists the operator-chosen StorageClass across restarts.
type Store struct {
	mu   sync.RWMutex
	path string
	cfg  Config
}

func NewStore(path string, initial string) (*Store, error) {
	s := &Store{path: strings.TrimSpace(path), cfg: Config{StorageClass: strings.TrimSpace(initial)}}
	if s.path == "" {
		return s, nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			if initial != "" {
				_ = s.Save(Config{StorageClass: initial})
			}
			return s, nil
		}
		return nil, err
	}
	var loaded Config
	if err := json.Unmarshal(b, &loaded); err != nil {
		return nil, err
	}
	if strings.TrimSpace(loaded.StorageClass) != "" {
		s.cfg = Config{StorageClass: strings.TrimSpace(loaded.StorageClass)}
	}
	return s, nil
}

func (s *Store) Get() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

func (s *Store) Save(cfg Config) error {
	cfg.StorageClass = strings.TrimSpace(cfg.StorageClass)
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

// LoadInventory lists StorageClasses and VolumeSnapshotClasses from the cluster.
func LoadInventory(ctx context.Context, kc *kubeapi.Client, current Config, provider string) (Inventory, error) {
	out := Inventory{Config: current, Provider: provider, StorageClasses: []Class{}, SnapshotClasses: []SnapshotClass{}}
	if kc == nil {
		return out, nil
	}
	var scList struct {
		Items []map[string]any `json:"items"`
	}
	if err := kc.JSON(ctx, http.MethodGet, "/apis/storage.k8s.io/v1/storageclasses", "", nil, &scList); err != nil {
		return out, fmt.Errorf("list storageclasses: %w", err)
	}
	var vscList struct {
		Items []map[string]any `json:"items"`
	}
	_ = kc.JSON(ctx, http.MethodGet, "/apis/snapshot.storage.k8s.io/v1/volumesnapshotclasses", "", nil, &vscList)

	driverToVSC := map[string]string{}
	for _, item := range vscList.Items {
		name := str(nested(item, "metadata"), "name")
		driver := str(item, "driver")
		def := anno(nested(item, "metadata"), "snapshot.storage.kubernetes.io/is-default-class") == "true"
		out.SnapshotClasses = append(out.SnapshotClasses, SnapshotClass{Name: name, Driver: driver, Default: def})
		if driver != "" {
			if _, ok := driverToVSC[driver]; !ok || def {
				driverToVSC[driver] = name
			}
		}
	}
	sort.Slice(out.SnapshotClasses, func(i, j int) bool { return out.SnapshotClasses[i].Name < out.SnapshotClasses[j].Name })

	for _, item := range scList.Items {
		name := str(nested(item, "metadata"), "name")
		prov := str(item, "provisioner")
		backend := classifyBackend(prov)
		vsc := driverToVSC[prov]
		snapOK := vsc != "" && backend != "local-path"
		out.StorageClasses = append(out.StorageClasses, Class{
			Name:            name,
			Provisioner:     prov,
			Default:         anno(nested(item, "metadata"), "storageclass.kubernetes.io/is-default-class") == "true",
			AllowExpansion:  boolVal(item["allowVolumeExpansion"]),
			BindingMode:     str(item, "volumeBindingMode"),
			Backend:         backend,
			SnapshotCapable: snapOK,
			SnapshotClass:   vsc,
			Recommended:     snapOK && (backend == "rook-ceph" || backend == "longhorn"),
		})
	}
	sort.Slice(out.StorageClasses, func(i, j int) bool {
		a, b := out.StorageClasses[i], out.StorageClasses[j]
		if a.Recommended != b.Recommended {
			return a.Recommended
		}
		if a.SnapshotCapable != b.SnapshotCapable {
			return a.SnapshotCapable
		}
		return a.Name < b.Name
	})
	return out, nil
}

func classifyBackend(provisioner string) string {
	p := strings.ToLower(provisioner)
	switch {
	case strings.Contains(p, "rook-ceph") || strings.Contains(p, "rbd.csi.ceph.com") || strings.Contains(p, "cephfs.csi.ceph.com"):
		return "rook-ceph"
	case strings.Contains(p, "longhorn"):
		return "longhorn"
	case strings.Contains(p, "local-path") || p == "kubernetes.io/no-provisioner":
		return "local-path"
	default:
		return "other"
	}
}

func nested(m map[string]any, key string) map[string]any {
	v, _ := m[key].(map[string]any)
	if v == nil {
		return map[string]any{}
	}
	return v
}
func str(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}
func anno(meta map[string]any, key string) string {
	a, _ := meta["annotations"].(map[string]any)
	return str(a, key)
}
func boolVal(v any) bool {
	b, _ := v.(bool)
	return b
}
