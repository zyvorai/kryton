package dockur

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zyvorai/kryton/internal/id"
	"github.com/zyvorai/kryton/internal/model"
	"github.com/zyvorai/kryton/internal/provider"
)

// VERSION map aligned with dockur/windows FAQ codes.
var defaultVersions = map[string]string{
	"windows-11-enterprise": "11e",
	"windows-server-2025":   "2025",
	"windows-server-2022":   "2022",
	"windows-11-pro":        "11",
	"windows-10-pro":        "10",
	"windows-server-2019":   "2019",
}

type Config struct {
	Runtime    string // docker (default) or podman
	DataDir    string
	PublicHost string // host used in console URLs
	HTTPBase   int    // first host port for web viewer (default 18006)
	RDPBase    int    // first host port for RDP (default 13389)
	Catalog    VersionResolver
}

type VersionResolver interface {
	DockurVersion(imageID string) (string, bool)
}

type Provider struct {
	mu         sync.Mutex
	runtime    string
	dataDir    string
	publicHost string
	httpBase   int
	rdpBase    int
	catalog    VersionResolver
	machines   map[string]map[string]model.Machine // project -> id -> machine
	ports      map[string]portPair                 // machine id -> ports
}

type portPair struct{ HTTP, RDP int }

func New(cfg Config) (*Provider, error) {
	runtime := strings.TrimSpace(cfg.Runtime)
	if runtime == "" {
		runtime = "docker"
	}
	dataDir := strings.TrimSpace(cfg.DataDir)
	if dataDir == "" {
		home, _ := os.UserHomeDir()
		dataDir = filepath.Join(home, ".kryton", "dockur")
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	httpBase := cfg.HTTPBase
	if httpBase == 0 {
		httpBase = 18006
	}
	rdpBase := cfg.RDPBase
	if rdpBase == 0 {
		rdpBase = 13389
	}
	host := strings.TrimSpace(cfg.PublicHost)
	if host == "" {
		host = "127.0.0.1"
	}
	p := &Provider{
		runtime: runtime, dataDir: dataDir, publicHost: host,
		httpBase: httpBase, rdpBase: rdpBase, catalog: cfg.Catalog,
		machines: map[string]map[string]model.Machine{}, ports: map[string]portPair{},
	}
	_ = p.loadState()
	return p, nil
}

func (p *Provider) Name() string { return "dockur" }

func (p *Provider) ConsoleTarget(_ context.Context, project, machineID string) (*provider.ConsoleTarget, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.machines[project][machineID]; !ok {
		return nil, provider.ErrNotFound
	}
	ports, ok := p.ports[machineID]
	if !ok {
		return nil, fmt.Errorf("console ports not allocated for machine %s", machineID)
	}
	return &provider.ConsoleTarget{
		Kind:        "web",
		UpstreamURL: fmt.Sprintf("http://127.0.0.1:%d", ports.HTTP),
	}, nil
}

func (p *Provider) consoleURL(project, machineID string) string {
	return fmt.Sprintf("/api/v1/machines/%s/console/?project=%s", machineID, url.QueryEscape(project))
}

func (p *Provider) upstreamURL(machineID string) string {
	if ports, ok := p.ports[machineID]; ok {
		return fmt.Sprintf("http://127.0.0.1:%d/", ports.HTTP)
	}
	return ""
}

func (p *Provider) Capabilities(context.Context) (model.Capabilities, error) {
	return model.Capabilities{Provider: p.Name(), Snapshots: false, Networks: false, TTL: true, Console: true}, nil
}

func (p *Provider) Health(ctx context.Context) error {
	if _, err := exec.LookPath(p.runtime); err != nil {
		return fmt.Errorf("%s not found: %w", p.runtime, err)
	}
	cmd := exec.CommandContext(ctx, p.runtime, "compose", "version")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s compose unavailable: %v (%s)", p.runtime, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (p *Provider) Create(ctx context.Context, project string, spec model.MachineSpec) (*model.Machine, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.machines[project] == nil {
		p.machines[project] = map[string]model.Machine{}
	}
	for _, existing := range p.machines[project] {
		if existing.Spec.Name == spec.Name {
			return nil, provider.ErrConflict
		}
	}
	version, ok := p.resolveVersion(spec.Image)
	if !ok {
		return nil, fmt.Errorf("%w: image %q has no dockur VERSION mapping", provider.ErrUnsupported, spec.Image)
	}
	if _, err := os.Stat("/dev/kvm"); err != nil {
		return nil, fmt.Errorf("dockur requires /dev/kvm (enable VT-x/AMD-V and load kvm modules): %w", err)
	}

	machineID := id.New()
	ports := p.allocatePortsLocked()
	dir := p.machineDir(project, machineID)
	if err := os.MkdirAll(filepath.Join(dir, "storage"), 0o755); err != nil {
		return nil, err
	}
	compose := renderCompose(spec, version, ports)
	if err := os.WriteFile(filepath.Join(dir, "compose.yml"), []byte(compose), 0o644); err != nil {
		return nil, err
	}
	if err := p.compose(ctx, dir, "up", "-d", "--pull", "missing"); err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}

	now := time.Now().UTC()
	progress := 10
	m := model.Machine{
		ID: machineID, Project: project, Provider: p.Name(), State: model.StateProvisioning, Spec: spec,
		ProviderRef:     model.ProviderRef{Provider: p.Name(), Namespace: project, Name: spec.Name},
		ConsoleURL:      p.consoleURL(project, machineID),
		ProgressPercent: &progress,
		Message:         "Windows image download / unattended install in progress (dockur web viewer)",
		CreatedAt:       now, UpdatedAt: now,
	}
	if spec.TTLMinutes > 0 {
		expires := now.Add(time.Duration(spec.TTLMinutes) * time.Minute)
		m.ExpiresAt = &expires
	}
	p.machines[project][machineID] = m
	p.ports[machineID] = ports
	_ = p.saveStateLocked()
	return clone(m), nil
}

func (p *Provider) Get(ctx context.Context, project, machineID string) (*model.Machine, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	m, ok := p.machines[project][machineID]
	if !ok {
		return nil, provider.ErrNotFound
	}
	p.refreshLocked(ctx, &m)
	p.machines[project][machineID] = m
	_ = p.saveStateLocked()
	return clone(m), nil
}

func (p *Provider) List(ctx context.Context, project string) ([]model.Machine, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []model.Machine
	for id, m := range p.machines[project] {
		p.refreshLocked(ctx, &m)
		p.machines[project][id] = m
		out = append(out, *clone(m))
	}
	_ = p.saveStateLocked()
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (p *Provider) Start(ctx context.Context, project, machineID string) (*model.Machine, error) {
	return p.lifecycle(ctx, project, machineID, "start", model.StateStarting)
}
func (p *Provider) Stop(ctx context.Context, project, machineID string) (*model.Machine, error) {
	return p.lifecycle(ctx, project, machineID, "stop", model.StateStopping)
}

func (p *Provider) lifecycle(ctx context.Context, project, machineID, action string, transient model.MachineState) (*model.Machine, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	m, ok := p.machines[project][machineID]
	if !ok {
		return nil, provider.ErrNotFound
	}
	m.State, m.UpdatedAt = transient, time.Now().UTC()
	dir := p.machineDir(project, machineID)
	if err := p.compose(ctx, dir, action); err != nil {
		m.State = model.StateFailed
		m.Message = err.Error()
		p.machines[project][machineID] = m
		_ = p.saveStateLocked()
		return nil, err
	}
	if action == "stop" {
		m.State = model.StateStopped
		m.Message = "Container stopped"
		m.ProgressPercent = nil
	} else {
		m.State = model.StateRunning
		m.Message = "Container started"
	}
	m.UpdatedAt = time.Now().UTC()
	p.machines[project][machineID] = m
	_ = p.saveStateLocked()
	return clone(m), nil
}

func (p *Provider) Delete(ctx context.Context, project, machineID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.machines[project][machineID]; !ok {
		return provider.ErrNotFound
	}
	dir := p.machineDir(project, machineID)
	_ = p.compose(ctx, dir, "down", "--remove-orphans")
	_ = os.RemoveAll(dir)
	delete(p.machines[project], machineID)
	delete(p.ports, machineID)
	return p.saveStateLocked()
}

func (p *Provider) Snapshot(context.Context, string, string, string) (*model.Snapshot, error) {
	return nil, fmt.Errorf("%w: dockur provider does not support snapshots yet", provider.ErrUnsupported)
}
func (p *Provider) ListSnapshots(context.Context, string, string) ([]model.Snapshot, error) {
	return nil, fmt.Errorf("%w: dockur provider does not support snapshots yet", provider.ErrUnsupported)
}
func (p *Provider) RestoreSnapshot(context.Context, string, string, string) (*model.Snapshot, error) {
	return nil, fmt.Errorf("%w: dockur provider does not support snapshots yet", provider.ErrUnsupported)
}
func (p *Provider) DeleteSnapshot(context.Context, string, string, string) error {
	return fmt.Errorf("%w: dockur provider does not support snapshots yet", provider.ErrUnsupported)
}

func (p *Provider) resolveVersion(imageID string) (string, bool) {
	if p.catalog != nil {
		if v, ok := p.catalog.DockurVersion(imageID); ok && v != "" {
			return v, true
		}
	}
	v, ok := defaultVersions[imageID]
	return v, ok
}

func (p *Provider) allocatePortsLocked() portPair {
	usedHTTP := map[int]bool{}
	usedRDP := map[int]bool{}
	for _, pp := range p.ports {
		usedHTTP[pp.HTTP] = true
		usedRDP[pp.RDP] = true
	}
	httpPort := p.httpBase
	for usedHTTP[httpPort] || !portFree(httpPort) {
		httpPort++
	}
	rdpPort := p.rdpBase
	for usedRDP[rdpPort] || !portFree(rdpPort) {
		rdpPort++
	}
	return portPair{HTTP: httpPort, RDP: rdpPort}
}

func portFree(port int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

func (p *Provider) machineDir(project, machineID string) string {
	return filepath.Join(p.dataDir, project, machineID)
}

func (p *Provider) compose(ctx context.Context, dir string, args ...string) error {
	full := append([]string{"compose", "-f", filepath.Join(dir, "compose.yml"), "--project-directory", dir}, args...)
	cmd := exec.CommandContext(ctx, p.runtime, full...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s compose %v: %v (%s)", p.runtime, args, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (p *Provider) refreshLocked(ctx context.Context, m *model.Machine) {
	p.normalizeConsole(m)
	if m.State == model.StateStopped || m.State == model.StateFailed || m.State == model.StateDeleting {
		return
	}
	upstream := p.upstreamURL(m.ID)
	if upstream == "" {
		return
	}
	cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, upstream, nil)
	if err != nil {
		return
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		if m.State == model.StateProvisioning {
			pct := 35
			m.ProgressPercent = &pct
			m.Message = "Waiting for dockur web viewer (ISO download / Windows setup)"
			m.UpdatedAt = time.Now().UTC()
		}
		return
	}
	defer res.Body.Close()
	if res.StatusCode >= 200 && res.StatusCode < 500 {
		if m.State == model.StateProvisioning || m.State == model.StateStarting {
			m.State = model.StateRunning
			pct := 100
			m.ProgressPercent = &pct
			m.Message = "Console reachable — complete Windows setup in the web viewer, then use RDP"
			m.UpdatedAt = time.Now().UTC()
		}
	}
}

func (p *Provider) normalizeConsole(m *model.Machine) {
	if ports, ok := p.ports[m.ID]; ok && m.Project != "" {
		m.ConsoleURL = p.consoleURL(m.Project, m.ID)
		m.RdpHost = p.publicHost
		m.RdpPort = ports.RDP
	}
}

type persisted struct {
	Machines map[string]map[string]model.Machine `json:"machines"`
	Ports    map[string]portPair                 `json:"ports"`
}

func (p *Provider) statePath() string { return filepath.Join(p.dataDir, "state.json") }

func (p *Provider) saveStateLocked() error {
	b, err := json.MarshalIndent(persisted{Machines: p.machines, Ports: p.ports}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p.statePath(), b, 0o644)
}

func (p *Provider) loadState() error {
	b, err := os.ReadFile(p.statePath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var s persisted
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	if s.Machines != nil {
		p.machines = s.Machines
	}
	if s.Ports != nil {
		p.ports = s.Ports
	}
	for project, machines := range p.machines {
		for id, m := range machines {
			p.normalizeConsole(&m)
			p.machines[project][id] = m
		}
	}
	return nil
}

func renderCompose(spec model.MachineSpec, version string, ports portPair) string {
	ramG := spec.Compute.MemoryMiB / 1024
	if ramG < 1 {
		ramG = 1
	}
	disk := strconv.Itoa(spec.Disk.SizeGiB) + "G"
	return fmt.Sprintf(`services:
  windows:
    image: docker.io/dockurr/windows
    environment:
      VERSION: %q
      RAM_SIZE: %q
      CPU_CORES: %q
      DISK_SIZE: %q
    devices:
      - /dev/kvm
      - /dev/net/tun
    cap_add:
      - NET_ADMIN
    ports:
      - "%d:8006"
      - "%d:3389/tcp"
      - "%d:3389/udp"
    volumes:
      - ./storage:/storage
    restart: unless-stopped
    stop_grace_period: 2m
`, version, fmt.Sprintf("%dG", ramG), strconv.Itoa(spec.Compute.CPU), disk, ports.HTTP, ports.RDP, ports.RDP)
}

func clone(m model.Machine) *model.Machine {
	c := m
	c.IPAddresses = append([]string(nil), m.IPAddresses...)
	c.Conditions = append([]model.Condition(nil), m.Conditions...)
	if m.ProgressPercent != nil {
		v := *m.ProgressPercent
		c.ProgressPercent = &v
	}
	if m.Spec.Labels != nil {
		c.Spec.Labels = map[string]string{}
		for k, v := range m.Spec.Labels {
			c.Spec.Labels[k] = v
		}
	}
	return &c
}
