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
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/zyvorai/kryton/internal/golden"
	"github.com/zyvorai/kryton/internal/model"
	"github.com/zyvorai/kryton/internal/provider"
	"github.com/zyvorai/kryton/internal/storage"
)

type Service struct {
	Provider     provider.Provider
	Golden       *golden.Manager
	StorageSetup *storage.SetupManager
	Projects     []string
	DockurData   string
	DockurRun    string
}

func (s *Service) List(ctx context.Context) ([]model.Job, error) {
	var out []model.Job
	if s.StorageSetup != nil {
		if j := storageSetupJob(s.StorageSetup); j != nil {
			out = append(out, *j)
		}
	}
	if s.Golden != nil {
		builds, err := s.Golden.List()
		if err == nil {
			for _, b := range builds {
				if j := goldenJob(s, b); j != nil {
					out = append(out, *j)
				}
			}
		}
	}
	if s.Provider != nil {
		for _, project := range s.Projects {
			ms, err := s.Provider.List(ctx, project)
			if err != nil {
				continue
			}
			for _, m := range ms {
				if j := machineJob(m); j != nil {
					j.Logs = append(j.Logs, s.tailDockurLogs(m)...)
					out = append(out, *j)
				}
			}
		}
	}
	sortJobs(out)
	return out, nil
}

func (s *Service) Get(ctx context.Context, id string) (*model.Job, error) {
	items, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	for _, j := range items {
		if j.ID == id {
			copy := j
			return &copy, nil
		}
	}
	return nil, os.ErrNotExist
}

func machineJob(m model.Machine) *model.Job {
	if m.Provider == "demo" {
		return nil
	}
	show := m.State == model.StateProvisioning || m.State == model.StateStarting || m.State == model.StatePending || m.State == model.StateFailed
	if !show && m.Provider == "dockur" && m.State == model.StateRunning {
		msg := strings.ToLower(m.Message)
		progress := 100
		if m.ProgressPercent != nil {
			progress = *m.ProgressPercent
		}
		show = progress < 100 ||
			strings.Contains(msg, "install") ||
			strings.Contains(msg, "console") ||
			strings.Contains(msg, "download") ||
			strings.Contains(msg, "setup") ||
			time.Since(m.CreatedAt) < 6*time.Hour
	}
	if !show {
		return nil
	}
	progress := 0
	if m.ProgressPercent != nil {
		progress = *m.ProgressPercent
	}
	state := model.JobRunning
	switch m.State {
	case model.StateFailed:
		state = model.JobFailed
	case model.StateRunning, model.StateStopped:
		if progress >= 100 {
			state = model.JobSucceeded
		}
	}

	stepIdx := machineStepIndex(m)
	steps := machineSteps(stepIdx, m.Message)
	logs := []model.JobLogLine{
		{Time: m.CreatedAt, Level: "info", Message: "Job created for " + m.Spec.Name},
	}
	if m.Message != "" {
		logs = append(logs, model.JobLogLine{Time: m.UpdatedAt, Level: logLevelForState(m.State), Message: m.Message})
	}
	if m.ConsoleURL != "" {
		logs = append(logs, model.JobLogLine{Time: m.UpdatedAt, Level: "ok", Message: "Console available at " + m.ConsoleURL})
	}

	return &model.Job{
		ID: "machine:" + m.ID, Kind: "machine", Name: m.Spec.Name, Project: m.Project,
		State: state, ProgressPercent: progress, Message: m.Message, ConsoleURL: m.ConsoleURL,
		Steps: steps, Logs: logs, StartedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}

func machineStepIndex(m model.Machine) int {
	msg := strings.ToLower(m.Message)
	switch m.State {
	case model.StateFailed:
		return 0
	case model.StateProvisioning:
		if strings.Contains(msg, "download") || strings.Contains(msg, "waiting") {
			return 2
		}
		if strings.Contains(msg, "install") {
			return 3
		}
		return 1
	case model.StateRunning:
		if m.ProgressPercent != nil && *m.ProgressPercent >= 100 {
			return 5
		}
		return 4
	case model.StateStarting:
		return 1
	default:
		return 1
	}
}

func machineSteps(active int, detail string) []model.JobStep {
	labels := []string{
		"Allocate resources",
		"Start dockur/windows",
		"Download Windows media",
		"Unattended Windows setup",
		"Console ready",
		"Install complete",
	}
	steps := make([]model.JobStep, len(labels))
	for i, label := range labels {
		st := model.JobStepPending
		if i+1 < active {
			st = model.JobStepDone
		} else if i+1 == active {
			st = model.JobStepActive
		}
		d := ""
		if st == model.JobStepActive && detail != "" {
			d = detail
		}
		steps[i] = model.JobStep{ID: strings.ReplaceAll(strings.ToLower(label), " ", "-"), Label: label, State: st, Detail: d}
	}
	return steps
}

func goldenJob(s *Service, b model.GoldenBuild) *model.Job {
	active := b.State != model.GoldenReady && b.State != model.GoldenFailed && b.State != model.GoldenIdle
	recent := time.Since(b.UpdatedAt) < 4*time.Hour
	if !active && !recent {
		return nil
	}
	state := model.JobRunning
	switch b.State {
	case model.GoldenReady:
		state = model.JobSucceeded
	case model.GoldenFailed:
		state = model.JobFailed
	}
	stepIdx := goldenStepIndex(b)
	steps := goldenSteps(stepIdx, b.Message)
	logs := readGoldenLogs(s, b.ID)
	if len(logs) == 0 {
		logs = []model.JobLogLine{
			{Time: b.StartedAt, Level: "info", Message: "Golden image build started (" + b.ImageID + ")"},
		}
		if b.Message != "" {
			logs = append(logs, model.JobLogLine{Time: b.UpdatedAt, Level: "info", Message: b.Message})
		}
		if b.Error != "" {
			logs = append(logs, model.JobLogLine{Time: b.UpdatedAt, Level: "err", Message: b.Error})
		}
		if b.OutputPath != "" {
			logs = append(logs, model.JobLogLine{Time: b.UpdatedAt, Level: "ok", Message: "Artifact: " + b.OutputPath})
		}
	}
	return &model.Job{
		ID: "golden:" + b.ID, Kind: "golden", Name: "Golden " + b.ImageID,
		State: state, ProgressPercent: b.ProgressPercent, Message: b.Message, ConsoleURL: b.ConsoleURL,
		Steps: steps, Logs: logs, StartedAt: b.StartedAt, UpdatedAt: b.UpdatedAt,
	}
}

func goldenStepIndex(b model.GoldenBuild) int {
	switch b.Phase {
	case "pull", "prepare":
		return 1
	case "download":
		return 2
	case "windows_setup":
		return 3
	case "generalize", "sysprep":
		return 4
	case "convert", "complete":
		return 5
	}
	switch b.State {
	case model.GoldenCapturing:
		return 5
	case model.GoldenSysprep:
		return 4
	case model.GoldenInstalling:
		return 3
	default:
		return 1
	}
}

func goldenSteps(active int, detail string) []model.JobStep {
	labels := []string{
		"Start dockur builder",
		"Download Windows ISO",
		"Install Windows",
		"Sysprep generalize",
		"Capture golden qcow2",
	}
	steps := make([]model.JobStep, len(labels))
	for i, label := range labels {
		st := model.JobStepPending
		if i+1 < active {
			st = model.JobStepDone
		} else if i+1 == active {
			st = model.JobStepActive
		}
		d := ""
		if st == model.JobStepActive && detail != "" {
			d = detail
		}
		steps[i] = model.JobStep{ID: strings.ReplaceAll(strings.ToLower(label), " ", "-"), Label: label, State: st, Detail: d}
	}
	return steps
}

func (s *Service) tailDockurLogs(m model.Machine) []model.JobLogLine {
	if m.Provider != "dockur" || s.DockurRun == "" {
		s.DockurRun = "docker"
	}
	name := m.ID + "-windows-1"
	cmd := exec.Command(s.DockurRun, "logs", "--tail", "40", name)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil
	}
	var lines []model.JobLogLine
	now := time.Now().UTC()
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		level := "info"
		low := strings.ToLower(line)
		switch {
		case strings.Contains(low, "error") || strings.Contains(low, "fail"):
			level = "err"
		case strings.Contains(low, "warn"):
			level = "warn"
		case strings.Contains(low, "ready") || strings.Contains(low, "complete") || strings.Contains(low, "success"):
			level = "ok"
		}
		lines = append(lines, model.JobLogLine{Time: now, Level: level, Message: line})
	}
	return lines
}

func readGoldenLogs(s *Service, buildID string) []model.JobLogLine {
	var paths []string
	if s.Golden != nil {
		paths = append(paths, filepath.Join(s.Golden.BaseDir(), buildID, "job.log"))
	}
	home, _ := os.UserHomeDir()
	paths = append(paths, filepath.Join(home, ".kryton", "golden", buildID, "job.log"))
	var lines []model.JobLogLine
	for _, path := range paths {
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		for _, raw := range strings.Split(string(b), "\n") {
			line := strings.TrimSpace(raw)
			if line == "" {
				continue
			}
			level := "info"
			if strings.HasPrefix(line, "[OK]") {
				level = "ok"
			} else if strings.HasPrefix(line, "[ERR]") {
				level = "err"
			} else if strings.HasPrefix(line, "[WARN]") {
				level = "warn"
			}
			lines = append(lines, model.JobLogLine{Time: time.Now().UTC(), Level: level, Message: line})
		}
		break
	}
	return lines
}

func logLevelForState(st model.MachineState) string {
	switch st {
	case model.StateFailed:
		return "err"
	case model.StateRunning:
		return "ok"
	default:
		return "info"
	}
}

func sortJobs(items []model.Job) {
	for i := 0; i < len(items); i++ {
		for j := i + 1; j < len(items); j++ {
			if items[j].UpdatedAt.After(items[i].UpdatedAt) {
				items[i], items[j] = items[j], items[i]
			}
		}
	}
}

func storageSetupJob(m *storage.SetupManager) *model.Job {
	st, err := m.Get()
	if err != nil || st == nil {
		return nil
	}
	active := st.State == "running"
	recent := time.Since(st.UpdatedAt) < 2*time.Hour
	if !active && !(recent && st.State != "idle" && st.State != "") {
		return nil
	}
	state := model.JobRunning
	switch st.State {
	case "succeeded":
		state = model.JobSucceeded
	case "failed":
		state = model.JobFailed
	case "idle", "":
		return nil
	}
	progress := 50
	if state == model.JobSucceeded {
		progress = 100
	} else if state == model.JobFailed {
		progress = 0
	}
	labels := []string{"Enable KubeVirt snapshots", "Install CSI backend", "Apply StorageClass", "Verify cluster inventory"}
	stepIdx := 2
	if state == model.JobSucceeded {
		stepIdx = 5
	} else if state == model.JobFailed {
		stepIdx = 2
	}
	steps := make([]model.JobStep, len(labels))
	for i, label := range labels {
		stepSt := model.JobStepPending
		if i+1 < stepIdx {
			stepSt = model.JobStepDone
		} else if i+1 == stepIdx && state == model.JobRunning {
			stepSt = model.JobStepActive
		}
		d := ""
		if stepSt == model.JobStepActive {
			d = st.Message
		}
		steps[i] = model.JobStep{ID: strings.ReplaceAll(strings.ToLower(label), " ", "-"), Label: label, State: stepSt, Detail: d}
	}
	var logs []model.JobLogLine
	for _, line := range m.Logs(120) {
		level := "info"
		switch {
		case strings.HasPrefix(line, "[ERR]"):
			level = "err"
		case strings.HasPrefix(line, "[OK]"):
			level = "ok"
		case strings.HasPrefix(line, "[WARN]"):
			level = "warn"
		}
		logs = append(logs, model.JobLogLine{Time: st.UpdatedAt, Level: level, Message: strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(line, "[INFO] "), "[ERR] "), "[OK] ")})
	}
	name := "Storage: " + st.Backend
	if st.Device != "" {
		name += " (" + st.Device + ")"
	}
	return &model.Job{
		ID: "storage:setup", Kind: "storage", Name: name, State: state,
		ProgressPercent: progress, Message: st.Message, Steps: steps, Logs: logs,
		StartedAt: st.StartedAt, UpdatedAt: st.UpdatedAt,
	}
}
