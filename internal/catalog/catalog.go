package catalog

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/zyvorai/kryton/internal/model"
)

type Catalog struct {
	items map[string]model.Image
}

type fileFormat struct {
	Images []model.Image `json:"images"`
}

func Load(path string) (*Catalog, error) {
	items := defaults()
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read images file: %w", err)
		}
		var f fileFormat
		if err := json.Unmarshal(b, &f); err != nil {
			return nil, fmt.Errorf("parse images file: %w", err)
		}
		if len(f.Images) == 0 {
			return nil, fmt.Errorf("images file contains no images")
		}
		items = f.Images
	}
	c := &Catalog{items: map[string]model.Image{}}
	for _, img := range items {
		if img.ID == "" || img.Name == "" {
			return nil, fmt.Errorf("image id and name are required")
		}
		if _, exists := c.items[img.ID]; exists {
			return nil, fmt.Errorf("duplicate image id %q", img.ID)
		}
		c.items[img.ID] = img
	}
	return c, nil
}

func (c *Catalog) List() []model.Image {
	out := make([]model.Image, 0, len(c.items))
	for _, img := range c.items {
		out = append(out, img)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (c *Catalog) Get(id string) (model.Image, bool) {
	img, ok := c.items[id]
	return img, ok
}

func defaults() []model.Image {
	return []model.Image{
		{ID: "windows-server-2025", Name: "Windows Server 2025", Version: "2025", Family: "windows-server", Description: "Enterprise Windows Server image reference for CDI DataSource / dockur provisioning.", MinCPU: 2, MinMemoryMiB: 4096, DefaultDiskGB: 80, DockurVersion: "2025", Tags: []string{"server", "enterprise", "dockur"}},
		{ID: "windows-server-2022", Name: "Windows Server 2022", Version: "2022", Family: "windows-server", Description: "Stable Windows Server image for application and infrastructure workloads.", MinCPU: 2, MinMemoryMiB: 4096, DefaultDiskGB: 80, DockurVersion: "2022", Tags: []string{"server", "lts", "dockur"}},
		{ID: "windows-server-2019", Name: "Windows Server 2019", Version: "2019", Family: "windows-server", Description: "Windows Server 2019 for legacy application labs.", MinCPU: 2, MinMemoryMiB: 4096, DefaultDiskGB: 80, DockurVersion: "2019", Tags: []string{"server", "legacy", "dockur"}},
		{ID: "windows-server-2016", Name: "Windows Server 2016", Version: "2016", Family: "windows-server", Description: "Windows Server 2016 for compatibility testing.", MinCPU: 2, MinMemoryMiB: 4096, DefaultDiskGB: 80, DockurVersion: "2016", Tags: []string{"server", "legacy", "dockur"}},
		{ID: "windows-11-enterprise", Name: "Windows 11 Enterprise", Version: "24H2", Family: "windows-desktop", Description: "Enterprise Windows desktop image for developer and test workspaces.", MinCPU: 4, MinMemoryMiB: 8192, DefaultDiskGB: 96, DockurVersion: "11e", Tags: []string{"desktop", "developer", "dockur"}},
		{ID: "windows-11-pro", Name: "Windows 11 Pro", Version: "24H2", Family: "windows-desktop", Description: "Windows 11 Pro for general desktop labs.", MinCPU: 4, MinMemoryMiB: 8192, DefaultDiskGB: 80, DockurVersion: "11", Tags: []string{"desktop", "dockur"}},
		{ID: "windows-11-ltsc", Name: "Windows 11 LTSC", Version: "LTSC", Family: "windows-desktop", Description: "Windows 11 LTSC — long-term servicing channel for locked-down labs.", MinCPU: 4, MinMemoryMiB: 8192, DefaultDiskGB: 64, DockurVersion: "11l", Tags: []string{"desktop", "ltsc", "dockur"}},
		{ID: "windows-10-enterprise", Name: "Windows 10 Enterprise", Version: "22H2", Family: "windows-desktop", Description: "Windows 10 Enterprise for legacy desktop workloads.", MinCPU: 2, MinMemoryMiB: 4096, DefaultDiskGB: 64, DockurVersion: "10e", Tags: []string{"desktop", "legacy", "dockur"}},
		{ID: "windows-10-pro", Name: "Windows 10 Pro", Version: "22H2", Family: "windows-desktop", Description: "Windows 10 Pro for general desktop labs.", MinCPU: 2, MinMemoryMiB: 4096, DefaultDiskGB: 64, DockurVersion: "10", Tags: []string{"desktop", "legacy", "dockur"}},
		{ID: "windows-10-ltsc", Name: "Windows 10 LTSC", Version: "LTSC", Family: "windows-desktop", Description: "Windows 10 LTSC for long-lived lab images.", MinCPU: 2, MinMemoryMiB: 4096, DefaultDiskGB: 64, DockurVersion: "10l", Tags: []string{"desktop", "ltsc", "dockur"}},
		{ID: "windows-tiny11", Name: "Tiny11", Version: "tiny11", Family: "windows-desktop", Description: "Compact Windows 11 derivative for lightweight dockur labs.", MinCPU: 2, MinMemoryMiB: 4096, DefaultDiskGB: 48, DockurVersion: "tiny11", Tags: []string{"desktop", "compact", "dockur"}},
		{ID: "windows-tiny11-core", Name: "Tiny11 Core", Version: "core11", Family: "windows-desktop", Description: "Minimal Tiny11 Core image for fast dockur boots.", MinCPU: 2, MinMemoryMiB: 2048, DefaultDiskGB: 32, DockurVersion: "core11", Tags: []string{"desktop", "compact", "dockur"}},
	}
}

// DockurVersion implements dockur.VersionResolver.
func (c *Catalog) DockurVersion(imageID string) (string, bool) {
	img, ok := c.items[imageID]
	if !ok || img.DockurVersion == "" {
		return "", false
	}
	return img.DockurVersion, true
}
