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

package kubevirt

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/zyvorai/kryton/internal/kubeapi"
	"github.com/zyvorai/kryton/internal/model"
)

func TestCreateAndLifecycleUsesKubernetesREST(t *testing.T) {
	var mu sync.Mutex
	var created map[string]any
	var patches []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "GET" && r.URL.Path == "/version":
			io.WriteString(w, `{"gitVersion":"v1.36.0"}`)
		case r.Method == "GET" && r.URL.Path == "/apis/kubevirt.io/v1":
			io.WriteString(w, `{"kind":"APIResourceList"}`)
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/api/v1/namespaces/"):
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		case r.Method == "POST" && r.URL.Path == "/api/v1/namespaces":
			w.WriteHeader(http.StatusCreated)
			io.WriteString(w, `{"metadata":{"name":"finance"}}`)
		case r.Method == "GET" && strings.Contains(r.URL.Path, "/roles/"):
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		case r.Method == "GET" && strings.Contains(r.URL.Path, "/rolebindings/"):
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		case r.Method == "POST" && strings.Contains(r.URL.Path, "/roles"):
			w.WriteHeader(http.StatusCreated)
			io.WriteString(w, `{"metadata":{"name":"kryton-datavolume-cloner"}}`)
		case r.Method == "POST" && strings.Contains(r.URL.Path, "/rolebindings"):
			w.WriteHeader(http.StatusCreated)
			io.WriteString(w, `{"metadata":{"name":"kryton-allow-clone-from-finance"}}`)
		case r.Method == "POST" && r.URL.Path == "/apis/kubevirt.io/v1/namespaces/finance/virtualmachines":
			_ = json.NewDecoder(r.Body).Decode(&created)
			created["status"] = map[string]any{"printableStatus": "Starting"}
			meta := created["metadata"].(map[string]any)
			meta["creationTimestamp"] = "2026-09-01T00:00:00Z"
			_ = json.NewEncoder(w).Encode(created)
		case r.Method == "GET" && r.URL.Path == "/apis/kubevirt.io/v1/namespaces/finance/virtualmachines":
			if created == nil {
				io.WriteString(w, `{"items":[]}`)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{created}})
		case r.Method == "GET" && strings.Contains(r.URL.Path, "/virtualmachineinstances/"):
			io.WriteString(w, `{"status":{"interfaces":[{"ipAddress":"10.0.0.42"}]}}`)
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/virtualmachineinstances"):
			io.WriteString(w, `{"items":[]}`)
		case r.Method == "PATCH" && strings.Contains(r.URL.Path, "/virtualmachines/"):
			var patch map[string]any
			_ = json.NewDecoder(r.Body).Decode(&patch)
			mu.Lock()
			patches = append(patches, patch)
			mu.Unlock()
			created["spec"].(map[string]any)["runStrategy"] = patch["spec"].(map[string]any)["runStrategy"]
			created["status"] = map[string]any{"printableStatus": "Stopped"}
			_ = json.NewEncoder(w).Encode(created)
		case r.Method == "POST" && strings.Contains(r.URL.Path, "virtualmachinesnapshots"):
			var x map[string]any
			_ = json.NewDecoder(r.Body).Decode(&x)
			x["metadata"].(map[string]any)["creationTimestamp"] = "2026-09-01T00:01:00Z"
			_ = json.NewEncoder(w).Encode(x)
		default:
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		}
	}))
	defer server.Close()
	client, err := kubeapi.New(kubeapi.Config{Endpoint: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	p := New(Config{Client: client, ImageNamespace: "kryton-images"})
	spec := model.MachineSpec{Name: "finance-win01", Image: "windows-server-2025", Compute: model.ComputeSpec{CPU: 4, MemoryMiB: 8192}, Disk: model.DiskSpec{SizeGiB: 80}}
	m, err := p.Create(context.Background(), "finance", spec)
	if err != nil {
		t.Fatal(err)
	}
	if m.State != model.StateStarting || m.ProviderRef.Namespace != "finance" || m.ID == "" {
		t.Fatalf("unexpected machine: %+v", m)
	}
	if _, err = p.Stop(context.Background(), "finance", m.ID); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(patches) != 1 || patches[0]["spec"].(map[string]any)["runStrategy"] != "Halted" {
		t.Fatalf("unexpected patches: %#v", patches)
	}
	if _, err = p.Snapshot(context.Background(), "finance", m.ID, "before-patch"); err != nil {
		t.Fatal(err)
	}
}

func TestSnapshotListRestoreDelete(t *testing.T) {
	var created, snapshot, restore map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "GET" && r.URL.Path == "/version":
			io.WriteString(w, `{"gitVersion":"v1.36.0"}`)
		case r.Method == "GET" && r.URL.Path == "/apis/kubevirt.io/v1":
			io.WriteString(w, `{"kind":"APIResourceList"}`)
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/api/v1/namespaces/"):
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		case r.Method == "POST" && r.URL.Path == "/api/v1/namespaces":
			w.WriteHeader(http.StatusCreated)
			io.WriteString(w, `{"metadata":{"name":"finance"}}`)
		case r.Method == "GET" && strings.Contains(r.URL.Path, "/roles/"):
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		case r.Method == "GET" && strings.Contains(r.URL.Path, "/rolebindings/"):
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		case r.Method == "POST" && strings.Contains(r.URL.Path, "/roles"):
			w.WriteHeader(http.StatusCreated)
			io.WriteString(w, `{"metadata":{"name":"kryton-datavolume-cloner"}}`)
		case r.Method == "POST" && strings.Contains(r.URL.Path, "/rolebindings"):
			w.WriteHeader(http.StatusCreated)
			io.WriteString(w, `{"metadata":{"name":"kryton-allow-clone-from-finance"}}`)
		case r.Method == "POST" && r.URL.Path == "/apis/kubevirt.io/v1/namespaces/finance/virtualmachines":
			_ = json.NewDecoder(r.Body).Decode(&created)
			created["status"] = map[string]any{"printableStatus": "Stopped"}
			meta := created["metadata"].(map[string]any)
			meta["creationTimestamp"] = "2026-09-01T00:00:00Z"
			_ = json.NewEncoder(w).Encode(created)
		case r.Method == "GET" && r.URL.Path == "/apis/kubevirt.io/v1/namespaces/finance/virtualmachines":
			_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{created}})
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/virtualmachineinstances"):
			io.WriteString(w, `{"items":[]}`)
		case r.Method == "POST" && strings.Contains(r.URL.Path, "virtualmachinesnapshots") && !strings.Contains(r.URL.Path, "virtualmachinerestores"):
			_ = json.NewDecoder(r.Body).Decode(&snapshot)
			snapshot["metadata"].(map[string]any)["namespace"] = "finance"
			snapshot["metadata"].(map[string]any)["creationTimestamp"] = "2026-09-01T00:01:00Z"
			snapshot["status"] = map[string]any{"readyToUse": true, "phase": "Succeeded"}
			_ = json.NewEncoder(w).Encode(snapshot)
		case r.Method == "GET" && strings.Contains(r.URL.Path, "virtualmachinesnapshots"):
			_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{snapshot}})
		case r.Method == "POST" && strings.Contains(r.URL.Path, "virtualmachinerestores"):
			_ = json.NewDecoder(r.Body).Decode(&restore)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(restore)
		case r.Method == "DELETE" && strings.Contains(r.URL.Path, "virtualmachinesnapshots/"):
			w.WriteHeader(http.StatusOK)
			io.WriteString(w, `{"kind":"Status","status":"Success"}`)
		default:
			http.Error(w, `{"message":"not found `+r.Method+" "+r.URL.Path+`"}`, http.StatusNotFound)
		}
	}))
	defer server.Close()
	client, err := kubeapi.New(kubeapi.Config{Endpoint: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	p := New(Config{Client: client, ImageNamespace: "kryton-images"})
	spec := model.MachineSpec{Name: "finance-win01", Image: "windows-server-2025", Compute: model.ComputeSpec{CPU: 4, MemoryMiB: 8192}, Disk: model.DiskSpec{SizeGiB: 80}}
	m, err := p.Create(context.Background(), "finance", spec)
	if err != nil {
		t.Fatal(err)
	}
	snap, err := p.Snapshot(context.Background(), "finance", m.ID, "nightly")
	if err != nil {
		t.Fatal(err)
	}
	listed, err := p.ListSnapshots(context.Background(), "finance", m.ID)
	if err != nil || len(listed) != 1 || listed[0].State != "ready" {
		t.Fatalf("list=%+v err=%v", listed, err)
	}
	restored, err := p.RestoreSnapshot(context.Background(), "finance", m.ID, snap.ID)
	if err != nil || restored.State != "restoring" {
		t.Fatalf("restore=%+v err=%v", restored, err)
	}
	if restore == nil || restore["kind"] != "VirtualMachineRestore" {
		t.Fatalf("restore object=%#v", restore)
	}
	if err := p.DeleteSnapshot(context.Background(), "finance", m.ID, snap.ID); err != nil {
		t.Fatal(err)
	}
}
