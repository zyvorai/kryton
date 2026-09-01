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
