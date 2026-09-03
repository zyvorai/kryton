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

package images

import (
	"context"
	"testing"

	"github.com/zyvorai/kryton/internal/catalog"
	"github.com/zyvorai/kryton/internal/model"
)

func TestApplyPriorityGoldenOverCDI(t *testing.T) {
	inv := &Inventory{Provider: "kubevirt"}
	img := model.Image{ID: "windows-11-enterprise", DockurVersion: "11e"}
	stored := map[string]string{"windows-11-enterprise": "kryton-images"}
	golden := map[string]goldenArtifact{"windows-11-enterprise": {Path: "/out/windows-11e-golden.qcow2", Certified: true, Score: 92}}

	got := inv.apply(img, stored, golden)
	if !got.Ready || got.Availability != "stored" || got.StorageSource != "golden" {
		t.Fatalf("expected golden to win over CDI, got %+v", got)
	}
	if got.StoragePath != "/out/windows-11e-golden.qcow2" {
		t.Fatalf("expected StoragePath from golden artifact, got %q", got.StoragePath)
	}
	if !got.Certified || got.ValidationScore != 92 {
		t.Fatalf("expected Certified/ValidationScore propagated from golden artifact, got %+v", got)
	}
}

func TestApplyPriorityCDIOverDockur(t *testing.T) {
	inv := &Inventory{Provider: "dockur"}
	img := model.Image{ID: "windows-11-enterprise", DockurVersion: "11e"}
	stored := map[string]string{"windows-11-enterprise": "kryton-images"}

	got := inv.apply(img, stored, map[string]goldenArtifact{})
	if !got.Ready || got.Availability != "stored" || got.StorageSource != "cdi" {
		t.Fatalf("expected CDI to win over dockur on-demand, got %+v", got)
	}
	if got.StorageNamespace != "kryton-images" {
		t.Fatalf("expected StorageNamespace set, got %q", got.StorageNamespace)
	}
}

func TestApplyDockurOnDemand(t *testing.T) {
	inv := &Inventory{Provider: "dockur"}
	img := model.Image{ID: "windows-11-enterprise", DockurVersion: "11e"}

	got := inv.apply(img, map[string]string{}, map[string]goldenArtifact{})
	if !got.Ready || got.Availability != "on-demand" || got.StorageSource != "dockur" {
		t.Fatalf("expected dockur on-demand, got %+v", got)
	}
}

func TestApplyDockurProviderWithoutVersionNotReady(t *testing.T) {
	inv := &Inventory{Provider: "dockur"}
	img := model.Image{ID: "custom", DockurVersion: ""}

	got := inv.apply(img, map[string]string{}, map[string]goldenArtifact{})
	if got.Ready || got.Availability != "catalog" {
		t.Fatalf("expected catalog-only when dockur has no version mapping, got %+v", got)
	}
}

func TestApplyDemoAlwaysOnDemand(t *testing.T) {
	inv := &Inventory{Provider: "demo"}
	img := model.Image{ID: "anything"}

	got := inv.apply(img, map[string]string{}, map[string]goldenArtifact{})
	if !got.Ready || got.Availability != "on-demand" || got.StorageSource != "demo" {
		t.Fatalf("expected demo on-demand, got %+v", got)
	}
}

func TestApplyKubevirtNoStorageNotReady(t *testing.T) {
	inv := &Inventory{Provider: "kubevirt"}
	img := model.Image{ID: "windows-11-enterprise"}

	got := inv.apply(img, map[string]string{}, map[string]goldenArtifact{})
	if got.Ready || got.Availability != "catalog" {
		t.Fatalf("expected not ready when kubevirt has no matching DataSource/golden artifact, got %+v", got)
	}
}

func TestGoldenIDFromFilename(t *testing.T) {
	cases := map[string]string{
		"windows-11e-golden.qcow2":  "windows-11-enterprise",
		"windows-11-golden.qcow2":   "windows-11-pro",
		"windows-2025-golden.qcow2": "windows-server-2025",
		"windows-2022-golden.qcow2": "windows-server-2022",
		"windows-2019-golden.qcow2": "windows-server-2019",
		"windows-10-golden.qcow2":   "windows-10-pro",
		// "tiny11" contains the substring "11", so it matches the "11" case
		// before falling through to no-match — a real quirk of the
		// substring-based classification, not a test mistake.
		"windows-tiny11-golden.qcow2": "windows-11-pro",
		"not-a-windows-image.qcow2":   "",
	}
	for in, want := range cases {
		if got := goldenIDFromFilename(in); got != want {
			t.Errorf("goldenIDFromFilename(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStoredOnly(t *testing.T) {
	imgs := []model.Image{
		{ID: "a", Availability: "stored"},
		{ID: "b", Availability: "on-demand"},
		{ID: "c", Availability: "catalog"},
		{ID: "d", Availability: "stored"},
	}
	out := StoredOnly(imgs)
	if len(out) != 2 || out[0].ID != "a" || out[1].ID != "d" {
		t.Fatalf("expected only stored images a,d, got %+v", out)
	}
}

func TestStoredOnlyEmpty(t *testing.T) {
	if out := StoredOnly(nil); out != nil {
		t.Fatalf("expected nil for no images, got %+v", out)
	}
}

func TestEnrichWithoutLiveDependenciesFallsBackToCatalog(t *testing.T) {
	cat, err := catalog.Load("")
	if err != nil {
		t.Fatalf("catalog.Load: %v", err)
	}
	inv := &Inventory{Provider: "kubevirt"}
	out := inv.Enrich(context.Background(), cat)
	if len(out) != len(cat.List()) {
		t.Fatalf("expected Enrich to return every catalog image, got %d want %d", len(out), len(cat.List()))
	}
	for _, img := range out {
		if img.Ready {
			t.Fatalf("expected no image ready with no KubeClient/Golden configured, got %+v", img)
		}
	}
}

func TestEnrichNilCatalogReturnsNil(t *testing.T) {
	inv := &Inventory{}
	if out := inv.Enrich(context.Background(), nil); out != nil {
		t.Fatalf("expected nil for nil catalog, got %+v", out)
	}
}
