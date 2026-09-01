package model

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

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

type ComputeSpec struct {
	CPU       int `json:"cpu"`
	MemoryMiB int `json:"memoryMiB"`
}

type DiskSpec struct {
	SizeGiB int `json:"sizeGiB"`
}

type NetworkSpec struct {
	NetworkID string `json:"networkId,omitempty"`
}

type MachineSpec struct {
	Name       string            `json:"name"`
	Image      string            `json:"image"`
	Compute    ComputeSpec       `json:"compute"`
	Disk       DiskSpec          `json:"disk"`
	Network    NetworkSpec       `json:"network,omitempty"`
	TTLMinutes int               `json:"ttlMinutes,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
}

type ProviderRef struct {
	Provider  string `json:"provider"`
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name"`
}

type Condition struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

type Machine struct {
	ID              string       `json:"id"`
	Project         string       `json:"project"`
	Provider        string       `json:"provider"`
	State           MachineState `json:"state"`
	Spec            MachineSpec  `json:"spec"`
	ProviderRef     ProviderRef  `json:"providerRef"`
	IPAddresses     []string     `json:"ipAddresses,omitempty"`
	ConsoleURL      string       `json:"consoleUrl,omitempty"`
	RdpHost         string       `json:"rdpHost,omitempty"`
	RdpPort         int          `json:"rdpPort,omitempty"`
	ProgressPercent *int         `json:"progressPercent,omitempty"`
	Message         string       `json:"message,omitempty"`
	Conditions      []Condition  `json:"conditions,omitempty"`
	CreatedAt       time.Time    `json:"createdAt"`
	UpdatedAt       time.Time    `json:"updatedAt"`
	ExpiresAt       *time.Time   `json:"expiresAt,omitempty"`
}

type Snapshot struct {
	ID        string    `json:"id"`
	Project   string    `json:"project"`
	MachineID string    `json:"machineId"`
	Name      string    `json:"name"`
	State     string    `json:"state"`
	CreatedAt time.Time `json:"createdAt"`
}

type Image struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Version       string   `json:"version"`
	Family        string   `json:"family"`
	Description   string   `json:"description"`
	MinCPU        int      `json:"minCpu"`
	MinMemoryMiB  int      `json:"minMemoryMiB"`
	DefaultDiskGB int      `json:"defaultDiskGiB"`
	DockurVersion      string   `json:"dockurVersion,omitempty"`
	Tags               []string `json:"tags,omitempty"`
	Availability       string   `json:"availability,omitempty"` // stored | on-demand | catalog
	StorageSource      string   `json:"storageSource,omitempty"` // cdi | dockur | golden | demo
	StorageNamespace   string   `json:"storageNamespace,omitempty"`
	StoragePath        string   `json:"storagePath,omitempty"`
	Ready              bool     `json:"ready"`
}

type Capabilities struct {
	Provider      string `json:"provider"`
	Snapshots     bool   `json:"snapshots"`
	Networks      bool   `json:"networks"`
	TTL           bool   `json:"ttl"`
	LiveMigration bool   `json:"liveMigration"`
	Console       bool   `json:"console"`
	GoldenImages  bool   `json:"goldenImages"`
}

type DoctorFinding struct {
	Check   string `json:"check"`
	Status  string `json:"status"` // pass | warn | fail
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

type DoctorReport struct {
	Provider string          `json:"provider"`
	Healthy  bool            `json:"healthy"`
	Findings []DoctorFinding `json:"findings"`
}

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

func ValidateProject(p string) error {
	if len(p) < 1 || len(p) > 63 || !projectLabel.MatchString(p) {
		return errors.New("project must be a DNS-style label up to 63 characters")
	}
	return nil
}

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
	if len(problems) > 0 {
		sort.Strings(problems)
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}
