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
