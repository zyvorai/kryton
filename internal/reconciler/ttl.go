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

type TTL struct {
	Provider provider.Provider
	Projects []string
	Events   *events.Bus
	Log      *slog.Logger
	Interval time.Duration
}

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
