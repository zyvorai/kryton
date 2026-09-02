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

package doctor

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zyvorai/kryton/internal/catalog"
	"github.com/zyvorai/kryton/internal/demo"
	"github.com/zyvorai/kryton/internal/kubeapi"
)

func TestRunDemoHealthy(t *testing.T) {
	cat, err := catalog.Load("")
	if err != nil {
		t.Fatal(err)
	}
	report := Run(context.Background(), Input{
		Provider: demo.New(),
		Catalog:  cat,
		AuthMode: "disabled",
		Projects: []string{"default"},
	})
	if !report.Healthy {
		t.Fatalf("expected healthy demo report: %+v", report.Findings)
	}
	if report.Provider != "demo" {
		t.Fatalf("provider=%s", report.Provider)
	}
	if len(report.Findings) < 3 {
		t.Fatalf("expected findings, got %d", len(report.Findings))
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

func TestCheckKubeVirtImagesSkippedWithoutKubeClient(t *testing.T) {
	f := checkKubeVirtImages(context.Background(), nil, mustCatalog(t), "kryton-images")
	if f.Status != "warn" {
		t.Fatalf("expected warn without a kube client, got %+v", f)
	}
}

func mustCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()
	cat, err := catalog.Load("")
	if err != nil {
		t.Fatal(err)
	}
	return cat
}

func TestCheckKubeVirtImagesAllPresent(t *testing.T) {
	kc := newFakeKubeClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/datasources/") {
			_, _ = io.WriteString(w, `{"metadata":{"name":"x"}}`)
			return
		}
		http.NotFound(w, r)
	})
	f := checkKubeVirtImages(context.Background(), kc, mustCatalog(t), "kryton-images")
	if f.Status != "pass" {
		t.Fatalf("expected pass when every DataSource exists, got %+v", f)
	}
}

func TestCheckKubeVirtImagesReportsMissing(t *testing.T) {
	kc := newFakeKubeClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"reason":"NotFound","message":"not found"}`, http.StatusNotFound)
	})
	f := checkKubeVirtImages(context.Background(), kc, mustCatalog(t), "kryton-images")
	if f.Status != "fail" || !strings.Contains(f.Message, "Missing DataSources") {
		t.Fatalf("expected fail listing missing DataSources, got %+v", f)
	}
}

func TestCheckKubeVirtImagesEmptyCatalogFails(t *testing.T) {
	kc := newFakeKubeClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	// catalog.Load("") always seeds the built-in defaults, so a nil
	// catalog is the only way to exercise checkKubeVirtImages' empty-catalog branch.
	f := checkKubeVirtImages(context.Background(), kc, nil, "kryton-images")
	if f.Status != "fail" {
		t.Fatalf("expected fail for nil/empty catalog, got %+v", f)
	}
}

func TestCheckKubeVirtNamespacesSkippedWithoutKubeClient(t *testing.T) {
	f := checkKubeVirtNamespaces(context.Background(), nil, "", []string{"default"})
	if f.Status != "warn" {
		t.Fatalf("expected warn without a kube client, got %+v", f)
	}
}

func TestCheckKubeVirtNamespacesAllPresent(t *testing.T) {
	kc := newFakeKubeClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"metadata":{"name":"default"}}`)
	})
	f := checkKubeVirtNamespaces(context.Background(), kc, "", []string{"default"})
	if f.Status != "pass" {
		t.Fatalf("expected pass when namespace exists, got %+v", f)
	}
}

func TestCheckKubeVirtNamespacesReportsMissing(t *testing.T) {
	kc := newFakeKubeClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"reason":"NotFound","message":"not found"}`, http.StatusNotFound)
	})
	f := checkKubeVirtNamespaces(context.Background(), kc, "", []string{"default", "finance"})
	if f.Status != "fail" || !strings.Contains(f.Message, "default") || !strings.Contains(f.Message, "finance") {
		t.Fatalf("expected fail listing both missing namespaces, got %+v", f)
	}
}

func TestCheckKubeVirtSnapshotsSkippedWithoutKubeClient(t *testing.T) {
	f := checkKubeVirtSnapshots(context.Background(), nil)
	if f.Status != "warn" {
		t.Fatalf("expected warn without a kube client, got %+v", f)
	}
}

func TestCheckKubeVirtSnapshotsMissingSnapshotAPI(t *testing.T) {
	kc := newFakeKubeClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	f := checkKubeVirtSnapshots(context.Background(), kc)
	if f.Status != "fail" || !strings.Contains(f.Message, "snapshot API is not available") {
		t.Fatalf("expected fail for missing snapshot API, got %+v", f)
	}
}

func TestCheckKubeVirtSnapshotsNoVolumeSnapshotClass(t *testing.T) {
	kc := newFakeKubeClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/apis/snapshot.kubevirt.io/v1beta1":
			_, _ = io.WriteString(w, `{}`)
		case "/apis/snapshot.storage.k8s.io/v1":
			_, _ = io.WriteString(w, `{}`)
		case "/apis/snapshot.storage.k8s.io/v1/volumesnapshotclasses":
			_, _ = io.WriteString(w, `{"items":[]}`)
		default:
			http.NotFound(w, r)
		}
	})
	f := checkKubeVirtSnapshots(context.Background(), kc)
	if f.Status != "fail" || !strings.Contains(f.Message, "No VolumeSnapshotClass") {
		t.Fatalf("expected fail for no VolumeSnapshotClass, got %+v", f)
	}
}

func TestCheckKubeVirtSnapshotsFeatureGateDisabled(t *testing.T) {
	kc := newFakeKubeClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/apis/snapshot.kubevirt.io/v1beta1":
			_, _ = io.WriteString(w, `{}`)
		case "/apis/snapshot.storage.k8s.io/v1":
			_, _ = io.WriteString(w, `{}`)
		case "/apis/snapshot.storage.k8s.io/v1/volumesnapshotclasses":
			_, _ = io.WriteString(w, `{"items":[{"metadata":{"name":"csi-vsc"},"driver":"rook-ceph.rbd.csi.ceph.com"}]}`)
		case "/apis/kubevirt.io/v1/kubevirts":
			_, _ = io.WriteString(w, `{"items":[{"spec":{"configuration":{"developerConfiguration":{"featureGates":[]}}}}]}`)
		default:
			http.NotFound(w, r)
		}
	})
	f := checkKubeVirtSnapshots(context.Background(), kc)
	if f.Status != "fail" || !strings.Contains(f.Message, "feature gate is not enabled") {
		t.Fatalf("expected fail for disabled feature gate, got %+v", f)
	}
}

func TestCheckKubeVirtSnapshotsHealthy(t *testing.T) {
	kc := newFakeKubeClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/apis/snapshot.kubevirt.io/v1beta1":
			_, _ = io.WriteString(w, `{}`)
		case "/apis/snapshot.storage.k8s.io/v1":
			_, _ = io.WriteString(w, `{}`)
		case "/apis/snapshot.storage.k8s.io/v1/volumesnapshotclasses":
			_, _ = io.WriteString(w, `{"items":[{"metadata":{"name":"csi-vsc"},"driver":"rook-ceph.rbd.csi.ceph.com"}]}`)
		case "/apis/kubevirt.io/v1/kubevirts":
			_, _ = io.WriteString(w, `{"items":[]}`)
		default:
			http.NotFound(w, r)
		}
	})
	f := checkKubeVirtSnapshots(context.Background(), kc)
	if f.Status != "pass" {
		t.Fatalf("expected pass for a healthy snapshot setup, got %+v", f)
	}
}

func TestCheckKubeVirtStorageSkippedWithoutKubeClient(t *testing.T) {
	f := checkKubeVirtStorage(context.Background(), nil, "rook-ceph-block")
	if f.Status != "warn" {
		t.Fatalf("expected warn without a kube client, got %+v", f)
	}
}

func TestCheckKubeVirtStorageEmptyStorageClassWarns(t *testing.T) {
	kc := newFakeKubeClient(t, func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	f := checkKubeVirtStorage(context.Background(), kc, "")
	if f.Status != "warn" {
		t.Fatalf("expected warn for empty KRYTON_STORAGE_CLASS, got %+v", f)
	}
}

func TestCheckKubeVirtStorageNotFound(t *testing.T) {
	kc := newFakeKubeClient(t, func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	f := checkKubeVirtStorage(context.Background(), kc, "rook-ceph-block")
	if f.Status != "fail" || !strings.Contains(f.Message, "not found") {
		t.Fatalf("expected fail for missing StorageClass, got %+v", f)
	}
}

func TestCheckKubeVirtStorageLocalPathFails(t *testing.T) {
	kc := newFakeKubeClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(r.URL.Path, "/apis/storage.k8s.io/v1/storageclasses/") {
			_, _ = io.WriteString(w, `{"provisioner":"rancher.io/local-path"}`)
			return
		}
		http.NotFound(w, r)
	})
	f := checkKubeVirtStorage(context.Background(), kc, "local-path")
	if f.Status != "fail" || !strings.Contains(f.Message, "cannot CSI-snapshot") {
		t.Fatalf("expected fail for local-path provisioner, got %+v", f)
	}
}

func TestCheckKubeVirtStorageMatchingSnapshotClassPasses(t *testing.T) {
	kc := newFakeKubeClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasPrefix(r.URL.Path, "/apis/storage.k8s.io/v1/storageclasses/"):
			_, _ = io.WriteString(w, `{"provisioner":"rook-ceph.rbd.csi.ceph.com"}`)
		case r.URL.Path == "/apis/snapshot.storage.k8s.io/v1/volumesnapshotclasses":
			_, _ = io.WriteString(w, `{"items":[{"driver":"rook-ceph.rbd.csi.ceph.com"}]}`)
		default:
			http.NotFound(w, r)
		}
	})
	f := checkKubeVirtStorage(context.Background(), kc, "rook-ceph-block")
	if f.Status != "pass" {
		t.Fatalf("expected pass for a matching VolumeSnapshotClass, got %+v", f)
	}
}

func TestCheckKubeVirtStorageNoMatchingSnapshotClassFails(t *testing.T) {
	kc := newFakeKubeClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasPrefix(r.URL.Path, "/apis/storage.k8s.io/v1/storageclasses/"):
			_, _ = io.WriteString(w, `{"provisioner":"rook-ceph.rbd.csi.ceph.com"}`)
		case r.URL.Path == "/apis/snapshot.storage.k8s.io/v1/volumesnapshotclasses":
			_, _ = io.WriteString(w, `{"items":[]}`)
		default:
			http.NotFound(w, r)
		}
	})
	f := checkKubeVirtStorage(context.Background(), kc, "rook-ceph-block")
	if f.Status != "fail" || !strings.Contains(f.Message, "No VolumeSnapshotClass") {
		t.Fatalf("expected fail for no matching VolumeSnapshotClass, got %+v", f)
	}
}
