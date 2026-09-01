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

// Package dockur implements provider.Provider on top of dockur/windows:
// it drives Docker or Podman Compose to run real Windows guests in
// containers, mapping Kryton image IDs (e.g. "windows-11-enterprise") to
// dockur VERSION codes (e.g. "11e") and exposing web-viewer/RDP ports per
// machine for lab and development use.
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
	"windows-11-pro":        "11",
	"windows-11-ltsc":       "11l",
	"windows-10-enterprise": "10e",
	"windows-10-pro":        "10",
	"windows-10-ltsc":       "10l",
	"windows-tiny11":        "tiny11",
	"windows-tiny11-core":   "core11",
	"windows-server-2025":   "2025",
	"windows-server-2022":   "2022",
	"windows-server-2019":   "2019",
	"windows-server-2016":   "2016",
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
	spec = applyDockurDefaults(spec)
	if err := prepareDockurDirs(dir, spec.Dockur); err != nil {
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
		RdpUsername:     dockurUsername(spec),
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
	if m.RdpUsername == "" {
		m.RdpUsername = dockurUsername(m.Spec)
	}
}

func applyDockurDefaults(spec model.MachineSpec) model.MachineSpec {
	d := spec.Dockur
	if d == nil {
		d = &model.DockurOptions{}
	}
	cp := *d
	if strings.TrimSpace(cp.Username) == "" {
		cp.Username = "Docker"
	}
	if strings.TrimSpace(cp.Password) == "" {
		cp.Password = "admin"
	}
	if strings.TrimSpace(cp.Hostname) == "" {
		cp.Hostname = spec.Name
	}
	spec.Dockur = &cp
	return spec
}

func dockurUsername(spec model.MachineSpec) string {
	if spec.Dockur != nil && strings.TrimSpace(spec.Dockur.Username) != "" {
		return spec.Dockur.Username
	}
	return "Docker"
}

func prepareDockurDirs(machineDir string, d *model.DockurOptions) error {
	if d == nil {
		return nil
	}
	for i := range d.ExtraDisksGiB {
		path := filepath.Join(machineDir, fmt.Sprintf("storage%d", i+2))
		if err := os.MkdirAll(path, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func yamlQuote(v string) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func renderCompose(spec model.MachineSpec, version string, ports portPair) string {
	ramG := spec.Compute.MemoryMiB / 1024
	if ramG < 1 {
		ramG = 1
	}
	disk := strconv.Itoa(spec.Disk.SizeGiB) + "G"
	d := spec.Dockur
	if d == nil {
		d = &model.DockurOptions{Username: "Docker", Password: "admin", Hostname: spec.Name}
	}

	ver := version
	if u := strings.TrimSpace(d.CustomISO); u != "" && (strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://")) {
		ver = u
	}

	var env []string
	add := func(k, v string) {
		v = strings.TrimSpace(v)
		if v == "" {
			return
		}
		env = append(env, fmt.Sprintf("      %s: %s", k, yamlQuote(v)))
	}
	add("VERSION", ver)
	add("RAM_SIZE", fmt.Sprintf("%dG", ramG))
	add("CPU_CORES", strconv.Itoa(spec.Compute.CPU))
	add("DISK_SIZE", disk)
	add("USERNAME", d.Username)
	add("PASSWORD", d.Password)
	add("HOST", d.Hostname)
	add("LANGUAGE", d.Language)
	add("REGION", d.Region)
	add("KEYBOARD", d.Keyboard)
	add("KEY", d.ProductKey)
	add("DOMAIN", d.Domain)
	add("DOMAIN_OU", d.DomainOU)
	add("EDITION", d.Edition)
	add("COMMAND", d.Command)
	if d.Audio {
		add("AUDIO", "Y")
	}
	if d.SecureBoot {
		add("BOOT_MODE", "windows_secure")
		add("TPM", "Y")
	}
	if d.Autologin != nil && !*d.Autologin {
		add("AUTOLOGIN", "N")
	}
	for i, g := range d.ExtraDisksGiB {
		add(fmt.Sprintf("DISK%d_SIZE", i+2), strconv.Itoa(g)+"G")
	}

	var vols []string
	vols = append(vols, "      - ./storage:/storage")
	if share := strings.TrimSpace(d.SharedDir); share != "" {
		vols = append(vols, fmt.Sprintf("      - %s", yamlQuote(share+":/shared")))
	}
	if oem := strings.TrimSpace(d.OemDir); oem != "" {
		vols = append(vols, fmt.Sprintf("      - %s", yamlQuote(oem+":/oem")))
	}
	if iso := strings.TrimSpace(d.CustomISO); iso != "" && !strings.HasPrefix(iso, "http://") && !strings.HasPrefix(iso, "https://") {
		vols = append(vols, fmt.Sprintf("      - %s", yamlQuote(iso+":/custom.iso")))
	}
	for i := range d.ExtraDisksGiB {
		vols = append(vols, fmt.Sprintf("      - ./storage%d:/storage%d", i+2, i+2))
	}

	return fmt.Sprintf(`services:
  windows:
    image: docker.io/dockurr/windows
    environment:
%s
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
%s
    restart: unless-stopped
    stop_grace_period: 2m
`, strings.Join(env, "\n"), ports.HTTP, ports.RDP, ports.RDP, strings.Join(vols, "\n"))
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
	if m.Spec.Dockur != nil {
		d := *m.Spec.Dockur
		d.Password = "" // never return guest password over the API
		if len(m.Spec.Dockur.ExtraDisksGiB) > 0 {
			d.ExtraDisksGiB = append([]int(nil), m.Spec.Dockur.ExtraDisksGiB...)
		}
		c.Spec.Dockur = &d
	}
	return &c
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
	return os.WriteFile(p.statePath(), b, 0o600)
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
