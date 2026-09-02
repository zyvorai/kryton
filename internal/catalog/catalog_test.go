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

package catalog

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	c, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	items := c.List()
	if len(items) == 0 {
		t.Fatal("expected default catalog to be non-empty")
	}
	seen := map[string]bool{}
	for _, img := range items {
		if seen[img.ID] {
			t.Fatalf("duplicate id %q in default catalog", img.ID)
		}
		seen[img.ID] = true
	}
	if _, ok := c.Get("windows-11-enterprise"); !ok {
		t.Fatal("expected windows-11-enterprise in default catalog")
	}
	if _, ok := c.Get("does-not-exist"); ok {
		t.Fatal("expected Get to report false for an unknown id")
	}
}

func TestListIsSortedByName(t *testing.T) {
	c, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	items := c.List()
	for i := 1; i < len(items); i++ {
		if items[i-1].Name > items[i].Name {
			t.Fatalf("List not sorted: %q before %q", items[i-1].Name, items[i].Name)
		}
	}
}

func writeCatalogFile(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "images.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write catalog file: %v", err)
	}
	return path
}

func TestLoadFromFileValidOverride(t *testing.T) {
	path := writeCatalogFile(t, `{"images":[{"id":"custom-1","name":"Custom One"}]}`)
	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	items := c.List()
	if len(items) != 1 || items[0].ID != "custom-1" {
		t.Fatalf("expected exactly the override image, got %+v", items)
	}
	if _, ok := c.Get("windows-11-enterprise"); ok {
		t.Fatal("expected file override to replace defaults entirely, not merge")
	}
}

func TestLoadFromFileDuplicateID(t *testing.T) {
	path := writeCatalogFile(t, `{"images":[{"id":"dup","name":"A"},{"id":"dup","name":"B"}]}`)
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for duplicate image id")
	}
}

func TestLoadFromFileEmptyID(t *testing.T) {
	path := writeCatalogFile(t, `{"images":[{"id":"","name":"A"}]}`)
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for empty image id")
	}
}

func TestLoadFromFileEmptyName(t *testing.T) {
	path := writeCatalogFile(t, `{"images":[{"id":"a","name":""}]}`)
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for empty image name")
	}
}

func TestLoadFromFileMalformedJSON(t *testing.T) {
	path := writeCatalogFile(t, `{not json`)
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestLoadFromFileEmptyImagesArray(t *testing.T) {
	path := writeCatalogFile(t, `{"images":[]}`)
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for an empty images array")
	}
}

func TestLoadFromFileMissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "does-not-exist.json")); err == nil {
		t.Fatal("expected error for a missing images file")
	}
}

func TestDockurVersion(t *testing.T) {
	c, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if v, ok := c.DockurVersion("windows-11-enterprise"); !ok || v != "11e" {
		t.Fatalf("got %q %v, want \"11e\" true", v, ok)
	}
	if _, ok := c.DockurVersion("does-not-exist"); ok {
		t.Fatal("expected false for unknown image id")
	}

	path := writeCatalogFile(t, `{"images":[{"id":"custom-1","name":"Custom One"}]}`)
	c2, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, ok := c2.DockurVersion("custom-1"); ok {
		t.Fatal("expected false when the image has no DockurVersion set")
	}
}
