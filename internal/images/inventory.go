package images

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/zyvorai/kryton/internal/catalog"
	"github.com/zyvorai/kryton/internal/golden"
	"github.com/zyvorai/kryton/internal/kubeapi"
	"github.com/zyvorai/kryton/internal/model"
)

// Inventory enriches catalog entries with what is actually stored / deployable.
type Inventory struct {
	Provider     string
	ImageNS      string
	KubeClient   *kubeapi.Client
	Golden       *golden.Manager
	ProjectRoot  string
}

func (inv *Inventory) Enrich(ctx context.Context, cat *catalog.Catalog) []model.Image {
	if cat == nil {
		return nil
	}
	stored := inv.dataSources(ctx)
	goldenReady := inv.goldenArtifacts()
	items := cat.List()
	out := make([]model.Image, 0, len(items))
	for _, img := range items {
		img = inv.apply(img, stored, goldenReady)
		out = append(out, img)
	}
	return out
}

func (inv *Inventory) apply(img model.Image, stored map[string]string, goldenReady map[string]string) model.Image {
	img.Ready = false
	img.Availability = "catalog"
	img.StorageSource = ""
	img.StoragePath = ""

	if path, ok := goldenReady[img.ID]; ok {
		img.Ready = true
		img.Availability = "stored"
		img.StorageSource = "golden"
		img.StoragePath = path
		return img
	}
	if ns, ok := stored[img.ID]; ok {
		img.Ready = true
		img.Availability = "stored"
		img.StorageSource = "cdi"
		img.StorageNamespace = ns
		return img
	}
	if inv.Provider == "dockur" && img.DockurVersion != "" {
		img.Ready = true
		img.Availability = "on-demand"
		img.StorageSource = "dockur"
		return img
	}
	if inv.Provider == "demo" {
		img.Ready = true
		img.Availability = "on-demand"
		img.StorageSource = "demo"
		return img
	}
	return img
}

func (inv *Inventory) dataSources(ctx context.Context) map[string]string {
	out := map[string]string{}
	if inv.KubeClient == nil {
		return out
	}
	ns := inv.ImageNS
	if ns == "" {
		ns = "kryton-images"
	}
	path := "/apis/cdi.kubevirt.io/v1beta1/namespaces/" + ns + "/datasources"
	var list map[string]any
	if err := inv.KubeClient.JSON(ctx, "GET", path, "", nil, &list); err != nil {
		return out
	}
	items, _ := list["items"].([]any)
	for _, raw := range items {
		m, _ := raw.(map[string]any)
		meta, _ := m["metadata"].(map[string]any)
		name, _ := meta["name"].(string)
		if name != "" {
			out[name] = ns
		}
	}
	return out
}

func (inv *Inventory) goldenArtifacts() map[string]string {
	out := map[string]string{}
	if inv.Golden != nil {
		builds, err := inv.Golden.List()
		if err == nil {
			for _, b := range builds {
				if b.State == model.GoldenReady && b.OutputPath != "" {
					out[b.ImageID] = b.OutputPath
				}
			}
		}
	}
	root := inv.ProjectRoot
	if root == "" {
		return out
	}
	matches, _ := filepath.Glob(filepath.Join(root, "out", "windows-*-golden.qcow2"))
	for _, path := range matches {
		base := filepath.Base(path)
		id := goldenIDFromFilename(base)
		if id != "" {
			out[id] = path
		}
	}
	// Also scan ~/.kryton golden outputs referenced in status
	return out
}

func goldenIDFromFilename(name string) string {
	// windows-11e-golden.qcow2, windows-2025-golden.qcow2
	name = strings.TrimSuffix(name, ".qcow2")
	name = strings.TrimSuffix(name, "-golden")
	switch {
	case strings.Contains(name, "11e"):
		return "windows-11-enterprise"
	case strings.Contains(name, "11"):
		return "windows-11-pro"
	case strings.Contains(name, "2025"):
		return "windows-server-2025"
	case strings.Contains(name, "2022"):
		return "windows-server-2022"
	case strings.Contains(name, "2019"):
		return "windows-server-2019"
	case strings.Contains(name, "10"):
		return "windows-10-pro"
	default:
		return ""
	}
}

// StoredOnly returns images that are ready to deploy from local/cluster storage.
func StoredOnly(images []model.Image) []model.Image {
	var out []model.Image
	for _, img := range images {
		if img.Availability == "stored" {
			out = append(out, img)
		}
	}
	return out
}
