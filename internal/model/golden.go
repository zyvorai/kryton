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

package model

import "time"

// GoldenBuildState tracks progress through the golden-image pipeline
// driven by internal/golden: boot a dockur instance, install Windows,
// Sysprep it, then capture the disk as a reusable qcow2.
type GoldenBuildState string

const (
	GoldenIdle       GoldenBuildState = "idle"
	GoldenStarting   GoldenBuildState = "starting"
	GoldenInstalling GoldenBuildState = "installing"
	GoldenSysprep    GoldenBuildState = "sysprep"
	GoldenCapturing  GoldenBuildState = "capturing"
	GoldenReady      GoldenBuildState = "ready"
	GoldenFailed     GoldenBuildState = "failed"
)

// GoldenBuild is the status of one golden-image build tracked by
// internal/golden, exposed via the Images page and GET /api/v1/jobs.
type GoldenBuild struct {
	ID              string           `json:"id"`
	Version         string           `json:"version"`
	ImageID         string           `json:"imageId"`
	State           GoldenBuildState `json:"state"`
	Phase           string           `json:"phase,omitempty"`
	ProgressPercent int              `json:"progressPercent"`
	Message         string           `json:"message"`
	ConsoleURL      string           `json:"consoleUrl,omitempty"`
	// OutputPath and SHA256 are set once the qcow2 has been captured to disk.
	OutputPath string    `json:"outputPath,omitempty"`
	SHA256     string    `json:"sha256,omitempty"`
	StartedAt  time.Time `json:"startedAt,omitempty"`
	UpdatedAt  time.Time `json:"updatedAt,omitempty"`
	Error      string    `json:"error,omitempty"`
	// BootstrapState/BootstrapMessage/DataSource track the follow-on step
	// that imports OutputPath into a KubeVirt CDI DataSource; empty until
	// that step is requested.
	BootstrapState   string `json:"bootstrapState,omitempty"`
	BootstrapMessage string `json:"bootstrapMessage,omitempty"`
	DataSource       string `json:"dataSource,omitempty"`
	// Certified and ValidationScore come from an offline guestkit gate
	// (github.com/zyvorai/guestkit) run against OutputPath during capture;
	// both stay zero-valued when guestkit wasn't available on the build
	// host. A false Certified never blocks Bootstrap — it's informational.
	Certified       bool    `json:"certified,omitempty"`
	ValidationScore float64 `json:"validationScore,omitempty"`
}

// GoldenStartRequest is the payload for starting a new golden-image build.
type GoldenStartRequest struct {
	ImageID string `json:"imageId"`
	Version string `json:"version,omitempty"`
	// Auto skips interactive Sysprep confirmation and finalizes automatically.
	Auto bool `json:"auto,omitempty"`
}
