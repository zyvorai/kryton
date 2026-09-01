package model

import "time"

type JobStepState string

const (
	JobStepPending JobStepState = "pending"
	JobStepActive  JobStepState = "active"
	JobStepDone    JobStepState = "done"
	JobStepFailed  JobStepState = "failed"
)

type JobState string

const (
	JobRunning   JobState = "running"
	JobSucceeded JobState = "succeeded"
	JobFailed    JobState = "failed"
)

type JobStep struct {
	ID     string       `json:"id"`
	Label  string       `json:"label"`
	State  JobStepState `json:"state"`
	Detail string       `json:"detail,omitempty"`
}

type JobLogLine struct {
	Time    time.Time `json:"time"`
	Level   string    `json:"level"` // info | ok | warn | err
	Message string    `json:"message"`
}

type Job struct {
	ID              string       `json:"id"`
	Kind            string       `json:"kind"` // machine | golden
	Name            string       `json:"name"`
	State           JobState     `json:"state"`
	ProgressPercent int          `json:"progressPercent"`
	Message         string       `json:"message,omitempty"`
	ConsoleURL      string       `json:"consoleUrl,omitempty"`
	Project         string       `json:"project,omitempty"`
	Steps           []JobStep    `json:"steps"`
	Logs            []JobLogLine `json:"logs"`
	StartedAt       time.Time    `json:"startedAt,omitempty"`
	UpdatedAt       time.Time    `json:"updatedAt,omitempty"`
}
