package dockur

import (
	"strings"
	"testing"

	"github.com/zyvorai/kryton/internal/model"
)

type mapResolver map[string]string

func (m mapResolver) DockurVersion(id string) (string, bool) {
	v, ok := m[id]
	return v, ok
}

func TestResolveDefaultVersions(t *testing.T) {
	p := &Provider{catalog: nil}
	v, ok := p.resolveVersion("windows-server-2025")
	if !ok || v != "2025" {
		t.Fatalf("got %q %v", v, ok)
	}
	v, ok = p.resolveVersion("windows-11-enterprise")
	if !ok || v != "11e" {
		t.Fatalf("got %q %v", v, ok)
	}
}

func TestCatalogOverridesDefault(t *testing.T) {
	p := &Provider{catalog: mapResolver{"windows-server-2025": "2025"}}
	v, ok := p.resolveVersion("windows-server-2025")
	if !ok || v != "2025" {
		t.Fatalf("got %q %v", v, ok)
	}
}

func TestConsoleURLAndRDP(t *testing.T) {
	p := &Provider{
		publicHost: "lab.example",
		ports:      map[string]portPair{"m1": {HTTP: 18006, RDP: 13389}},
	}
	m := &model.Machine{ID: "m1", Project: "default"}
	p.normalizeConsole(m)
	want := "/api/v1/machines/m1/console/?project=default"
	if m.ConsoleURL != want {
		t.Fatalf("consoleUrl = %q, want %q", m.ConsoleURL, want)
	}
	if m.RdpHost != "lab.example" || m.RdpPort != 13389 {
		t.Fatalf("rdp = %s:%d", m.RdpHost, m.RdpPort)
	}
}

func TestRenderCompose(t *testing.T) {
	spec := model.MachineSpec{Name: "win1", Image: "windows-11-enterprise", Compute: model.ComputeSpec{CPU: 4, MemoryMiB: 8192}, Disk: model.DiskSpec{SizeGiB: 64}}
	out := renderCompose(spec, "11e", portPair{HTTP: 18006, RDP: 13389})
	for _, want := range []string{`VERSION: "11e"`, `RAM_SIZE: "8G"`, `CPU_CORES: "4"`, `DISK_SIZE: "64G"`, `18006:8006`, `13389:3389`} {
		if !strings.Contains(out, want) {
			t.Fatalf("compose missing %q\n%s", want, out)
		}
	}
}
