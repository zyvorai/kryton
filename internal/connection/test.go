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

// Package connection runs a battery of connectivity probes — Kubernetes
// API reachability, storage class availability, and provider health —
// and aggregates them into a Result for the Settings UI's "Test
// connection" action and related diagnostics.
package connection

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/zyvorai/kryton/internal/kubeapi"
	"github.com/zyvorai/kryton/internal/provider"
	"github.com/zyvorai/kryton/internal/storage"
)

// Probe is one connection or dependency check.
type Probe struct {
	Name      string `json:"name"`
	Status    string `json:"status"` // pass | warn | fail
	Message   string `json:"message"`
	Hint      string `json:"hint,omitempty"`
	LatencyMs int64  `json:"latencyMs,omitempty"`
}

// Result aggregates probes for the Settings "Test connection" action.
type Result struct {
	Healthy  bool    `json:"healthy"`
	Probes   []Probe `json:"probes"`
	TestedAt string  `json:"testedAt"`
}

// Input supplies the live handles Test needs to reach each dependency;
// KubeClient nil skips the Kubernetes/KubeVirt/storage probes entirely.
type Input struct {
	Provider        provider.Provider
	KubeClient      *kubeapi.Client
	StorageClass    string
	ScriptsOK       bool
	SnapshotsScript string
}

// Test runs every applicable probe against in and aggregates them into a
// Result; it never returns an error, reporting failed dependencies as
// failed Probes instead. Individual probes are internally time-bounded,
// so Test always returns even if a dependency hangs.
func Test(ctx context.Context, in Input) Result {
	start := time.Now()
	var probes []Probe
	probes = append(probes, probeProvider(ctx, in.Provider))
	if in.KubeClient != nil {
		probes = append(probes, probeKubernetes(ctx, in.KubeClient))
		probes = append(probes, probeKubeVirt(ctx, in.KubeClient))
		probes = append(probes, probeStorage(ctx, in.KubeClient, in.StorageClass))
	} else if in.Provider != nil && in.Provider.Name() == "kubevirt" {
		probes = append(probes, Probe{Name: "kubernetes", Status: "fail", Message: "Kubernetes client is not configured", Hint: "Set KRYTON_KUBECONFIG or KRYTON_KUBERNETES_ENDPOINT"})
	}
	probes = append(probes, probeScripts(in.ScriptsOK, in.SnapshotsScript))
	probes = append(probes, probeKubectl(ctx))

	healthy := true
	for _, p := range probes {
		if p.Status == "fail" {
			healthy = false
			break
		}
	}
	return Result{Healthy: healthy, Probes: probes, TestedAt: start.UTC().Format(time.RFC3339)}
}

func probeProvider(ctx context.Context, p provider.Provider) Probe {
	if p == nil {
		return Probe{Name: "provider", Status: "fail", Message: "Provider not configured"}
	}
	t0 := time.Now()
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := p.Health(cctx); err != nil {
		return Probe{Name: "provider", Status: "fail", Message: err.Error(), Hint: "Fix provider connectivity", LatencyMs: time.Since(t0).Milliseconds()}
	}
	return Probe{Name: "provider", Status: "pass", Message: fmt.Sprintf("%s provider healthy", p.Name()), LatencyMs: time.Since(t0).Milliseconds()}
}

func probeKubernetes(ctx context.Context, kc *kubeapi.Client) Probe {
	t0 := time.Now()
	cctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	var ver struct {
		Major string `json:"major"`
		Minor string `json:"minor"`
		Git   string `json:"gitVersion"`
	}
	if err := kc.JSON(cctx, http.MethodGet, "/version", "", nil, &ver); err != nil {
		return Probe{Name: "kubernetes", Status: "fail", Message: "API unreachable: " + err.Error(), Hint: "Check KRYTON_KUBECONFIG and cluster connectivity", LatencyMs: time.Since(t0).Milliseconds()}
	}
	var nodes struct {
		Items []any `json:"items"`
	}
	if err := kc.JSON(cctx, http.MethodGet, "/api/v1/nodes", "", nil, &nodes); err != nil {
		return Probe{Name: "kubernetes", Status: "warn", Message: fmt.Sprintf("Connected (v%s) but nodes list failed", ver.Git), LatencyMs: time.Since(t0).Milliseconds()}
	}
	return Probe{Name: "kubernetes", Status: "pass", Message: fmt.Sprintf("API v%s · %d node(s)", ver.Git, len(nodes.Items)), LatencyMs: time.Since(t0).Milliseconds()}
}

func probeKubeVirt(ctx context.Context, kc *kubeapi.Client) Probe {
	t0 := time.Now()
	cctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	var list struct {
		Items []map[string]any `json:"items"`
	}
	if err := kc.JSON(cctx, http.MethodGet, "/apis/kubevirt.io/v1/kubevirts", "", nil, &list); err != nil {
		return Probe{Name: "kubevirt", Status: "fail", Message: "KubeVirt CRD/API not reachable", Hint: "Install KubeVirt on the cluster", LatencyMs: time.Since(t0).Milliseconds()}
	}
	if len(list.Items) == 0 {
		return Probe{Name: "kubevirt", Status: "fail", Message: "No KubeVirt custom resource found", Hint: "Install KubeVirt operator", LatencyMs: time.Since(t0).Milliseconds()}
	}
	meta := nested(list.Items[0], "metadata")
	name := str(meta, "name")
	ns := str(meta, "namespace")
	return Probe{Name: "kubevirt", Status: "pass", Message: fmt.Sprintf("KubeVirt %s/%s ready", ns, name), LatencyMs: time.Since(t0).Milliseconds()}
}

func probeStorage(ctx context.Context, kc *kubeapi.Client, storageClass string) Probe {
	t0 := time.Now()
	cctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	inv, err := storage.LoadInventory(cctx, kc, storage.Config{StorageClass: storageClass}, "kubevirt")
	if err != nil {
		return Probe{Name: "storage", Status: "fail", Message: err.Error(), LatencyMs: time.Since(t0).Milliseconds()}
	}
	sc := strings.TrimSpace(storageClass)
	if sc == "" {
		sc = inv.Config.StorageClass
	}
	if sc == "" {
		for _, c := range inv.StorageClasses {
			if c.Default {
				sc = c.Name
				break
			}
		}
	}
	if len(inv.StorageClasses) == 0 {
		return Probe{Name: "storage", Status: "fail", Message: "No StorageClasses on cluster", Hint: "Install Longhorn or Rook Ceph from Settings", LatencyMs: time.Since(t0).Milliseconds()}
	}
	if sc != "" {
		for _, c := range inv.StorageClasses {
			if c.Name == sc {
				if !c.SnapshotCapable {
					return Probe{Name: "storage", Status: "warn", Message: fmt.Sprintf("%s cannot CSI-snapshot", sc), Hint: "Pick rook-ceph-block or longhorn", LatencyMs: time.Since(t0).Milliseconds()}
				}
				return Probe{Name: "storage", Status: "pass", Message: fmt.Sprintf("%s · snapshot %s", sc, c.SnapshotClass), LatencyMs: time.Since(t0).Milliseconds()}
			}
		}
		return Probe{Name: "storage", Status: "warn", Message: fmt.Sprintf("Configured class %s not found", sc), Hint: "Refresh storage inventory or reinstall backend", LatencyMs: time.Since(t0).Milliseconds()}
	}
	snap := 0
	for _, c := range inv.StorageClasses {
		if c.SnapshotCapable {
			snap++
		}
	}
	if snap == 0 {
		return Probe{Name: "storage", Status: "warn", Message: "No snapshot-capable StorageClass", Hint: "Install Rook or Longhorn", LatencyMs: time.Since(t0).Milliseconds()}
	}
	return Probe{Name: "storage", Status: "pass", Message: fmt.Sprintf("%d StorageClass(es), %d snapshot-capable", len(inv.StorageClasses), snap), LatencyMs: time.Since(t0).Milliseconds()}
}

func probeScripts(ok bool, path string) Probe {
	if !ok {
		return Probe{Name: "install-scripts", Status: "warn", Message: "Storage install scripts not found on this host", Hint: "Set KRYTON_PROJECT_ROOT to the repo with scripts/"}
	}
	return Probe{Name: "install-scripts", Status: "pass", Message: "Storage setup scripts available"}
}

func probeKubectl(ctx context.Context) Probe {
	t0 := time.Now()
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, "kubectl", "cluster-info")
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return Probe{Name: "kubectl", Status: "warn", Message: "kubectl cluster-info failed", Hint: msg, LatencyMs: time.Since(t0).Milliseconds()}
	}
	line := strings.Split(strings.TrimSpace(string(out)), "\n")[0]
	return Probe{Name: "kubectl", Status: "pass", Message: line, LatencyMs: time.Since(t0).Milliseconds()}
}

func nested(m map[string]any, key string) map[string]any {
	v, _ := m[key].(map[string]any)
	if v == nil {
		return map[string]any{}
	}
	return v
}

func str(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}
