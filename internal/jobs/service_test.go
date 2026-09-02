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

package jobs

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zyvorai/kryton/internal/golden"
	"github.com/zyvorai/kryton/internal/model"
	"github.com/zyvorai/kryton/internal/storage"
)

func TestMachineStepIndex(t *testing.T) {
	cases := []struct {
		name string
		m    model.Machine
		want int
	}{
		{"failed", model.Machine{State: model.StateFailed}, 0},
		{"provisioning default", model.Machine{State: model.StateProvisioning}, 1},
		{"provisioning downloading", model.Machine{State: model.StateProvisioning, Message: "Downloading Windows ISO"}, 2},
		{"provisioning waiting", model.Machine{State: model.StateProvisioning, Message: "Waiting for network"}, 2},
		{"provisioning installing", model.Machine{State: model.StateProvisioning, Message: "Installing Windows"}, 3},
		{"running incomplete", model.Machine{State: model.StateRunning, ProgressPercent: intPtr(40)}, 4},
		{"running complete", model.Machine{State: model.StateRunning, ProgressPercent: intPtr(100)}, 5},
		{"running no progress info", model.Machine{State: model.StateRunning}, 4},
		{"starting", model.Machine{State: model.StateStarting}, 1},
		{"unknown state falls back", model.Machine{State: model.StateStopped}, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := machineStepIndex(tc.m); got != tc.want {
				t.Fatalf("got %d, want %d", got, tc.want)
			}
		})
	}
}

func intPtr(v int) *int { return &v }

func TestMachineJobFiltering(t *testing.T) {
	cases := []struct {
		name    string
		m       model.Machine
		wantNil bool
	}{
		{"demo provider always hidden", model.Machine{Provider: "demo", State: model.StateProvisioning}, true},
		{"provisioning shown", model.Machine{Provider: "dockur", State: model.StateProvisioning}, false},
		{"failed shown", model.Machine{Provider: "dockur", State: model.StateFailed}, false},
		{"dockur running complete recent hidden by content but shown by recency", model.Machine{
			Provider: "dockur", State: model.StateRunning, ProgressPercent: intPtr(100), CreatedAt: time.Now(),
		}, false}, // within 6h recency window
		{"dockur running complete old hidden", model.Machine{
			Provider: "dockur", State: model.StateRunning, ProgressPercent: intPtr(100), CreatedAt: time.Now().Add(-7 * time.Hour),
		}, true},
		{"dockur running incomplete progress shown", model.Machine{
			Provider: "dockur", State: model.StateRunning, ProgressPercent: intPtr(50), CreatedAt: time.Now().Add(-7 * time.Hour),
		}, false},
		{"dockur running with install message shown despite old+complete", model.Machine{
			Provider: "dockur", State: model.StateRunning, ProgressPercent: intPtr(100), Message: "Finishing install", CreatedAt: time.Now().Add(-7 * time.Hour),
		}, false},
		{"kubevirt running complete hidden", model.Machine{
			Provider: "kubevirt", State: model.StateRunning, ProgressPercent: intPtr(100), CreatedAt: time.Now().Add(-7 * time.Hour),
		}, true},
		{"stopped hidden", model.Machine{Provider: "dockur", State: model.StateStopped, CreatedAt: time.Now().Add(-7 * time.Hour)}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.m.Spec.Name = "vm"
			got := machineJob(tc.m)
			if tc.wantNil && got != nil {
				t.Fatalf("expected nil job, got %+v", got)
			}
			if !tc.wantNil && got == nil {
				t.Fatal("expected a job, got nil")
			}
		})
	}
}

func TestMachineJobStateMapping(t *testing.T) {
	m := model.Machine{Provider: "dockur", State: model.StateFailed, Spec: model.MachineSpec{Name: "vm"}}
	j := machineJob(m)
	if j == nil || j.State != model.JobFailed {
		t.Fatalf("expected JobFailed, got %+v", j)
	}

	m = model.Machine{Provider: "dockur", State: model.StateRunning, ProgressPercent: intPtr(100), Spec: model.MachineSpec{Name: "vm"}, CreatedAt: time.Now()}
	j = machineJob(m)
	if j == nil || j.State != model.JobSucceeded {
		t.Fatalf("expected JobSucceeded, got %+v", j)
	}

	m = model.Machine{Provider: "dockur", State: model.StateProvisioning, Spec: model.MachineSpec{Name: "vm"}}
	j = machineJob(m)
	if j == nil || j.State != model.JobRunning {
		t.Fatalf("expected JobRunning, got %+v", j)
	}
}

func TestGoldenStepIndex(t *testing.T) {
	cases := []struct {
		name string
		b    model.GoldenBuild
		want int
	}{
		{"phase pull", model.GoldenBuild{Phase: "pull"}, 1},
		{"phase prepare", model.GoldenBuild{Phase: "prepare"}, 1},
		{"phase download", model.GoldenBuild{Phase: "download"}, 2},
		{"phase windows_setup", model.GoldenBuild{Phase: "windows_setup"}, 3},
		{"phase sysprep", model.GoldenBuild{Phase: "generalize"}, 4},
		{"phase complete", model.GoldenBuild{Phase: "complete"}, 5},
		{"no phase, state capturing", model.GoldenBuild{State: model.GoldenCapturing}, 5},
		{"no phase, state sysprep", model.GoldenBuild{State: model.GoldenSysprep}, 4},
		{"no phase, state installing", model.GoldenBuild{State: model.GoldenInstalling}, 3},
		{"no phase, state starting falls back", model.GoldenBuild{State: model.GoldenStarting}, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := goldenStepIndex(tc.b); got != tc.want {
				t.Fatalf("got %d, want %d", got, tc.want)
			}
		})
	}
}

func TestGoldenJobActiveAndRecency(t *testing.T) {
	s := &Service{}

	active := model.GoldenBuild{ID: "b1", State: model.GoldenInstalling, ImageID: "windows-11e", UpdatedAt: time.Now().Add(-10 * time.Hour)}
	if j := goldenJob(s, active); j == nil {
		t.Fatal("expected active build to always be shown regardless of age")
	}

	readyRecent := model.GoldenBuild{ID: "b2", State: model.GoldenReady, ImageID: "windows-11e", UpdatedAt: time.Now().Add(-time.Hour)}
	j := goldenJob(s, readyRecent)
	if j == nil {
		t.Fatal("expected recently-ready build to be shown")
	}
	if j.State != model.JobSucceeded {
		t.Fatalf("expected JobSucceeded, got %v", j.State)
	}

	readyOld := model.GoldenBuild{ID: "b3", State: model.GoldenReady, ImageID: "windows-11e", UpdatedAt: time.Now().Add(-5 * time.Hour)}
	if j := goldenJob(s, readyOld); j != nil {
		t.Fatalf("expected old ready build to age out, got %+v", j)
	}

	failedRecent := model.GoldenBuild{ID: "b4", State: model.GoldenFailed, ImageID: "windows-11e", UpdatedAt: time.Now().Add(-time.Hour)}
	j = goldenJob(s, failedRecent)
	if j == nil || j.State != model.JobFailed {
		t.Fatalf("expected JobFailed for recent failed build, got %+v", j)
	}
}

func TestReadGoldenLogs(t *testing.T) {
	m, err := golden.New(golden.Config{BaseDir: t.TempDir()})
	if err != nil {
		t.Fatalf("golden.New: %v", err)
	}
	s := &Service{Golden: m}

	buildDir := filepath.Join(m.BaseDir(), "b1")
	if err := os.MkdirAll(buildDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	logContent := "[INFO] starting\n[OK] pulled image\n[ERR] disk full\n[WARN] retrying\nplain line\n"
	if err := os.WriteFile(filepath.Join(buildDir, "job.log"), []byte(logContent), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}

	lines := readGoldenLogs(s, "b1")
	if len(lines) != 5 {
		t.Fatalf("expected 5 log lines, got %d: %+v", len(lines), lines)
	}
	wantLevels := []string{"info", "ok", "err", "warn", "info"}
	for i, want := range wantLevels {
		if lines[i].Level != want {
			t.Fatalf("line %d: expected level %q, got %q (%+v)", i, want, lines[i].Level, lines[i])
		}
	}
}

func TestReadGoldenLogsMissingReturnsNil(t *testing.T) {
	m, err := golden.New(golden.Config{BaseDir: t.TempDir()})
	if err != nil {
		t.Fatalf("golden.New: %v", err)
	}
	s := &Service{Golden: m}
	if lines := readGoldenLogs(s, "does-not-exist"); lines != nil {
		t.Fatalf("expected nil for missing log, got %+v", lines)
	}
}

func newTestSetupManager(t *testing.T) *storage.SetupManager {
	t.Helper()
	m, err := storage.NewSetupManager(storage.SetupConfig{BaseDir: t.TempDir()})
	if err != nil {
		t.Fatalf("NewSetupManager: %v", err)
	}
	return m
}

func writeSetupStatus(t *testing.T, m *storage.SetupManager, jsonBody string) {
	t.Helper()
	if err := os.WriteFile(m.StatusPath(), []byte(jsonBody), 0o644); err != nil {
		t.Fatalf("write status: %v", err)
	}
}

func TestStorageSetupJobIdle(t *testing.T) {
	m := newTestSetupManager(t)
	// No status.json written yet: Get() returns the idle default.
	if j := storageSetupJob(m); j != nil {
		t.Fatalf("expected nil job for idle setup, got %+v", j)
	}
}

func TestStorageSetupJobRunningAlwaysShown(t *testing.T) {
	m := newTestSetupManager(t)
	old := time.Now().Add(-10 * time.Hour).UTC().Format(time.RFC3339Nano)
	writeSetupStatus(t, m, `{"id":"current","backend":"longhorn","state":"running","message":"installing","updatedAt":"`+old+`"}`)
	j := storageSetupJob(m)
	if j == nil {
		t.Fatal("expected running setup to always be shown regardless of age")
	}
	if j.State != model.JobRunning || j.ProgressPercent != 50 {
		t.Fatalf("unexpected job %+v", j)
	}
}

func TestStorageSetupJobSucceededRecent(t *testing.T) {
	m := newTestSetupManager(t)
	recent := time.Now().Add(-30 * time.Minute).UTC().Format(time.RFC3339Nano)
	writeSetupStatus(t, m, `{"id":"current","backend":"rook-ceph","state":"succeeded","updatedAt":"`+recent+`"}`)
	j := storageSetupJob(m)
	if j == nil || j.State != model.JobSucceeded || j.ProgressPercent != 100 {
		t.Fatalf("unexpected job %+v", j)
	}
}

func TestStorageSetupJobSucceededAgesOut(t *testing.T) {
	m := newTestSetupManager(t)
	old := time.Now().Add(-3 * time.Hour).UTC().Format(time.RFC3339Nano)
	writeSetupStatus(t, m, `{"id":"current","backend":"rook-ceph","state":"succeeded","updatedAt":"`+old+`"}`)
	if j := storageSetupJob(m); j != nil {
		t.Fatalf("expected succeeded setup older than 2h to age out, got %+v", j)
	}
}

func TestStorageSetupJobFailedRecent(t *testing.T) {
	m := newTestSetupManager(t)
	recent := time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano)
	writeSetupStatus(t, m, `{"id":"current","backend":"longhorn","state":"failed","message":"boom","updatedAt":"`+recent+`"}`)
	j := storageSetupJob(m)
	if j == nil || j.State != model.JobFailed || j.ProgressPercent != 0 {
		t.Fatalf("unexpected job %+v", j)
	}
}

func TestSortJobsOrdersNewestFirst(t *testing.T) {
	now := time.Now()
	items := []model.Job{
		{ID: "a", UpdatedAt: now.Add(-2 * time.Hour)},
		{ID: "b", UpdatedAt: now},
		{ID: "c", UpdatedAt: now.Add(-1 * time.Hour)},
	}
	sortJobs(items)
	got := []string{items[0].ID, items[1].ID, items[2].ID}
	want := []string{"b", "c", "a"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got order %v, want %v", got, want)
		}
	}
}
