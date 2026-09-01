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

package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSettingsStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	s, err := NewStore(path, Runtime{DefaultProject: "default", ImageNamespace: "kryton-images"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Save(Runtime{DefaultProject: "lab", ImageNamespace: "images", EventWebhookURL: "https://example/hook"}); err != nil {
		t.Fatal(err)
	}
	s2, err := NewStore(path, Runtime{})
	if err != nil {
		t.Fatal(err)
	}
	got := s2.Get()
	if got.DefaultProject != "lab" || got.ImageNamespace != "images" || got.EventWebhookURL != "https://example/hook" {
		t.Fatalf("got %+v", got)
	}
	b, err := os.ReadFile(path)
	if err != nil || len(b) == 0 {
		t.Fatalf("file missing: %v", err)
	}
}
