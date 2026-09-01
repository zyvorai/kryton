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

// Package model defines the domain types shared by the API, CLI, and all
// provider backends: machine lifecycle state, compute/disk/network specs,
// dockur-specific options, capabilities, snapshots, images, jobs, and
// doctor reports — the common vocabulary that keeps providers
// interchangeable.
package model

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// MachineState is the provider-neutral lifecycle state of a Machine.
// Not every provider reaches every state (e.g. only kubevirt uses
// StateMigrating and StatePaused).
type MachineState string

const (
	StatePending      MachineState = "pending"
	StateProvisioning MachineState = "provisioning"
	StateStarting     MachineState = "starting"
	StateRunning      MachineState = "running"
	StateStopping     MachineState = "stopping"
	StateStopped      MachineState = "stopped"
	StateMigrating    MachineState = "migrating"
	StatePaused       MachineState = "paused"
	StateDeleting     MachineState = "deleting"
	StateFailed       MachineState = "failed"
	StateUnknown      MachineState = "unknown"
)

// ComputeSpec is the requested CPU/memory shape of a machine.
type ComputeSpec struct {
	CPU       int `json:"cpu"`
	MemoryMiB int `json:"memoryMiB"`
}

// DiskSpec is the requested primary disk for a machine.
type DiskSpec struct {
	SizeGiB int `json:"sizeGiB"`
	// StorageClass overrides the provider/cluster default StorageClass
	// (kubevirt only; ignored by demo and dockur).
	StorageClass string `json:"storageClass,omitempty"`
}

// NetworkSpec selects a provider-specific network attachment.
type NetworkSpec struct {
	NetworkID string `json:"networkId,omitempty"`
}

// DockurOptions maps to dockur/windows compose environment and volumes.
// Password is accepted on create and written into compose; API responses redact it.
type DockurOptions struct {
	Username      string `json:"username,omitempty"`
	Password      string `json:"password,omitempty"`
	Hostname      string `json:"hostname,omitempty"`
	Language      string `json:"language,omitempty"`
	Region        string `json:"region,omitempty"`
	Keyboard      string `json:"keyboard,omitempty"`
	ProductKey    string `json:"productKey,omitempty"`
	Domain        string `json:"domain,omitempty"`
	DomainOU      string `json:"domainOu,omitempty"`
	Autologin     *bool  `json:"autologin,omitempty"`
	Audio         bool   `json:"audio,omitempty"`
	SecureBoot    bool   `json:"secureBoot,omitempty"`
	SharedDir     string `json:"sharedDir,omitempty"`
	OemDir        string `json:"oemDir,omitempty"`
	Command       string `json:"command,omitempty"`
	CustomISO     string `json:"customIso,omitempty"`
	Edition       string `json:"edition,omitempty"`
	ExtraDisksGiB []int  `json:"extraDisksGiB,omitempty"`
}

// MachineSpec is the caller-supplied desired state for Provider.Create.
// It is echoed back on Machine.Spec so the original request survives
// provider translation (e.g. into a KubeVirt VirtualMachine).
type MachineSpec struct {
	Name    string      `json:"name"`
	Image   string      `json:"image"`
	Compute ComputeSpec `json:"compute"`
	Disk    DiskSpec    `json:"disk"`
	Network NetworkSpec `json:"network,omitempty"`
	// TTLMinutes schedules automatic deletion by internal/reconciler; 0 means no expiry.
	TTLMinutes int               `json:"ttlMinutes,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
	// Dockur carries options only the dockur provider consumes; nil for demo/kubevirt.
	Dockur *DockurOptions `json:"dockur,omitempty"`
}

// ProviderRef records where a Machine actually lives in the backing
// system, so operators can cross-reference it (e.g. `kubectl -n
// Namespace get vm Name`) without Kryton exposing that as the primary ID.
type ProviderRef struct {
	Provider  string `json:"provider"`
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name"`
}

// Condition is a Kubernetes-style status condition surfaced from the
// backing provider (kubevirt VirtualMachine/VMI conditions today).
type Condition struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

// Machine is the provider-neutral view of a Windows workload returned by
// every Provider method. ID is a stable UUID independent of ProviderRef,
// so callers never need to know the backend's own naming.
type Machine struct {
	ID          string       `json:"id"`
	Project     string       `json:"project"`
	Provider    string       `json:"provider"`
	State       MachineState `json:"state"`
	Spec        MachineSpec  `json:"spec"`
	ProviderRef ProviderRef  `json:"providerRef"`
	IPAddresses []string     `json:"ipAddresses,omitempty"`
	ConsoleURL  string       `json:"consoleUrl,omitempty"`
	RdpHost     string       `json:"rdpHost,omitempty"`
	RdpPort     int          `json:"rdpPort,omitempty"`
	RdpUsername string       `json:"rdpUsername,omitempty"`
	// ProgressPercent and Message are populated during install/provisioning
	// by providers that support it (dockur, demo); nil/empty once running.
	ProgressPercent *int        `json:"progressPercent,omitempty"`
	Message         string      `json:"message,omitempty"`
	Conditions      []Condition `json:"conditions,omitempty"`
	CreatedAt       time.Time   `json:"createdAt"`
	UpdatedAt       time.Time   `json:"updatedAt"`
	// ExpiresAt is derived from Spec.TTLMinutes; nil means the machine never expires.
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
}

// Snapshot is a point-in-time disk capture of a Machine.
type Snapshot struct {
	ID        string    `json:"id"`
	Project   string    `json:"project"`
	MachineID string    `json:"machineId"`
	Name      string    `json:"name"`
	State     string    `json:"state"`
	Message   string    `json:"message,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

// Image describes one entry in the deployable Windows image catalog
// (internal/catalog), optionally enriched with live storage state by
// internal/images.
type Image struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Version       string `json:"version"`
	Family        string `json:"family"`
	Description   string `json:"description"`
	MinCPU        int    `json:"minCpu"`
	MinMemoryMiB  int    `json:"minMemoryMiB"`
	DefaultDiskGB int    `json:"defaultDiskGiB"`
	// DockurVersion is the dockur/windows VERSION env value this image maps to; empty for kubevirt-only images.
	DockurVersion    string   `json:"dockurVersion,omitempty"`
	Tags             []string `json:"tags,omitempty"`
	Availability     string   `json:"availability,omitempty"`  // stored | on-demand | catalog
	StorageSource    string   `json:"storageSource,omitempty"` // cdi | dockur | golden | demo
	StorageNamespace string   `json:"storageNamespace,omitempty"`
	StoragePath      string   `json:"storagePath,omitempty"`
	// Ready is true once the underlying disk/DataSource is actually usable for Create.
	Ready bool `json:"ready"`
}

// Capabilities advertises which optional features a Provider supports,
// so the UI/CLI can hide controls (e.g. Snapshot) the active backend can't do.
type Capabilities struct {
	Provider      string `json:"provider"`
	Snapshots     bool   `json:"snapshots"`
	Networks      bool   `json:"networks"`
	TTL           bool   `json:"ttl"`
	LiveMigration bool   `json:"liveMigration"`
	Console       bool   `json:"console"`
	GoldenImages  bool   `json:"goldenImages"`
}

// DoctorFinding is one check result from internal/doctor.
type DoctorFinding struct {
	Check   string `json:"check"`
	Status  string `json:"status"` // pass | warn | fail
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

// DoctorReport is the aggregate result of `krytonctl doctor` / GET /api/v1/doctor.
type DoctorReport struct {
	Provider string          `json:"provider"`
	Healthy  bool            `json:"healthy"`
	Findings []DoctorFinding `json:"findings"`
}

// Summary is the per-project machine/resource rollup shown on the UI Overview page.
type Summary struct {
	Project   string `json:"project"`
	Provider  string `json:"provider"`
	Machines  int    `json:"machines"`
	Running   int    `json:"running"`
	Stopped   int    `json:"stopped"`
	Attention int    `json:"attention"`
	CPU       int    `json:"cpu"`
	MemoryMiB int    `json:"memoryMiB"`
}

var dnsLabel = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
var projectLabel = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
var labelKey = regexp.MustCompile(`^[A-Za-z0-9_.:/-]{1,128}$`)
var labelValue = regexp.MustCompile(`^[A-Za-z0-9_.:/ -]{0,256}$`)

// ValidateProject reports whether p is a valid Kryton project name
// (DNS-style label, 1-63 chars); providers use this as their
// namespace/tenant key.
func ValidateProject(p string) error {
	if len(p) < 1 || len(p) > 63 || !projectLabel.MatchString(p) {
		return errors.New("project must be a DNS-style label up to 63 characters")
	}
	return nil
}

// ValidateMachineSpec checks a MachineSpec against Kryton's size/format
// limits before it reaches a Provider, returning every violation found
// joined into a single error rather than failing on the first one.
func ValidateMachineSpec(s MachineSpec) error {
	var problems []string
	if len(s.Name) < 1 || len(s.Name) > 63 || !dnsLabel.MatchString(s.Name) {
		problems = append(problems, "name must be a DNS-style label up to 63 characters")
	}
	if len(s.Image) < 1 || len(s.Image) > 128 || !labelKey.MatchString(s.Image) {
		problems = append(problems, "image is invalid")
	}
	if s.Compute.CPU < 1 || s.Compute.CPU > 256 {
		problems = append(problems, "compute.cpu must be between 1 and 256")
	}
	if s.Compute.MemoryMiB < 512 || s.Compute.MemoryMiB > 2*1024*1024 {
		problems = append(problems, "compute.memoryMiB must be between 512 and 2097152")
	}
	if s.Disk.SizeGiB < 16 || s.Disk.SizeGiB > 65536 {
		problems = append(problems, "disk.sizeGiB must be between 16 and 65536")
	}
	if len(s.Network.NetworkID) > 253 {
		problems = append(problems, "network.networkId is too long")
	}
	if s.TTLMinutes < 0 || s.TTLMinutes > 60*24*30 {
		problems = append(problems, "ttlMinutes must be between 0 and 43200")
	}
	for k, v := range s.Labels {
		if !labelKey.MatchString(k) || !labelValue.MatchString(v) {
			problems = append(problems, fmt.Sprintf("invalid label %q", k))
		}
	}
	if d := s.Dockur; d != nil {
		if len(d.Username) > 64 {
			problems = append(problems, "dockur.username is too long")
		}
		if len(d.Password) > 128 {
			problems = append(problems, "dockur.password is too long")
		}
		if len(d.Hostname) > 63 {
			problems = append(problems, "dockur.hostname is too long")
		}
		if len(d.Language) > 64 || len(d.Region) > 32 || len(d.Keyboard) > 32 {
			problems = append(problems, "dockur locale fields are too long")
		}
		if len(d.ProductKey) > 64 {
			problems = append(problems, "dockur.productKey is too long")
		}
		if len(d.Domain) > 253 || len(d.DomainOU) > 512 {
			problems = append(problems, "dockur domain fields are too long")
		}
		if len(d.SharedDir) > 1024 || len(d.OemDir) > 1024 || len(d.CustomISO) > 2048 {
			problems = append(problems, "dockur path/url fields are too long")
		}
		if len(d.Command) > 4096 {
			problems = append(problems, "dockur.command is too long")
		}
		if len(d.ExtraDisksGiB) > 3 {
			problems = append(problems, "dockur.extraDisksGiB supports at most 3 disks")
		}
		for _, g := range d.ExtraDisksGiB {
			if g < 1 || g > 65536 {
				problems = append(problems, "dockur.extraDisksGiB entries must be between 1 and 65536")
				break
			}
		}
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}
