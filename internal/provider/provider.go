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
