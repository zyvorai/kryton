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

package storage

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const setupID = "current"

// SetupRequest starts cluster storage installation from the Settings UI.
type SetupRequest struct {
	Backend    string `json:"backend"`            // longhorn | rook-ceph
	RookMode   string `json:"rookMode,omitempty"` // lab | devices | pool-only | device
	Device     string `json:"device,omitempty"`   // e.g. /dev/sdb1 when rookMode=device
	WipeDevice bool   `json:"wipeDevice,omitempty"`
	SetDefault bool   `json:"setDefault,omitempty"`
}

// SetupState tracks an in-flight or recent storage install job.
type SetupState struct {
	ID           string    `json:"id"`
	Backend      string    `json:"backend"`
	RookMode     string    `json:"rookMode,omitempty"`
	Device       string    `json:"device,omitempty"`
	State        string    `json:"state"` // idle | running | succeeded | failed
	Message      string    `json:"message,omitempty"`
	Error        string    `json:"error,omitempty"`
	StorageClass string    `json:"storageClass,omitempty"`
	StartedAt    time.Time `json:"startedAt,omitempty"`
	UpdatedAt    time.Time `json:"updatedAt,omitempty"`
}

// BlockDevice is a host disk hint for Rook OSD selection.
type BlockDevice struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Size        string `json:"size,omitempty"`
	Model       string `json:"model,omitempty"`
	Mountpoint  string `json:"mountpoint,omitempty"`
	Recommended bool   `json:"recommended,omitempty"`
	Blocked     bool   `json:"blocked,omitempty"`
	BlockReason string `json:"blockReason,omitempty"`
}

// SetupManager runs enable-* scripts on the krytond host.
type SetupManager struct {
	mu              sync.Mutex
	running         bool
	baseDir         string
	snapshotsScript string
	rookScript      string
	onComplete      func(storageClass string, setDefault bool)
}

// SetupConfig configures NewSetupManager; OnComplete, if set, is called
// after a successful Start run with the StorageClass it installed and
// whether the caller asked it to become the Kryton default.
type SetupConfig struct {
	BaseDir         string
	SnapshotsScript string
	RookScript      string
	OnComplete      func(storageClass string, setDefault bool)
}

// NewSetupManager builds a SetupManager, creating cfg.BaseDir if needed
// (defaulting to ~/.kryton/storage-setup).
func NewSetupManager(cfg SetupConfig) (*SetupManager, error) {
	base := strings.TrimSpace(cfg.BaseDir)
	if base == "" {
		home, _ := os.UserHomeDir()
		if home == "" {
			home = os.TempDir()
		}
		base = filepath.Join(home, ".kryton", "storage-setup")
	}
	if err := os.MkdirAll(base, 0o755); err != nil {
		return nil, err
	}
	return &SetupManager{
		baseDir:         base,
		snapshotsScript: cfg.SnapshotsScript,
		rookScript:      cfg.RookScript,
		onComplete:      cfg.OnComplete,
	}, nil
}

// SnapshotsScript returns the configured enable-kubevirt-snapshots.sh
// path; safe to call on a nil *SetupManager (returns "").
func (m *SetupManager) SnapshotsScript() string {
	if m == nil {
		return ""
	}
	return m.snapshotsScript
}

// Available reports whether m is non-nil, its snapshots script is
// configured and present, and kubectl is on PATH — i.e. whether Start
// can actually run on this host.
func (m *SetupManager) Available() bool {
	if m == nil {
		return false
	}
	if m.snapshotsScript == "" {
		return false
	}
	if _, err := os.Stat(m.snapshotsScript); err != nil {
		return false
	}
	if _, err := exec.LookPath("kubectl"); err != nil {
		return false
	}
	return true
}

// StatusPath is the JSON file Start's background run reports progress to.
func (m *SetupManager) StatusPath() string { return filepath.Join(m.baseDir, "status.json") }

// LogPath is the file the setup script's combined stdout/stderr is appended to.
func (m *SetupManager) LogPath() string { return filepath.Join(m.baseDir, "job.log") }

// Get returns the current setup status, or an idle SetupState if none has run yet.
func (m *SetupManager) Get() (*SetupState, error) {
	if m == nil {
		return &SetupState{ID: setupID, State: "idle"}, nil
	}
	b, err := os.ReadFile(m.StatusPath())
	if err != nil {
		if os.IsNotExist(err) {
			return &SetupState{ID: setupID, State: "idle"}, nil
		}
		return nil, err
	}
	var st SetupState
	if err := json.Unmarshal(b, &st); err != nil {
		return nil, err
	}
	if st.ID == "" {
		st.ID = setupID
	}
	return &st, nil
}

// Logs returns up to the last limit lines from LogPath, oldest of that
// window first; nil if there's no log yet or limit <= 0.
func (m *SetupManager) Logs(limit int) []string {
	if m == nil || limit <= 0 {
		return nil
	}
	f, err := os.Open(m.LogPath())
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}
	return lines
}

// Start validates req and, if no setup is already running, launches the
// matching enable-* script in the background, returning immediately with
// the initial "running" SetupState — poll Get for progress. Returns an
// error without starting anything if req is invalid, a setup is already
// running, or scripts aren't Available.
func (m *SetupManager) Start(req SetupRequest) (*SetupState, error) {
	if m == nil || !m.Available() {
		return nil, fmt.Errorf("storage setup scripts are not available on this host")
	}
	req.Backend = strings.TrimSpace(req.Backend)
	req.RookMode = strings.TrimSpace(req.RookMode)
	req.Device = strings.TrimSpace(req.Device)

	if err := validateSetupRequest(req); err != nil {
		return nil, err
	}

	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return nil, fmt.Errorf("storage setup already running")
	}
	cur, _ := m.Get()
	if cur != nil && cur.State == "running" {
		m.mu.Unlock()
		return nil, fmt.Errorf("storage setup already running")
	}
	m.running = true
	m.mu.Unlock()

	sc := expectedStorageClass(req)
	now := time.Now().UTC()
	st := SetupState{
		ID: setupID, Backend: req.Backend, RookMode: req.RookMode, Device: req.Device,
		State: "running", Message: "Starting storage setup", StorageClass: sc,
		StartedAt: now, UpdatedAt: now,
	}
	if err := m.writeStatus(st); err != nil {
		m.mu.Lock()
		m.running = false
		m.mu.Unlock()
		return nil, err
	}
	_ = os.WriteFile(m.LogPath(), []byte(""), 0o644)

	go m.run(req, st)
	return &st, nil
}

func (m *SetupManager) run(req SetupRequest, st SetupState) {
	defer func() {
		m.mu.Lock()
		m.running = false
		m.mu.Unlock()
	}()

	script, args, err := buildSetupCommand(m, req)
	if err != nil {
		st.State = "failed"
		st.Error = err.Error()
		st.Message = "Invalid setup request"
		st.UpdatedAt = time.Now().UTC()
		_ = m.writeStatus(st)
		return
	}

	logf, err := os.OpenFile(m.LogPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		st.State = "failed"
		st.Error = err.Error()
		st.UpdatedAt = time.Now().UTC()
		_ = m.writeStatus(st)
		return
	}
	defer func() { _ = logf.Close() }()

	_, _ = fmt.Fprintf(logf, "[INFO] %s %v\n", script, args)
	cmd := exec.Command(script, args...)
	cmd.Env = os.Environ()
	cmd.Stdout = logf
	cmd.Stderr = logf

	st.Message = setupMessage(req)
	st.UpdatedAt = time.Now().UTC()
	_ = m.writeStatus(st)

	if err := cmd.Run(); err != nil {
		st.State = "failed"
		st.Error = err.Error()
		st.Message = "Storage setup failed"
		st.UpdatedAt = time.Now().UTC()
		_ = m.writeStatus(st)
		_, _ = fmt.Fprintf(logf, "[ERR] %v\n", err)
		return
	}

	st.State = "succeeded"
	st.Message = fmt.Sprintf("Installed %s — use StorageClass %s", req.Backend, st.StorageClass)
	st.UpdatedAt = time.Now().UTC()
	_ = m.writeStatus(st)
	_, _ = fmt.Fprintf(logf, "[OK] %s\n", st.Message)

	if m.onComplete != nil && st.StorageClass != "" {
		m.onComplete(st.StorageClass, req.SetDefault)
	}
}

func (m *SetupManager) writeStatus(st SetupState) error {
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.StatusPath(), append(b, '\n'), 0o644)
}

func validateSetupRequest(req SetupRequest) error {
	switch req.Backend {
	case "longhorn", "rook-ceph":
	default:
		return fmt.Errorf("backend must be longhorn or rook-ceph")
	}
	if req.Backend == "rook-ceph" {
		mode := req.RookMode
		if mode == "" {
			mode = "lab"
		}
		switch mode {
		case "lab", "devices", "pool-only", "device":
		default:
			return fmt.Errorf("rookMode must be lab, devices, pool-only, or device")
		}
		if mode == "device" {
			if req.Device == "" {
				return fmt.Errorf("device path required for rook device mode (e.g. /dev/sdb1)")
			}
			if reason := blockedDeviceReason(req.Device); reason != "" {
				return errors.New(reason)
			}
		}
	}
	return nil
}

func expectedStorageClass(req SetupRequest) string {
	switch req.Backend {
	case "longhorn":
		return "longhorn"
	case "rook-ceph":
		return "rook-ceph-block"
	default:
		return ""
	}
}

func setupMessage(req SetupRequest) string {
	switch req.Backend {
	case "longhorn":
		return "Installing Longhorn CSI and VolumeSnapshotClass"
	case "rook-ceph":
		mode := req.RookMode
		if mode == "" {
			mode = "lab"
		}
		if mode == "device" && req.Device != "" {
			return "Installing Rook Ceph on " + req.Device
		}
		return "Installing Rook Ceph (" + mode + ")"
	default:
		return "Installing storage backend"
	}
}

func buildSetupCommand(m *SetupManager, req SetupRequest) (string, []string, error) {
	switch req.Backend {
	case "longhorn":
		return m.snapshotsScript, []string{"--storage", "longhorn"}, nil
	case "rook-ceph":
		mode := req.RookMode
		if mode == "" {
			mode = "lab"
		}
		if mode == "pool-only" {
			if m.rookScript == "" {
				return "", nil, fmt.Errorf("rook setup script not configured")
			}
			return m.rookScript, []string{"--pool-only"}, nil
		}
		args := []string{"--storage", "rook-ceph"}
		if mode == "device" {
			args = append(args, "--device", req.Device)
			if req.WipeDevice {
				args = append(args, "--wipe-device")
			}
		} else {
			args = append(args, "--rook-mode", mode)
		}
		return m.snapshotsScript, args, nil
	default:
		return "", nil, fmt.Errorf("unsupported backend %q", req.Backend)
	}
}

func blockedDeviceReason(path string) string {
	switch filepath.Clean(path) {
	case "/dev/sda", "/dev/nvme0n1", "/dev/sdb", "/dev/sdb2":
		return "refusing OS or k3s disk: " + path
	}
	if strings.HasPrefix(path, "/dev/sda") || path == "/dev/nvme0n1" {
		return "refusing OS disk path: " + path
	}
	if path == "/dev/sdb2" {
		return "refusing /dev/sdb2 (k3s data partition on this lab)"
	}
	if path == "/dev/sdb" {
		return "refusing whole /dev/sdb — use a dedicated partition like /dev/sdb1"
	}
	return ""
}

// ListBlockDevices returns lsblk output for Rook device picker (best effort).
func ListBlockDevices(ctx context.Context) []BlockDevice {
	out, err := exec.CommandContext(ctx, "lsblk", "-J", "-o", "NAME,PATH,SIZE,MODEL,MOUNTPOINT,TYPE").CombinedOutput()
	if err != nil {
		return nil
	}
	var parsed struct {
		Blockdevices []lsblkNode `json:"blockdevices"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		return nil
	}
	var devices []BlockDevice
	for _, n := range parsed.Blockdevices {
		devices = append(devices, flattenBlockDevices(n, "")...)
	}
	return devices
}

type lsblkNode struct {
	Name       string      `json:"name"`
	Path       string      `json:"path"`
	Size       string      `json:"size"`
	Model      string      `json:"model"`
	Mountpoint string      `json:"mountpoint"`
	Type       string      `json:"type"`
	Children   []lsblkNode `json:"children"`
}

func flattenBlockDevices(n lsblkNode, parentModel string) []BlockDevice {
	model := strings.TrimSpace(n.Model)
	if model == "" {
		model = parentModel
	}
	var out []BlockDevice
	path := n.Path
	if path == "" && n.Name != "" {
		path = "/dev/" + n.Name
	}
	if n.Type == "part" || (n.Type == "disk" && len(n.Children) == 0) {
		d := BlockDevice{
			Name: n.Name, Path: path, Size: n.Size, Model: model, Mountpoint: n.Mountpoint,
		}
		if reason := blockedDeviceReason(path); reason != "" {
			d.Blocked = true
			d.BlockReason = reason
		} else if n.Mountpoint == "" && n.Type == "part" {
			d.Recommended = true
		}
		out = append(out, d)
	}
	for _, c := range n.Children {
		out = append(out, flattenBlockDevices(c, model)...)
	}
	return out
}
