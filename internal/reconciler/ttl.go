// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

// Package reconciler runs krytond's background loops. TTL periodically
// lists machines across all configured projects through a
// provider.Provider, deletes those past their expiry, and emits an event
// on the shared events.Bus for each expiry.
package reconciler

import (
	"context"
	"log/slog"
	"time"

	"github.com/zyvorai/kryton/internal/events"
	"github.com/zyvorai/kryton/internal/provider"
)

// TTL periodically expires machines whose Spec.TTLMinutes deadline has
// passed. Zero-value Interval defaults to 30s in Run.
type TTL struct {
	Provider provider.Provider
	Projects []string
	Events   *events.Bus
	Log      *slog.Logger
	Interval time.Duration
}

// Run sweeps every configured project once immediately, then on every
// tick of Interval, until ctx is canceled. Intended to run for the
// lifetime of the krytond process in its own goroutine; blocks until
// ctx.Done().
func (r TTL) Run(ctx context.Context) {
	if r.Interval <= 0 {
		r.Interval = 30 * time.Second
	}
	ticker := time.NewTicker(r.Interval)
	defer ticker.Stop()
	r.runOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.runOnce(ctx)
		}
	}
}

func (r TTL) runOnce(ctx context.Context) {
	now := time.Now().UTC()
	for _, project := range r.Projects {
		machines, err := r.Provider.List(ctx, project)
		if err != nil {
			r.Log.Warn("ttl reconcile list failed", "project", project, "error", err)
			continue
		}
		for _, m := range machines {
			if m.ExpiresAt == nil || now.Before(*m.ExpiresAt) {
				continue
			}
			if err := r.Provider.Delete(ctx, project, m.ID); err != nil {
				r.Log.Warn("ttl delete failed", "project", project, "machine", m.ID, "error", err)
				continue
			}
			r.Events.Publish(ctx, "io.kryton.machine.expired", "machines/"+m.ID, map[string]any{"project": project, "machineId": m.ID, "name": m.Spec.Name})
		}
	}
}
