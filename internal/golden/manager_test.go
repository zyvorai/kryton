package golden

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zyvorai/kryton/internal/model"
)

func TestBootstrapRequiresReadyArtifact(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "bootstrap.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	m, err := New(Config{BaseDir: dir, BootstrapPath: script, ImageNamespace: "kryton-images"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Bootstrap("missing"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	workdir := filepath.Join(dir, "11e-1")
	build := model.GoldenBuild{ID: "11e-1", ImageID: "windows-11-enterprise", State: model.GoldenInstalling, StartedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := m.writeStatus(workdir, build); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Bootstrap("11e-1"); err != ErrNotReady {
		t.Fatalf("expected ErrNotReady, got %v", err)
	}

	qcow := filepath.Join(dir, "win.qcow2")
	if err := os.WriteFile(qcow, []byte("qcow"), 0o644); err != nil {
		t.Fatal(err)
	}
	build.State = model.GoldenReady
	build.OutputPath = qcow
	if err := m.writeStatus(workdir, build); err != nil {
		t.Fatal(err)
	}
	got, err := m.Bootstrap("11e-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.BootstrapState != "running" {
		t.Fatalf("bootstrap state %q", got.BootstrapState)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		cur, err := m.Get("11e-1")
		if err != nil {
			t.Fatal(err)
		}
		if cur.BootstrapState == "ready" {
			if cur.DataSource != "kryton-images/windows-11-enterprise" {
				t.Fatalf("datasource %q", cur.DataSource)
			}
			return
		}
		if cur.BootstrapState == "failed" {
			t.Fatalf("bootstrap failed: %s", cur.BootstrapMessage)
		}
		time.Sleep(20 * time.Millisecond)
	}
	raw, _ := os.ReadFile(filepath.Join(workdir, "status.json"))
	t.Fatalf("bootstrap did not finish: %s", raw)
}

func TestBootstrapDisabledWithoutScript(t *testing.T) {
	m, err := New(Config{BaseDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Bootstrap("x"); err != ErrBootstrapDisabled {
		t.Fatalf("expected ErrBootstrapDisabled, got %v", err)
	}
}

func TestWriteStatusRoundTrip(t *testing.T) {
	dir := t.TempDir()
	m, err := New(Config{BaseDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	b := model.GoldenBuild{ID: "a", ImageID: "windows-11-enterprise", State: model.GoldenReady}
	if err := m.writeStatus(filepath.Join(dir, "a"), b); err != nil {
		t.Fatal(err)
	}
	got, err := m.Get("a")
	if err != nil {
		t.Fatal(err)
	}
	if got.State != model.GoldenReady {
		t.Fatalf("state %s", got.State)
	}
	var parsed model.GoldenBuild
	raw, _ := os.ReadFile(filepath.Join(dir, "a", "status.json"))
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatal(err)
	}
}
