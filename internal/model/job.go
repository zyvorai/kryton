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
