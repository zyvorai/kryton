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

package storage

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/zyvorai/kryton/internal/kubeapi"
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

func newFakeKubeClient(t *testing.T, handler http.HandlerFunc) *kubeapi.Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	kc, err := kubeapi.New(kubeapi.Config{Endpoint: srv.URL})
	if err != nil {
		t.Fatalf("kubeapi.New: %v", err)
	}
	return kc
}

func TestLoadInventoryNilKubeClientReturnsEmpty(t *testing.T) {
	inv, err := LoadInventory(context.Background(), nil, Config{StorageClass: "longhorn"}, "kubevirt")
	if err != nil {
		t.Fatalf("LoadInventory: %v", err)
	}
	if len(inv.StorageClasses) != 0 || len(inv.SnapshotClasses) != 0 {
		t.Fatalf("expected empty inventory for nil client, got %+v", inv)
	}
	if inv.Config.StorageClass != "longhorn" || inv.Provider != "kubevirt" {
		t.Fatalf("expected current config/provider echoed back, got %+v", inv)
	}
}

func TestLoadInventoryClassifiesAndRanks(t *testing.T) {
	kc := newFakeKubeClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/apis/storage.k8s.io/v1/storageclasses":
			_, _ = io.WriteString(w, `{"items":[
				{"metadata":{"name":"local-path"},"provisioner":"rancher.io/no-provisioner","allowVolumeExpansion":false},
				{"metadata":{"name":"rook-ceph-block","annotations":{"storageclass.kubernetes.io/is-default-class":"true"}},"provisioner":"rook-ceph.rbd.csi.ceph.com","allowVolumeExpansion":true,"volumeBindingMode":"Immediate"},
				{"metadata":{"name":"longhorn"},"provisioner":"driver.longhorn.io","allowVolumeExpansion":true}
			]}`)
		case "/apis/snapshot.storage.k8s.io/v1/volumesnapshotclasses":
			_, _ = io.WriteString(w, `{"items":[
				{"metadata":{"name":"rook-ceph-vsc"},"driver":"rook-ceph.rbd.csi.ceph.com"},
				{"metadata":{"name":"longhorn-vsc"},"driver":"driver.longhorn.io"}
			]}`)
		default:
			http.NotFound(w, r)
		}
	})

	inv, err := LoadInventory(context.Background(), kc, Config{}, "kubevirt")
	if err != nil {
		t.Fatalf("LoadInventory: %v", err)
	}
	if len(inv.StorageClasses) != 3 {
		t.Fatalf("expected 3 storage classes, got %+v", inv.StorageClasses)
	}
	// rook-ceph with a matching VolumeSnapshotClass ranks first, then
	// longhorn with a matching class, then local-path last regardless of
	// its provisioner string (classifyBackend's own bucket order).
	if inv.StorageClasses[0].Name != "rook-ceph-block" || !inv.StorageClasses[0].SnapshotCapable || !inv.StorageClasses[0].Default {
		t.Fatalf("expected rook-ceph-block ranked first and marked default+snapshot-capable, got %+v", inv.StorageClasses[0])
	}
	if inv.StorageClasses[0].SnapshotClass != "rook-ceph-vsc" {
		t.Fatalf("expected matching snapshot class name, got %q", inv.StorageClasses[0].SnapshotClass)
	}
	if inv.StorageClasses[1].Name != "longhorn" || !inv.StorageClasses[1].SnapshotCapable {
		t.Fatalf("expected longhorn ranked second and snapshot-capable, got %+v", inv.StorageClasses[1])
	}
	if inv.StorageClasses[2].Name != "local-path" || inv.StorageClasses[2].SnapshotCapable {
		t.Fatalf("expected local-path ranked last and not snapshot-capable, got %+v", inv.StorageClasses[2])
	}
	if len(inv.SnapshotClasses) != 2 {
		t.Fatalf("expected 2 snapshot classes, got %+v", inv.SnapshotClasses)
	}
}

func TestLoadInventoryStorageClassesErrorPropagates(t *testing.T) {
	kc := newFakeKubeClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"boom"}`, http.StatusInternalServerError)
	})
	_, err := LoadInventory(context.Background(), kc, Config{}, "kubevirt")
	if err == nil {
		t.Fatal("expected an error when listing storageclasses fails")
	}
}

func TestLoadInventoryToleratesMissingSnapshotAPI(t *testing.T) {
	kc := newFakeKubeClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/apis/storage.k8s.io/v1/storageclasses":
			_, _ = io.WriteString(w, `{"items":[{"metadata":{"name":"longhorn"},"provisioner":"driver.longhorn.io"}]}`)
		default:
			// Simulate a cluster without the snapshot.storage.k8s.io API installed.
			http.NotFound(w, r)
		}
	})
	inv, err := LoadInventory(context.Background(), kc, Config{}, "kubevirt")
	if err != nil {
		t.Fatalf("expected no error when VolumeSnapshotClass listing 404s, got %v", err)
	}
	if len(inv.StorageClasses) != 1 || inv.StorageClasses[0].SnapshotCapable {
		t.Fatalf("expected the storage class to still be listed but not snapshot-capable, got %+v", inv.StorageClasses)
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
