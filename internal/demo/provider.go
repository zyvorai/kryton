package demo

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/zyvorai/kryton/internal/id"
	"github.com/zyvorai/kryton/internal/model"
	"github.com/zyvorai/kryton/internal/provider"
)

type Provider struct {
	mu       sync.RWMutex
	machines map[string]map[string]model.Machine
	nextIP   int
}

func New() *Provider {
	return &Provider{machines: map[string]map[string]model.Machine{}, nextIP: 20}
}

func (p *Provider) Name() string                 { return "demo" }
func (p *Provider) Health(context.Context) error { return nil }
func (p *Provider) Capabilities(context.Context) (model.Capabilities, error) {
	return model.Capabilities{Provider: p.Name(), Snapshots: true, Networks: true, TTL: true}, nil
}

func (p *Provider) Create(_ context.Context, project string, spec model.MachineSpec) (*model.Machine, error) {
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
	now := time.Now().UTC()
	machineID := id.New()
	m := model.Machine{
		ID: machineID, Project: project, Provider: p.Name(), State: model.StateRunning, Spec: spec,
		ProviderRef: model.ProviderRef{Provider: p.Name(), Namespace: project, Name: spec.Name},
		IPAddresses: []string{fmt.Sprintf("10.44.0.%d", p.nextIP)}, CreatedAt: now, UpdatedAt: now,
	}
	p.nextIP++
	if spec.TTLMinutes > 0 {
		expires := now.Add(time.Duration(spec.TTLMinutes) * time.Minute)
		m.ExpiresAt = &expires
	}
	p.machines[project][machineID] = m
	return clone(m), nil
}

func (p *Provider) Get(_ context.Context, project, machineID string) (*model.Machine, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	m, ok := p.machines[project][machineID]
	if !ok {
		return nil, provider.ErrNotFound
	}
	return clone(m), nil
}

func (p *Provider) List(_ context.Context, project string) ([]model.Machine, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	var out []model.Machine
	for _, m := range p.machines[project] {
		out = append(out, *clone(m))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (p *Provider) Start(ctx context.Context, project, machineID string) (*model.Machine, error) {
	return p.setState(project, machineID, model.StateRunning)
}
func (p *Provider) Stop(ctx context.Context, project, machineID string) (*model.Machine, error) {
	return p.setState(project, machineID, model.StateStopped)
}
func (p *Provider) setState(project, machineID string, state model.MachineState) (*model.Machine, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	m, ok := p.machines[project][machineID]
	if !ok {
		return nil, provider.ErrNotFound
	}
	m.State, m.UpdatedAt = state, time.Now().UTC()
	p.machines[project][machineID] = m
	return clone(m), nil
}

func (p *Provider) Delete(_ context.Context, project, machineID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.machines[project][machineID]; !ok {
		return provider.ErrNotFound
	}
	delete(p.machines[project], machineID)
	return nil
}

func (p *Provider) Snapshot(_ context.Context, project, machineID, name string) (*model.Snapshot, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if _, ok := p.machines[project][machineID]; !ok {
		return nil, provider.ErrNotFound
	}
	return &model.Snapshot{ID: id.New(), Project: project, MachineID: machineID, Name: name, State: "ready", CreatedAt: time.Now().UTC()}, nil
}

func clone(m model.Machine) *model.Machine {
	c := m
	c.IPAddresses = append([]string(nil), m.IPAddresses...)
	c.Conditions = append([]model.Condition(nil), m.Conditions...)
	if m.Spec.Labels != nil {
		c.Spec.Labels = map[string]string{}
		for k, v := range m.Spec.Labels {
			c.Spec.Labels[k] = v
		}
	}
	return &c
}
