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
		{ID: "windows-11-enterprise", Name: "Windows 11 Enterprise", Version: "24H2", Family: "windows-desktop", Description: "Enterprise Windows desktop image for developer and test workspaces.", MinCPU: 4, MinMemoryMiB: 8192, DefaultDiskGB: 96, DockurVersion: "11e", Tags: []string{"desktop", "developer", "dockur"}},
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
