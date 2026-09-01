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
