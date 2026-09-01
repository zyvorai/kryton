package kubevirt

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/zyvorai/kryton/internal/id"
	"github.com/zyvorai/kryton/internal/kubeapi"
	"github.com/zyvorai/kryton/internal/model"
	"github.com/zyvorai/kryton/internal/provider"
)

const (
	managedLabel = "kryton.io/managed"
	idLabel      = "kryton.io/id"
	projectLabel = "kryton.io/project"
	specAnno     = "kryton.io/spec"
	expiresAnno  = "kryton.io/expires-at"
)

type Config struct {
	Client          *kubeapi.Client
	NamespacePrefix string
	ImageNamespace  string
	StorageClass    string
}

type Provider struct {
	client                          *kubeapi.Client
	namespacePrefix, imageNamespace string
	mu                              sync.RWMutex
	storageClass                    string
}

func New(cfg Config) *Provider {
	return &Provider{client: cfg.Client, namespacePrefix: cfg.NamespacePrefix, imageNamespace: cfg.ImageNamespace, storageClass: cfg.StorageClass}
}
func (p *Provider) Name() string                    { return "kubevirt" }
func (p *Provider) namespace(project string) string { return p.namespacePrefix + project }

func (p *Provider) StorageClass() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.storageClass
}

func (p *Provider) SetStorageClass(name string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.storageClass = strings.TrimSpace(name)
}

func (p *Provider) SetImageNamespace(ns string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.imageNamespace = strings.TrimSpace(ns)
}

func (p *Provider) ImageNamespace() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.imageNamespace
}

func (p *Provider) Health(ctx context.Context) error {
	if err := p.client.Health(ctx); err != nil {
		return err
	}
	var out map[string]any
	return p.client.JSON(ctx, http.MethodGet, "/apis/kubevirt.io/v1", "", nil, &out)
}

func (p *Provider) Capabilities(context.Context) (model.Capabilities, error) {
	return model.Capabilities{Provider: p.Name(), Snapshots: true, Networks: true, TTL: true, LiveMigration: false, Console: true}, nil
}

func (p *Provider) ConsoleTarget(ctx context.Context, project, machineID string) (*provider.ConsoleTarget, error) {
	m, err := p.Get(ctx, project, machineID)
	if err != nil {
		return nil, err
	}
	if m.ProviderRef.Name == "" || m.ProviderRef.Namespace == "" {
		return nil, provider.ErrNotFound
	}
	vmi := p.getVMI(ctx, project, m.ProviderRef.Name)
	if vmi == nil {
		return nil, fmt.Errorf("console unavailable: guest instance is not running yet")
	}
	return &provider.ConsoleTarget{Namespace: m.ProviderRef.Namespace, Name: m.ProviderRef.Name, Kind: "vnc"}, nil
}

func (p *Provider) Create(ctx context.Context, project string, spec model.MachineSpec) (*model.Machine, error) {
	if err := EnsureNamespaces(ctx, p.client, p.namespacePrefix, []string{project}); err != nil {
		return nil, err
	}
	machineID := id.New()
	ns := p.namespace(project)
	rootName := trimDNS(spec.Name+"-root", 63)
	specJSON, _ := json.Marshal(spec)
	labels := map[string]any{managedLabel: "true", idLabel: machineID, projectLabel: project, "app.kubernetes.io/managed-by": "kryton"}
	annotations := map[string]any{specAnno: string(specJSON), "kryton.io/image": spec.Image}
	var expires *time.Time
	if spec.TTLMinutes > 0 {
		x := time.Now().UTC().Add(time.Duration(spec.TTLMinutes) * time.Minute)
		expires = &x
		annotations[expiresAnno] = x.Format(time.RFC3339)
	}

	iface := map[string]any{"name": "default", "masquerade": map[string]any{}}
	network := map[string]any{"name": "default", "pod": map[string]any{}}
	if spec.Network.NetworkID != "" {
		iface = map[string]any{"name": "default", "bridge": map[string]any{}}
		network = map[string]any{"name": "default", "multus": map[string]any{"networkName": spec.Network.NetworkID}}
	}
	storage := map[string]any{
		"resources":   map[string]any{"requests": map[string]any{"storage": fmt.Sprintf("%dGi", spec.Disk.SizeGiB)}},
		"accessModes": []any{"ReadWriteOnce"},
	}
	if sc := firstNonEmpty(spec.Disk.StorageClass, p.StorageClass()); sc != "" {
		storage["storageClassName"] = sc
	}
	vm := map[string]any{
		"apiVersion": "kubevirt.io/v1", "kind": "VirtualMachine",
		"metadata": map[string]any{"name": spec.Name, "namespace": ns, "labels": labels, "annotations": annotations},
		"spec": map[string]any{
			"runStrategy": "Always",
			"dataVolumeTemplates": []any{map[string]any{
				"metadata": map[string]any{"name": rootName},
				"spec": map[string]any{
					"sourceRef": map[string]any{"kind": "DataSource", "name": spec.Image, "namespace": p.imageNamespace},
					"storage":   storage,
				},
			}},
			"template": map[string]any{
				"metadata": map[string]any{"labels": labels},
				"spec": map[string]any{
					"terminationGracePeriodSeconds": 30,
					"domain": map[string]any{
						"cpu":      map[string]any{"sockets": 1, "cores": spec.Compute.CPU, "threads": 1},
						"memory":   map[string]any{"guest": fmt.Sprintf("%dMi", spec.Compute.MemoryMiB)},
						"features": map[string]any{"acpi": map[string]any{}, "apic": map[string]any{}, "hyperv": map[string]any{"relaxed": map[string]any{}, "vapic": map[string]any{}, "spinlocks": map[string]any{"spinlocks": 8191}}},
						"devices":  map[string]any{"disks": []any{map[string]any{"name": "root", "disk": map[string]any{"bus": "virtio"}}}, "interfaces": []any{iface}},
					},
					"networks": []any{network},
					"volumes":  []any{map[string]any{"name": "root", "dataVolume": map[string]any{"name": rootName}}},
				},
			},
		},
	}
	var created map[string]any
	path := fmt.Sprintf("/apis/kubevirt.io/v1/namespaces/%s/virtualmachines", url.PathEscape(ns))
	if err := p.client.JSON(ctx, http.MethodPost, path, "application/json", vm, &created); err != nil {
		if kubeapi.IsConflict(err) {
			return nil, provider.ErrConflict
		}
		return nil, err
	}
	m := p.mapVM(project, created, nil)
	m.ExpiresAt = expires
	return &m, nil
}

func (p *Provider) Get(ctx context.Context, project, machineID string) (*model.Machine, error) {
	items, err := p.listVMs(ctx, project, fmt.Sprintf("%s=%s", idLabel, machineID))
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, provider.ErrNotFound
	}
	vmi := p.getVMI(ctx, project, nestedString(items[0], "metadata", "name"))
	m := p.mapVM(project, items[0], vmi)
	return &m, nil
}

func (p *Provider) List(ctx context.Context, project string) ([]model.Machine, error) {
	vms, err := p.listVMs(ctx, project, managedLabel+"=true")
	if err != nil {
		return nil, err
	}
	vmis := p.listVMIs(ctx, project)
	byName := map[string]map[string]any{}
	for _, vmi := range vmis {
		byName[nestedString(vmi, "metadata", "name")] = vmi
	}
	out := make([]model.Machine, 0, len(vms))
	for _, vm := range vms {
		out = append(out, p.mapVM(project, vm, byName[nestedString(vm, "metadata", "name")]))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (p *Provider) Start(ctx context.Context, project, machineID string) (*model.Machine, error) {
	return p.setRunStrategy(ctx, project, machineID, "Always")
}
func (p *Provider) Stop(ctx context.Context, project, machineID string) (*model.Machine, error) {
	return p.setRunStrategy(ctx, project, machineID, "Halted")
}
func (p *Provider) setRunStrategy(ctx context.Context, project, machineID, strategy string) (*model.Machine, error) {
	m, err := p.Get(ctx, project, machineID)
	if err != nil {
		return nil, err
	}
	path := fmt.Sprintf("/apis/kubevirt.io/v1/namespaces/%s/virtualmachines/%s", url.PathEscape(m.ProviderRef.Namespace), url.PathEscape(m.ProviderRef.Name))
	patch := map[string]any{"spec": map[string]any{"runStrategy": strategy}}
	var out map[string]any
	if err := p.client.JSON(ctx, http.MethodPatch, path, "application/merge-patch+json", patch, &out); err != nil {
		if kubeapi.IsNotFound(err) {
			return nil, provider.ErrNotFound
		}
		return nil, err
	}
	vmi := p.getVMI(ctx, project, m.ProviderRef.Name)
	mapped := p.mapVM(project, out, vmi)
	return &mapped, nil
}

func (p *Provider) Delete(ctx context.Context, project, machineID string) error {
	m, err := p.Get(ctx, project, machineID)
	if err != nil {
		return err
	}
	path := fmt.Sprintf("/apis/kubevirt.io/v1/namespaces/%s/virtualmachines/%s", url.PathEscape(m.ProviderRef.Namespace), url.PathEscape(m.ProviderRef.Name))
	body := map[string]any{"propagationPolicy": "Foreground"}
	if err := p.client.JSON(ctx, http.MethodDelete, path, "application/json", body, nil); err != nil {
		if kubeapi.IsNotFound(err) {
			return provider.ErrNotFound
		}
		return err
	}
	return nil
}

func (p *Provider) Snapshot(ctx context.Context, project, machineID, name string) (*model.Snapshot, error) {
	m, err := p.Get(ctx, project, machineID)
	if err != nil {
		return nil, err
	}
	if name == "" {
		name = m.Spec.Name + "-" + time.Now().UTC().Format("20060102-150405")
	}
	name = sanitizeDNS(name, 63)
	snapshotID := id.New()
	obj := map[string]any{
		"apiVersion": "snapshot.kubevirt.io/v1beta1", "kind": "VirtualMachineSnapshot",
		"metadata": map[string]any{"name": name, "namespace": m.ProviderRef.Namespace, "labels": map[string]any{"kryton.io/snapshot-id": snapshotID, "kryton.io/machine-id": machineID, "app.kubernetes.io/managed-by": "kryton"}},
		"spec":     map[string]any{"source": map[string]any{"apiGroup": "kubevirt.io", "kind": "VirtualMachine", "name": m.ProviderRef.Name}},
	}
	var out map[string]any
	path := fmt.Sprintf("/apis/snapshot.kubevirt.io/v1beta1/namespaces/%s/virtualmachinesnapshots", url.PathEscape(m.ProviderRef.Namespace))
	if err := p.client.JSON(ctx, http.MethodPost, path, "application/json", obj, &out); err != nil {
		if kubeapi.IsConflict(err) {
			return nil, provider.ErrConflict
		}
		return nil, err
	}
	created := parseTime(nestedString(out, "metadata", "creationTimestamp"))
	if created.IsZero() {
		created = time.Now().UTC()
	}
	return &model.Snapshot{ID: snapshotID, Project: project, MachineID: machineID, Name: name, State: "creating", CreatedAt: created}, nil
}

func (p *Provider) ListSnapshots(ctx context.Context, project, machineID string) ([]model.Snapshot, error) {
	m, err := p.Get(ctx, project, machineID)
	if err != nil {
		return nil, err
	}
	path := fmt.Sprintf("/apis/snapshot.kubevirt.io/v1beta1/namespaces/%s/virtualmachinesnapshots?labelSelector=%s", url.PathEscape(m.ProviderRef.Namespace), url.QueryEscape("kryton.io/machine-id="+machineID))
	var list struct {
		Items []map[string]any `json:"items"`
	}
	if err := p.client.JSON(ctx, http.MethodGet, path, "", nil, &list); err != nil {
		return nil, err
	}
	out := make([]model.Snapshot, 0, len(list.Items))
	for _, item := range list.Items {
		out = append(out, mapVMSnapshot(project, machineID, item))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (p *Provider) RestoreSnapshot(ctx context.Context, project, machineID, snapshotID string) (*model.Snapshot, error) {
	m, err := p.Get(ctx, project, machineID)
	if err != nil {
		return nil, err
	}
	snap, obj, err := p.getVMSnapshot(ctx, project, machineID, snapshotID)
	if err != nil {
		return nil, err
	}
	if snap.State != "ready" {
		return nil, fmt.Errorf("%w: snapshot %s is %s", provider.ErrUnsupported, snap.Name, snap.State)
	}
	if m.State == model.StateRunning || m.State == model.StateStarting {
		if _, err := p.Stop(ctx, project, machineID); err != nil {
			return nil, err
		}
	}
	restoreName := sanitizeDNS("restore-"+snap.Name+"-"+time.Now().UTC().Format("150405"), 63)
	body := map[string]any{
		"apiVersion": "snapshot.kubevirt.io/v1beta1", "kind": "VirtualMachineRestore",
		"metadata": map[string]any{"name": restoreName, "namespace": m.ProviderRef.Namespace, "labels": map[string]any{"kryton.io/snapshot-id": snap.ID, "kryton.io/machine-id": machineID, "app.kubernetes.io/managed-by": "kryton"}},
		"spec": map[string]any{
			"target":                      map[string]any{"apiGroup": "kubevirt.io", "kind": "VirtualMachine", "name": m.ProviderRef.Name},
			"virtualMachineSnapshotName": nestedString(obj, "metadata", "name"),
		},
	}
	path := fmt.Sprintf("/apis/snapshot.kubevirt.io/v1beta1/namespaces/%s/virtualmachinerestores", url.PathEscape(m.ProviderRef.Namespace))
	if err := p.client.JSON(ctx, http.MethodPost, path, "application/json", body, nil); err != nil {
		if kubeapi.IsConflict(err) {
			return nil, provider.ErrConflict
		}
		return nil, err
	}
	snap.State = "restoring"
	snap.Message = "VirtualMachineRestore " + restoreName + " requested"
	return snap, nil
}

func (p *Provider) DeleteSnapshot(ctx context.Context, project, machineID, snapshotID string) error {
	_, obj, err := p.getVMSnapshot(ctx, project, machineID, snapshotID)
	if err != nil {
		return err
	}
	ns := nestedString(obj, "metadata", "namespace")
	name := nestedString(obj, "metadata", "name")
	path := fmt.Sprintf("/apis/snapshot.kubevirt.io/v1beta1/namespaces/%s/virtualmachinesnapshots/%s", url.PathEscape(ns), url.PathEscape(name))
	if err := p.client.JSON(ctx, http.MethodDelete, path, "application/json", map[string]any{"propagationPolicy": "Foreground"}, nil); err != nil {
		if kubeapi.IsNotFound(err) {
			return provider.ErrNotFound
		}
		return err
	}
	return nil
}

func (p *Provider) getVMSnapshot(ctx context.Context, project, machineID, snapshotID string) (*model.Snapshot, map[string]any, error) {
	m, err := p.Get(ctx, project, machineID)
	if err != nil {
		return nil, nil, err
	}
	path := fmt.Sprintf("/apis/snapshot.kubevirt.io/v1beta1/namespaces/%s/virtualmachinesnapshots?labelSelector=%s", url.PathEscape(m.ProviderRef.Namespace), url.QueryEscape("kryton.io/machine-id="+machineID))
	var list struct {
		Items []map[string]any `json:"items"`
	}
	if err := p.client.JSON(ctx, http.MethodGet, path, "", nil, &list); err != nil {
		return nil, nil, err
	}
	for _, item := range list.Items {
		s := mapVMSnapshot(project, machineID, item)
		if s.ID == snapshotID || nestedString(item, "metadata", "name") == snapshotID {
			return &s, item, nil
		}
	}
	return nil, nil, provider.ErrNotFound
}

func mapVMSnapshot(project, machineID string, obj map[string]any) model.Snapshot {
	labels := anyStringMap(nestedMap(obj, "metadata")["labels"])
	sid := labels["kryton.io/snapshot-id"]
	name := nestedString(obj, "metadata", "name")
	if sid == "" {
		sid = name
	}
	status := nestedMap(obj, "status")
	state := "creating"
	if ready, ok := status["readyToUse"].(bool); ok && ready {
		state = "ready"
	}
	switch stringAny(status["phase"]) {
	case "Succeeded", "Ready":
		state = "ready"
	case "Failed":
		state = "failed"
	}
	msg := ""
	if errObj, ok := status["error"].(map[string]any); ok {
		msg = stringAny(errObj["message"])
		if msg != "" {
			state = "failed"
		}
	}
	created := parseTime(nestedString(obj, "metadata", "creationTimestamp"))
	if created.IsZero() {
		created = time.Now().UTC()
	}
	return model.Snapshot{ID: sid, Project: project, MachineID: machineID, Name: name, State: state, Message: msg, CreatedAt: created}
}

func (p *Provider) listVMs(ctx context.Context, project, selector string) ([]map[string]any, error) {
	ns := p.namespace(project)
	path := fmt.Sprintf("/apis/kubevirt.io/v1/namespaces/%s/virtualmachines?labelSelector=%s", url.PathEscape(ns), url.QueryEscape(selector))
	var list struct {
		Items []map[string]any `json:"items"`
	}
	if err := p.client.JSON(ctx, http.MethodGet, path, "", nil, &list); err != nil {
		return nil, err
	}
	return list.Items, nil
}
func (p *Provider) listVMIs(ctx context.Context, project string) []map[string]any {
	ns := p.namespace(project)
	path := fmt.Sprintf("/apis/kubevirt.io/v1/namespaces/%s/virtualmachineinstances?labelSelector=%s", url.PathEscape(ns), url.QueryEscape(managedLabel+"=true"))
	var list struct {
		Items []map[string]any `json:"items"`
	}
	if err := p.client.JSON(ctx, http.MethodGet, path, "", nil, &list); err != nil {
		return nil
	}
	return list.Items
}
func (p *Provider) getVMI(ctx context.Context, project, name string) map[string]any {
	if name == "" {
		return nil
	}
	path := fmt.Sprintf("/apis/kubevirt.io/v1/namespaces/%s/virtualmachineinstances/%s", url.PathEscape(p.namespace(project)), url.PathEscape(name))
	var out map[string]any
	if err := p.client.JSON(ctx, http.MethodGet, path, "", nil, &out); err != nil {
		return nil
	}
	return out
}

func (p *Provider) mapVM(project string, vm, vmi map[string]any) model.Machine {
	meta := nestedMap(vm, "metadata")
	labels := anyStringMap(meta["labels"])
	annotations := anyStringMap(meta["annotations"])
	var spec model.MachineSpec
	if raw := annotations[specAnno]; raw != "" {
		_ = json.Unmarshal([]byte(raw), &spec)
	}
	if spec.Name == "" {
		spec.Name = stringAny(meta["name"])
		spec.Image = annotations["kryton.io/image"]
	}
	created := parseTime(stringAny(meta["creationTimestamp"]))
	if created.IsZero() {
		created = time.Now().UTC()
	}
	m := model.Machine{
		ID: labels[idLabel], Project: project, Provider: p.Name(), State: mapState(nestedString(vm, "status", "printableStatus")), Spec: spec,
		ProviderRef: model.ProviderRef{Provider: p.Name(), Namespace: stringAny(meta["namespace"]), Name: stringAny(meta["name"])}, CreatedAt: created, UpdatedAt: time.Now().UTC(),
	}
	if m.ID == "" {
		m.ID = m.ProviderRef.Name
	}
	if exp := parseTime(annotations[expiresAnno]); !exp.IsZero() {
		m.ExpiresAt = &exp
	}
	m.IPAddresses = vmiIPs(vmi)
	m.Conditions = vmConditions(vm)
	if vmi != nil && m.ID != "" {
		m.ConsoleURL = fmt.Sprintf("/api/v1/machines/%s/console?project=%s", m.ID, url.QueryEscape(project))
	}
	return m
}

func mapState(v string) model.MachineState {
	s := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(v), " ", ""))
	switch s {
	case "running":
		return model.StateRunning
	case "stopped", "halted":
		return model.StateStopped
	case "starting":
		return model.StateStarting
	case "stopping":
		return model.StateStopping
	case "migrating":
		return model.StateMigrating
	case "paused":
		return model.StatePaused
	case "terminating":
		return model.StateDeleting
	case "provisioning", "scheduling", "waitingforvolumebinding", "importing", "cloning", "creating":
		return model.StateProvisioning
	case "crashloopbackoff", "errimagepull", "imagepullbackoff", "pvcnotfound", "datavolumeerror", "unschedulable", "error":
		return model.StateFailed
	case "":
		return model.StatePending
	default:
		return model.StateUnknown
	}
}

func vmiIPs(vmi map[string]any) []string {
	status := nestedMap(vmi, "status")
	raw, _ := status["interfaces"].([]any)
	seen := map[string]bool{}
	var out []string
	for _, x := range raw {
		im, _ := x.(map[string]any)
		if ip := stringAny(im["ipAddress"]); ip != "" && !seen[ip] {
			seen[ip] = true
			out = append(out, ip)
		}
		if ips, ok := im["ipAddresses"].([]any); ok {
			for _, z := range ips {
				ip := stringAny(z)
				if ip != "" && !seen[ip] {
					seen[ip] = true
					out = append(out, ip)
				}
			}
		}
	}
	return out
}
func vmConditions(vm map[string]any) []model.Condition {
	status := nestedMap(vm, "status")
	raw, _ := status["conditions"].([]any)
	var out []model.Condition
	for _, x := range raw {
		m, _ := x.(map[string]any)
		out = append(out, model.Condition{Type: stringAny(m["type"]), Status: stringAny(m["status"]), Reason: stringAny(m["reason"]), Message: stringAny(m["message"])})
	}
	return out
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
	return stringAny(parent[keys[len(keys)-1]])
}
func stringAny(v any) string { s, _ := v.(string); return s }
func anyStringMap(v any) map[string]string {
	out := map[string]string{}
	m, _ := v.(map[string]any)
	for k, x := range m {
		if s, ok := x.(string); ok {
			out[k] = s
		}
	}
	return out
}
func parseTime(v string) time.Time { t, _ := time.Parse(time.RFC3339, v); return t }
func sanitizeDNS(v string, max int) string {
	v = strings.ToLower(v)
	var b strings.Builder
	lastDash := false
	for _, r := range v {
		valid := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if valid {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "snapshot"
	}
	return trimDNS(out, max)
}

func trimDNS(v string, max int) string {
	v = strings.Trim(v, "-")
	if len(v) <= max {
		return v
	}
	v = v[:max]
	return strings.TrimRight(v, "-")
}

func firstNonEmpty(v, d string) string {
	if strings.TrimSpace(v) != "" {
		return v
	}
	return d
}
