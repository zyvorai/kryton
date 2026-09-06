// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

// Package provider defines the backend-agnostic contract that the demo,
// dockur, and kubevirt implementations satisfy, so the API layer, CLI,
// doctor, and reconciler never depend on a specific virtualization
// backend — only on this interface.
package provider

import (
	"context"
	"errors"

	"github.com/zyvorai/kryton/internal/model"
)

// Sentinel errors returned by Provider implementations; the API layer
// maps these to HTTP status codes regardless of backend.
var (
	ErrNotFound    = errors.New("resource not found")
	ErrConflict    = errors.New("resource already exists")
	ErrUnsupported = errors.New("operation unsupported")
)

// Provider is the machine lifecycle contract every backend (demo, dockur,
// kubevirt) implements. Every method except Name and Health takes a
// project as its first (or only) string argument, scoping the operation
// to that project's namespace/tenant; a second string argument, where
// present, is the machine ID.
type Provider interface {
	// Name identifies the provider (e.g. "demo", "dockur", "kubevirt").
	Name() string
	// Health reports whether the backend is reachable and usable.
	Health(context.Context) error
	// Capabilities describes which optional features (snapshots,
	// networks, TTL, console) this backend supports.
	Capabilities(context.Context) (model.Capabilities, error)
	// Create provisions a new machine in project from spec.
	Create(ctx context.Context, project string, spec model.MachineSpec) (*model.Machine, error)
	// Get fetches one machine by ID within project.
	Get(ctx context.Context, project, id string) (*model.Machine, error)
	// List returns all machines in project.
	List(ctx context.Context, project string) ([]model.Machine, error)
	// Start powers on a stopped machine.
	Start(ctx context.Context, project, id string) (*model.Machine, error)
	// Stop powers off a running machine.
	Stop(ctx context.Context, project, id string) (*model.Machine, error)
	// Delete permanently removes a machine and its backing resources.
	Delete(ctx context.Context, project, id string) error
	// Snapshot captures the current disk state of a machine under name.
	Snapshot(ctx context.Context, project, id, name string) (*model.Snapshot, error)
	// ListSnapshots returns all snapshots for a machine.
	ListSnapshots(ctx context.Context, project, id string) ([]model.Snapshot, error)
	// RestoreSnapshot reverts a machine's disk to a prior snapshot.
	RestoreSnapshot(ctx context.Context, project, id, snapshotID string) (*model.Snapshot, error)
	// DeleteSnapshot removes a snapshot without affecting the machine.
	DeleteSnapshot(ctx context.Context, project, id, snapshotID string) error
}
