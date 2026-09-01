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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/zyvorai/kryton/internal/catalog"
	"github.com/zyvorai/kryton/internal/kubeapi"
	"github.com/zyvorai/kryton/internal/kubevirt"
	"github.com/zyvorai/kryton/internal/model"
	"github.com/zyvorai/kryton/internal/provider"
)

type Input struct {
	Provider        provider.Provider
	Catalog         *catalog.Catalog
	AuthMode        string
	Projects        []string
	DockurDir       string
	Runtime         string // docker | podman
	KubeClient      *kubeapi.Client
	ImageNamespace  string
	NamespacePrefix string
	StorageClass    string
}

func Run(ctx context.Context, in Input) model.DoctorReport {
	report := model.DoctorReport{Provider: in.Provider.Name(), Findings: []model.DoctorFinding{}}
	add := func(f model.DoctorFinding) { report.Findings = append(report.Findings, f) }

	add(checkAuth(in.AuthMode, in.Provider.Name()))
	add(checkProjects(in.Projects))
	add(checkCatalog(in.Catalog))
	add(checkProviderHealth(ctx, in.Provider))

	switch in.Provider.Name() {
	case "dockur":
		add(checkBinary(firstNonEmpty(in.Runtime, "docker"), "Container runtime for dockur/windows lab VMs"))
		add(checkCompose(firstNonEmpty(in.Runtime, "docker")))
		add(checkKVM())
		add(checkDirWritable(firstNonEmpty(in.DockurDir, filepath.Join(os.TempDir(), "kryton-dockur"))))
	case "kubevirt":
		add(checkKubeVirtImages(ctx, in.KubeClient, in.Catalog, in.ImageNamespace))
		add(checkKubeVirtNamespaces(ctx, in.KubeClient, in.NamespacePrefix, in.Projects))
		add(checkKubeVirtSnapshots(ctx, in.KubeClient))
		add(checkKubeVirtStorage(ctx, in.KubeClient, in.StorageClass))
	case "demo":
		add(model.DoctorFinding{Check: "demo-provider", Status: "pass", Message: "In-memory demo provider is active (no real Windows guests)"})
	}

	report.Healthy = true
	for _, f := range report.Findings {
		if f.Status == "fail" {
			report.Healthy = false
			break
		}
	}
	return report
}

func checkAuth(mode, providerName string) model.DoctorFinding {
	switch mode {
	case "disabled":
		if providerName != "demo" {
			return model.DoctorFinding{Check: "auth", Status: "warn", Message: "Authentication is disabled on a non-demo provider", Hint: "Use KRYTON_AUTH_MODE=apikey (or proxy) for production"}
		}
		return model.DoctorFinding{Check: "auth", Status: "pass", Message: "Auth disabled (local demo mode)"}
	case "apikey", "proxy":
		return model.DoctorFinding{Check: "auth", Status: "pass", Message: fmt.Sprintf("Auth mode %s", mode)}
	default:
		return model.DoctorFinding{Check: "auth", Status: "fail", Message: fmt.Sprintf("Unknown auth mode %q", mode)}
	}
}

func checkProjects(projects []string) model.DoctorFinding {
	if len(projects) == 0 {
		return model.DoctorFinding{Check: "projects", Status: "fail", Message: "No projects configured", Hint: "Set KRYTON_PROJECTS"}
	}
	return model.DoctorFinding{Check: "projects", Status: "pass", Message: fmt.Sprintf("%d project(s): %s", len(projects), strings.Join(projects, ", "))}
}

func checkCatalog(cat *catalog.Catalog) model.DoctorFinding {
	if cat == nil || len(cat.List()) == 0 {
		return model.DoctorFinding{Check: "catalog", Status: "fail", Message: "Image catalog is empty"}
	}
	items := cat.List()
	dockur := 0
	for _, img := range items {
		if img.DockurVersion != "" {
			dockur++
		}
	}
	msg := fmt.Sprintf("%d image(s) loaded", len(items))
	if dockur > 0 {
		msg += fmt.Sprintf(" (%d with dockurVersion)", dockur)
	}
	return model.DoctorFinding{Check: "catalog", Status: "pass", Message: msg}
}

func checkProviderHealth(ctx context.Context, p provider.Provider) model.DoctorFinding {
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := p.Health(cctx); err != nil {
		return model.DoctorFinding{Check: "provider-health", Status: "fail", Message: err.Error(), Hint: "Fix provider connectivity before creating machines"}
	}
	return model.DoctorFinding{Check: "provider-health", Status: "pass", Message: fmt.Sprintf("Provider %s is healthy", p.Name())}
}

func checkBinary(name, purpose string) model.DoctorFinding {
	path, err := exec.LookPath(name)
	if err != nil {
		return model.DoctorFinding{Check: "runtime", Status: "fail", Message: fmt.Sprintf("%s not found in PATH", name), Hint: purpose}
	}
	return model.DoctorFinding{Check: "runtime", Status: "pass", Message: fmt.Sprintf("%s found at %s", name, path)}
}

func checkCompose(runtime string) model.DoctorFinding {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, runtime, "compose", "version")
	if out, err := cmd.CombinedOutput(); err != nil {
		return model.DoctorFinding{Check: "compose", Status: "fail", Message: fmt.Sprintf("%s compose unavailable: %v", runtime, err), Hint: "Install Docker Compose v2 or podman-compose"}
	} else {
		line := strings.TrimSpace(string(out))
		if i := strings.IndexByte(line, '\n'); i > 0 {
			line = line[:i]
		}
		return model.DoctorFinding{Check: "compose", Status: "pass", Message: line}
	}
}

func checkKVM() model.DoctorFinding {
	if runtime.GOOS != "linux" {
		return model.DoctorFinding{Check: "kvm", Status: "warn", Message: fmt.Sprintf("Host OS is %s; dockur needs Linux KVM (or nested virt)", runtime.GOOS), Hint: "Run dockur provider on a Linux host with /dev/kvm"}
	}
	if _, err := os.Stat("/dev/kvm"); err != nil {
		return model.DoctorFinding{Check: "kvm", Status: "fail", Message: "/dev/kvm is missing", Hint: "Enable VT-x/AMD-V in firmware and load kvm_intel/kvm_amd"}
	}
	return model.DoctorFinding{Check: "kvm", Status: "pass", Message: "/dev/kvm is present"}
}

func checkDirWritable(dir string) model.DoctorFinding {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return model.DoctorFinding{Check: "data-dir", Status: "fail", Message: err.Error(), Hint: "Set KRYTON_DOCKUR_DATA_DIR to a writable path"}
	}
	probe := filepath.Join(dir, ".kryton-write-test")
	if err := os.WriteFile(probe, []byte("ok"), 0o644); err != nil {
		return model.DoctorFinding{Check: "data-dir", Status: "fail", Message: err.Error()}
	}
	_ = os.Remove(probe)
	return model.DoctorFinding{Check: "data-dir", Status: "pass", Message: fmt.Sprintf("Writable data dir %s", dir)}
}

func checkHint(name, status, message, hint string) model.DoctorFinding {
	return model.DoctorFinding{Check: name, Status: status, Message: message, Hint: hint}
}

func checkKubeVirtImages(ctx context.Context, kc *kubeapi.Client, cat *catalog.Catalog, imageNS string) model.DoctorFinding {
	if kc == nil {
		return checkHint("kubevirt-images", "warn", "CDI DataSource check skipped (no Kubernetes client)", "Run scripts/bootstrap-kubevirt-images.sh")
	}
	if imageNS == "" {
		imageNS = "kryton-images"
	}
	if cat == nil || len(cat.List()) == 0 {
		return model.DoctorFinding{Check: "kubevirt-images", Status: "fail", Message: "Image catalog is empty"}
	}
	missing := []string{}
	for _, img := range cat.List() {
		path := fmt.Sprintf("/apis/cdi.kubevirt.io/v1beta1/namespaces/%s/datasources/%s", imageNS, img.ID)
		var out map[string]any
		if err := kc.JSON(ctx, "GET", path, "", nil, &out); err != nil {
			if kubeapi.IsNotFound(err) {
				missing = append(missing, img.ID)
				continue
			}
			return model.DoctorFinding{Check: "kubevirt-images", Status: "fail", Message: err.Error(), Hint: "Confirm CDI is installed and Kryton can read DataSources"}
		}
	}
	if len(missing) > 0 {
		return model.DoctorFinding{
			Check: "kubevirt-images", Status: "fail",
			Message: fmt.Sprintf("Missing DataSources in %s: %s", imageNS, strings.Join(missing, ", ")),
			Hint:    "Build: scripts/build-golden-image.sh — Bootstrap: scripts/bootstrap-kubevirt-images.sh (see docs/GOLDEN-IMAGES.md)",
		}
	}
	return model.DoctorFinding{Check: "kubevirt-images", Status: "pass", Message: fmt.Sprintf("All %d catalog image(s) have CDI DataSources in %s", len(cat.List()), imageNS)}
}

func checkKubeVirtSnapshots(ctx context.Context, kc *kubeapi.Client) model.DoctorFinding {
	if kc == nil {
		return checkHint("kubevirt-snapshots", "warn", "Snapshot CRD check skipped (no Kubernetes client)", "Run scripts/enable-kubevirt-snapshots.sh")
	}
	var kv map[string]any
	if err := kc.JSON(ctx, "GET", "/apis/snapshot.kubevirt.io/v1beta1", "", nil, &kv); err != nil {
		return model.DoctorFinding{Check: "kubevirt-snapshots", Status: "fail", Message: "KubeVirt snapshot API is not available", Hint: "Run scripts/enable-kubevirt-snapshots.sh to enable the Snapshot feature gate"}
	}
	var csi map[string]any
	if err := kc.JSON(ctx, "GET", "/apis/snapshot.storage.k8s.io/v1", "", nil, &csi); err != nil {
		return model.DoctorFinding{Check: "kubevirt-snapshots", Status: "warn", Message: "KubeVirt snapshot CRDs present; CSI VolumeSnapshot API missing", Hint: "Run scripts/enable-kubevirt-snapshots.sh"}
	}
	var classes struct {
		Items []map[string]any `json:"items"`
	}
	if err := kc.JSON(ctx, "GET", "/apis/snapshot.storage.k8s.io/v1/volumesnapshotclasses", "", nil, &classes); err != nil {
		return model.DoctorFinding{Check: "kubevirt-snapshots", Status: "fail", Message: err.Error(), Hint: "Grant Kryton get/list on volumesnapshotclasses"}
	}
	if len(classes.Items) == 0 {
		return model.DoctorFinding{Check: "kubevirt-snapshots", Status: "fail", Message: "No VolumeSnapshotClass; disks on rancher.io/local-path cannot be snapshotted", Hint: "Run scripts/enable-rook-ceph.sh or scripts/enable-kubevirt-snapshots.sh --storage longhorn (docs/STORAGE.md)"}
	}
	if !kubevirtSnapshotGateEnabled(ctx, kc) {
		return model.DoctorFinding{Check: "kubevirt-snapshots", Status: "fail", Message: "KubeVirt Snapshot feature gate is not enabled", Hint: "Run scripts/enable-kubevirt-snapshots.sh"}
	}
	names := make([]string, 0, len(classes.Items))
	for _, item := range classes.Items {
		if n := nestedString(item, "metadata", "name"); n != "" {
			names = append(names, n)
		}
	}
	return model.DoctorFinding{Check: "kubevirt-snapshots", Status: "pass", Message: fmt.Sprintf("Snapshot feature gate on; VolumeSnapshotClass: %s", strings.Join(names, ", "))}
}

func checkKubeVirtStorage(ctx context.Context, kc *kubeapi.Client, storageClass string) model.DoctorFinding {
	if kc == nil {
		return checkHint("kubevirt-storage", "warn", "StorageClass check skipped (no Kubernetes client)", "Set KRYTON_STORAGE_CLASS to rook-ceph-block or longhorn (docs/STORAGE.md)")
	}
	scName := strings.TrimSpace(storageClass)
	if scName == "" {
		return model.DoctorFinding{
			Check: "kubevirt-storage", Status: "warn",
			Message: "KRYTON_STORAGE_CLASS is empty; new VM disks use the cluster default StorageClass",
			Hint:    "Use rook-ceph-block (scripts/enable-rook-ceph.sh) or longhorn (scripts/enable-kubevirt-snapshots.sh). Avoid rancher.io/local-path for snapshottable VMs.",
		}
	}
	var sc map[string]any
	if err := kc.JSON(ctx, "GET", "/apis/storage.k8s.io/v1/storageclasses/"+scName, "", nil, &sc); err != nil {
		return model.DoctorFinding{Check: "kubevirt-storage", Status: "fail", Message: fmt.Sprintf("StorageClass %s not found", scName), Hint: "Install Rook (docs/STORAGE.md) or Longhorn, then set KRYTON_STORAGE_CLASS"}
	}
	provisioner, _ := sc["provisioner"].(string)
	if provisioner == "rancher.io/local-path" || provisioner == "kubernetes.io/no-provisioner" {
		return model.DoctorFinding{Check: "kubevirt-storage", Status: "fail", Message: fmt.Sprintf("StorageClass %s (%s) cannot CSI-snapshot VM disks", scName, provisioner), Hint: "Switch KRYTON_STORAGE_CLASS to rook-ceph-block or longhorn"}
	}
	var classes struct {
		Items []map[string]any `json:"items"`
	}
	if err := kc.JSON(ctx, "GET", "/apis/snapshot.storage.k8s.io/v1/volumesnapshotclasses", "", nil, &classes); err != nil {
		return model.DoctorFinding{Check: "kubevirt-storage", Status: "warn", Message: fmt.Sprintf("StorageClass %s present; could not list VolumeSnapshotClass", scName), Hint: err.Error()}
	}
	for _, item := range classes.Items {
		if driver, _ := item["driver"].(string); driver == provisioner {
			return model.DoctorFinding{Check: "kubevirt-storage", Status: "pass", Message: fmt.Sprintf("StorageClass %s (%s) has a matching VolumeSnapshotClass", scName, provisioner)}
		}
	}
	return model.DoctorFinding{Check: "kubevirt-storage", Status: "fail", Message: fmt.Sprintf("No VolumeSnapshotClass for provisioner %s", provisioner), Hint: "Apply deploy/rook-ceph/volumesnapshotclass.yaml or Longhorn snapshot class (docs/STORAGE.md)"}
}

func kubevirtSnapshotGateEnabled(ctx context.Context, kc *kubeapi.Client) bool {
	var list struct {
		Items []map[string]any `json:"items"`
	}
	if err := kc.JSON(ctx, "GET", "/apis/kubevirt.io/v1/kubevirts", "", nil, &list); err != nil || len(list.Items) == 0 {
		return true
	}
	for _, item := range list.Items {
		dev := nestedMap(item, "spec", "configuration", "developerConfiguration")
		raw, _ := dev["featureGates"].([]any)
		for _, g := range raw {
			if s, ok := g.(string); ok && s == "Snapshot" {
				return true
			}
		}
		return false
	}
	return true
}

func nestedMap(m map[string]any, keys ...string) map[string]any {
	cur := m
	for _, k := range keys {
		v, ok := cur[k].(map[string]any)
		if !ok {
			return map[string]any{}
		}
		cur = v
	}
	return cur
}

func nestedString(m map[string]any, keys ...string) string {
	if len(keys) == 0 {
		return ""
	}
	parent := nestedMap(m, keys[:len(keys)-1]...)
	s, _ := parent[keys[len(keys)-1]].(string)
	return s
}

func checkKubeVirtNamespaces(ctx context.Context, kc *kubeapi.Client, prefix string, projects []string) model.DoctorFinding {
	if kc == nil {
		return checkHint("kubevirt-namespaces", "warn", "Namespace check skipped (no Kubernetes client)", "Kryton auto-creates project namespaces on startup when using kubevirt")
	}
	missing, err := kubevirt.MissingNamespaces(ctx, kc, prefix, projects)
	if err != nil {
		return model.DoctorFinding{Check: "kubevirt-namespaces", Status: "fail", Message: err.Error()}
	}
	if len(missing) > 0 {
		return model.DoctorFinding{Check: "kubevirt-namespaces", Status: "fail", Message: fmt.Sprintf("Missing namespaces: %s", strings.Join(missing, ", ")), Hint: "Restart krytond or run scripts/setup-kubevirt.sh to auto-create project namespaces"}
	}
	return model.DoctorFinding{Check: "kubevirt-namespaces", Status: "pass", Message: fmt.Sprintf("%d project namespace(s) ready", len(projects))}
}

func firstNonEmpty(v, d string) string {
	if strings.TrimSpace(v) != "" {
		return v
	}
	return d
}
