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

package provider

import (
	"context"
	"errors"

	"github.com/zyvorai/kryton/internal/model"
)

var (
	ErrNotFound    = errors.New("resource not found")
	ErrConflict    = errors.New("resource already exists")
	ErrUnsupported = errors.New("operation unsupported")
)

type Provider interface {
	Name() string
	Health(context.Context) error
	Capabilities(context.Context) (model.Capabilities, error)
	Create(context.Context, string, model.MachineSpec) (*model.Machine, error)
	Get(context.Context, string, string) (*model.Machine, error)
	List(context.Context, string) ([]model.Machine, error)
	Start(context.Context, string, string) (*model.Machine, error)
	Stop(context.Context, string, string) (*model.Machine, error)
	Delete(context.Context, string, string) error
	Snapshot(context.Context, string, string, string) (*model.Snapshot, error)
	ListSnapshots(context.Context, string, string) ([]model.Snapshot, error)
	RestoreSnapshot(context.Context, string, string, string) (*model.Snapshot, error)
	DeleteSnapshot(context.Context, string, string, string) error
}
