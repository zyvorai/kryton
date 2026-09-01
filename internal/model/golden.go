package model

import "time"

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

type GoldenBuild struct {
	ID              string           `json:"id"`
	Version         string           `json:"version"`
	ImageID         string           `json:"imageId"`
	State           GoldenBuildState `json:"state"`
	Phase           string           `json:"phase,omitempty"`
	ProgressPercent int              `json:"progressPercent"`
	Message         string           `json:"message"`
	ConsoleURL      string           `json:"consoleUrl,omitempty"`
	OutputPath         string           `json:"outputPath,omitempty"`
	SHA256             string           `json:"sha256,omitempty"`
	StartedAt          time.Time        `json:"startedAt,omitempty"`
	UpdatedAt          time.Time        `json:"updatedAt,omitempty"`
	Error              string           `json:"error,omitempty"`
	BootstrapState     string           `json:"bootstrapState,omitempty"`
	BootstrapMessage   string           `json:"bootstrapMessage,omitempty"`
	DataSource         string           `json:"dataSource,omitempty"`
}

type GoldenStartRequest struct {
	ImageID string `json:"imageId"`
	Version string `json:"version,omitempty"`
	Auto    bool   `json:"auto,omitempty"`
}
