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

package reconciler

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/zyvorai/kryton/internal/events"
	"github.com/zyvorai/kryton/internal/model"
	"github.com/zyvorai/kryton/internal/provider"
)

// fakeProvider is a minimal provider.Provider stand-in: only List and
// Delete carry real behavior since that's all TTL uses; every other
// method panics if reached, so a test that hits one fails loudly.
type fakeProvider struct {
	mu        sync.Mutex
	machines  map[string][]model.Machine // by project
	listErr   map[string]error           // by project
	deleteErr error
	deleted   []string // "project/id"
	listCalls int
}

func (f *fakeProvider) Name() string                 { return "fake" }
func (f *fakeProvider) Health(context.Context) error { return nil }
func (f *fakeProvider) Capabilities(context.Context) (model.Capabilities, error) {
	panic("not used by TTL")
}
func (f *fakeProvider) Create(context.Context, string, model.MachineSpec) (*model.Machine, error) {
	panic("not used by TTL")
}
func (f *fakeProvider) Get(context.Context, string, string) (*model.Machine, error) {
	panic("not used by TTL")
}

func (f *fakeProvider) List(_ context.Context, project string) ([]model.Machine, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listCalls++
	if err := f.listErr[project]; err != nil {
		return nil, err
	}
	out := make([]model.Machine, len(f.machines[project]))
	copy(out, f.machines[project])
	return out, nil
}

func (f *fakeProvider) Start(context.Context, string, string) (*model.Machine, error) {
	panic("not used by TTL")
}
func (f *fakeProvider) Stop(context.Context, string, string) (*model.Machine, error) {
	panic("not used by TTL")
}

func (f *fakeProvider) Delete(_ context.Context, project, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.deleted = append(f.deleted, project+"/"+id)
	return nil
}

func (f *fakeProvider) Snapshot(context.Context, string, string, string) (*model.Snapshot, error) {
	panic("not used by TTL")
}
func (f *fakeProvider) ListSnapshots(context.Context, string, string) ([]model.Snapshot, error) {
	panic("not used by TTL")
}
func (f *fakeProvider) RestoreSnapshot(context.Context, string, string, string) (*model.Snapshot, error) {
	panic("not used by TTL")
}
func (f *fakeProvider) DeleteSnapshot(context.Context, string, string, string) error {
	panic("not used by TTL")
}

var _ provider.Provider = (*fakeProvider)(nil)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func expiresAt(d time.Duration) *time.Time {
	t := time.Now().UTC().Add(d)
	return &t
}

func TestRunOnceDeletesExpiredMachines(t *testing.T) {
	fp := &fakeProvider{
		machines: map[string][]model.Machine{
			"default": {
				{ID: "past", ExpiresAt: expiresAt(-time.Hour), Spec: model.MachineSpec{Name: "past-vm"}},
				{ID: "future", ExpiresAt: expiresAt(time.Hour), Spec: model.MachineSpec{Name: "future-vm"}},
				{ID: "no-ttl", ExpiresAt: nil, Spec: model.MachineSpec{Name: "no-ttl-vm"}},
			},
		},
	}
	bus, err := events.New(10, "", "", "", discardLogger())
	if err != nil {
		t.Fatalf("events.New: %v", err)
	}
	sub, cancel := bus.Subscribe()
	defer cancel()

	r := TTL{Provider: fp, Projects: []string{"default"}, Events: bus, Log: discardLogger()}
	r.runOnce(context.Background())

	if len(fp.deleted) != 1 || fp.deleted[0] != "default/past" {
		t.Fatalf("expected only default/past deleted, got %v", fp.deleted)
	}

	select {
	case e := <-sub:
		if e.Type != "io.kryton.machine.expired" {
			t.Fatalf("unexpected event type %q", e.Type)
		}
		if e.Data["machineId"] != "past" {
			t.Fatalf("unexpected event data %v", e.Data)
		}
	default:
		t.Fatal("expected an expiry event to be published")
	}
}

func TestRunOnceSkipsUnexpiredAndUnset(t *testing.T) {
	fp := &fakeProvider{
		machines: map[string][]model.Machine{
			"default": {
				{ID: "future", ExpiresAt: expiresAt(time.Hour)},
				{ID: "no-ttl", ExpiresAt: nil},
			},
		},
	}
	bus, err := events.New(10, "", "", "", discardLogger())
	if err != nil {
		t.Fatalf("events.New: %v", err)
	}

	r := TTL{Provider: fp, Projects: []string{"default"}, Events: bus, Log: discardLogger()}
	r.runOnce(context.Background())

	if len(fp.deleted) != 0 {
		t.Fatalf("expected nothing deleted, got %v", fp.deleted)
	}
	if len(bus.List(0)) != 0 {
		t.Fatalf("expected no events published, got %d", len(bus.List(0)))
	}
}

func TestRunOnceListErrorSkipsProjectWithoutPanicking(t *testing.T) {
	fp := &fakeProvider{
		machines: map[string][]model.Machine{
			"good": {{ID: "past", ExpiresAt: expiresAt(-time.Hour)}},
		},
		listErr: map[string]error{"bad": errors.New("list boom")},
	}
	bus, err := events.New(10, "", "", "", discardLogger())
	if err != nil {
		t.Fatalf("events.New: %v", err)
	}

	r := TTL{Provider: fp, Projects: []string{"bad", "good"}, Events: bus, Log: discardLogger()}
	r.runOnce(context.Background())

	if len(fp.deleted) != 1 || fp.deleted[0] != "good/past" {
		t.Fatalf("expected good/past deleted despite bad project's List error, got %v", fp.deleted)
	}
}

func TestRunOnceDeleteErrorDoesNotPublishEvent(t *testing.T) {
	fp := &fakeProvider{
		machines: map[string][]model.Machine{
			"default": {{ID: "past", ExpiresAt: expiresAt(-time.Hour)}},
		},
		deleteErr: errors.New("delete boom"),
	}
	bus, err := events.New(10, "", "", "", discardLogger())
	if err != nil {
		t.Fatalf("events.New: %v", err)
	}

	r := TTL{Provider: fp, Projects: []string{"default"}, Events: bus, Log: discardLogger()}
	r.runOnce(context.Background())

	if len(fp.deleted) != 0 {
		t.Fatalf("expected no successful deletes, got %v", fp.deleted)
	}
	if len(bus.List(0)) != 0 {
		t.Fatalf("expected no expiry event when Delete fails, got %d", len(bus.List(0)))
	}
}

func TestRunTicksUntilContextCanceled(t *testing.T) {
	fp := &fakeProvider{machines: map[string][]model.Machine{"default": nil}}
	bus, err := events.New(10, "", "", "", discardLogger())
	if err != nil {
		t.Fatalf("events.New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()

	r := TTL{Provider: fp, Projects: []string{"default"}, Events: bus, Log: discardLogger(), Interval: 20 * time.Millisecond}

	done := make(chan struct{})
	go func() {
		r.Run(ctx)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after context cancellation")
	}

	fp.mu.Lock()
	calls := fp.listCalls
	fp.mu.Unlock()
	if calls < 2 {
		t.Fatalf("expected at least an immediate sweep plus one tick, got %d List calls", calls)
	}
}
