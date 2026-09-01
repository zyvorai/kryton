package provider

import "context"

// ConsoleTarget identifies a provider guest console endpoint.
type ConsoleTarget struct {
	Namespace   string
	Name        string
	Kind        string // vnc | web
	UpstreamURL string // for web consoles (dockur), reverse-proxied by the API
}

// ConsoleResolver is implemented by providers that expose guest consoles.
type ConsoleResolver interface {
	ConsoleTarget(ctx context.Context, project, machineID string) (*ConsoleTarget, error)
}
