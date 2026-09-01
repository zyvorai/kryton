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
	v, ok = p.resolveVersion("windows-tiny11")
	if !ok || v != "tiny11" {
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
	m := &model.Machine{ID: "m1", Project: "default", Spec: model.MachineSpec{Dockur: &model.DockurOptions{Username: "lab"}}}
	p.normalizeConsole(m)
	want := "/api/v1/machines/m1/console/?project=default"
	if m.ConsoleURL != want {
		t.Fatalf("consoleUrl = %q, want %q", m.ConsoleURL, want)
	}
	if m.RdpHost != "lab.example" || m.RdpPort != 13389 || m.RdpUsername != "lab" {
		t.Fatalf("rdp = %s:%d user=%s", m.RdpHost, m.RdpPort, m.RdpUsername)
	}
}

func TestRenderComposeBasics(t *testing.T) {
	spec := model.MachineSpec{Name: "win1", Image: "windows-11-enterprise", Compute: model.ComputeSpec{CPU: 4, MemoryMiB: 8192}, Disk: model.DiskSpec{SizeGiB: 64}}
	out := renderCompose(applyDockurDefaults(spec), "11e", portPair{HTTP: 18006, RDP: 13389})
	for _, want := range []string{`VERSION: "11e"`, `RAM_SIZE: "8G"`, `CPU_CORES: "4"`, `DISK_SIZE: "64G"`, `USERNAME: "Docker"`, `PASSWORD: "admin"`, `HOST: "win1"`, `18006:8006`, `13389:3389`} {
		if !strings.Contains(out, want) {
			t.Fatalf("compose missing %q\n%s", want, out)
		}
	}
}

func TestRenderComposeRichOptions(t *testing.T) {
	auto := false
	spec := model.MachineSpec{
		Name: "lab", Image: "windows-11-pro",
		Compute: model.ComputeSpec{CPU: 2, MemoryMiB: 4096}, Disk: model.DiskSpec{SizeGiB: 40},
		Dockur: &model.DockurOptions{
			Username: "bill", Password: "gates", Language: "French", Region: "fr-FR", Keyboard: "fr-FR",
			ProductKey: "AAAAA-BBBBB-CCCCC-DDDDD-EEEEE", Domain: "example.com", DomainOU: "OU=Labs,DC=example,DC=com",
			Autologin: &auto, Audio: true, SecureBoot: true,
			SharedDir: "/tmp/shared", OemDir: "/tmp/oem", Command: "echo hi",
			ExtraDisksGiB: []int{32, 64}, Edition: "core",
		},
	}
	out := renderCompose(applyDockurDefaults(spec), "11", portPair{HTTP: 18007, RDP: 13390})
	for _, want := range []string{
		`USERNAME: "bill"`, `PASSWORD: "gates"`, `LANGUAGE: "French"`, `AUDIO: "Y"`,
		`BOOT_MODE: "windows_secure"`, `TPM: "Y"`, `AUTOLOGIN: "N"`,
		`DISK2_SIZE: "32G"`, `DISK3_SIZE: "64G"`, `./storage2:/storage2`,
		`"/tmp/shared:/shared"`, `"/tmp/oem:/oem"`, `DOMAIN: "example.com"`, `EDITION: "core"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("compose missing %q\n%s", want, out)
		}
	}
}

func TestCloneRedactsPassword(t *testing.T) {
	m := model.Machine{Spec: model.MachineSpec{Dockur: &model.DockurOptions{Username: "u", Password: "secret"}}}
	c := clone(m)
	if c.Spec.Dockur.Password != "" {
		t.Fatal("password should be redacted")
	}
	if m.Spec.Dockur.Password != "secret" {
		t.Fatal("original password mutated")
	}
}
