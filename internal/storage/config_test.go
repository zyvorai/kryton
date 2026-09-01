package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "storage.json")
	s, err := NewStore(path, "longhorn")
	if err != nil {
		t.Fatal(err)
	}
	if s.Get().StorageClass != "longhorn" {
		t.Fatalf("initial=%q", s.Get().StorageClass)
	}
	if err := s.Save(Config{StorageClass: "rook-ceph-block"}); err != nil {
		t.Fatal(err)
	}
	s2, err := NewStore(path, "")
	if err != nil {
		t.Fatal(err)
	}
	if s2.Get().StorageClass != "rook-ceph-block" {
		t.Fatalf("reloaded=%q", s2.Get().StorageClass)
	}
	b, err := os.ReadFile(path)
	if err != nil || len(b) == 0 {
		t.Fatalf("file missing: %v", err)
	}
}

func TestClassifyBackend(t *testing.T) {
	cases := map[string]string{
		"rook-ceph.rbd.csi.ceph.com": "rook-ceph",
		"driver.longhorn.io":         "longhorn",
		"rancher.io/local-path":      "local-path",
		"pd.csi.storage.gke.io":      "other",
	}
	for in, want := range cases {
		if got := classifyBackend(in); got != want {
			t.Fatalf("%s: got %s want %s", in, got, want)
		}
	}
}
