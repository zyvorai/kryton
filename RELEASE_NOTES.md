# Kryton 1.0.0

This release promotes Kryton from the original prototype to a production-oriented standalone control plane.

## Major changes

- Replaced `kubectl` / `virtctl` subprocess execution with direct Kubernetes REST API integration.
- Introduced stable provider-neutral machine UUIDs and explicit provider references.
- Added KubeVirt state translation from `VirtualMachine.status.printableStatus` plus VMI IP discovery.
- Added project RBAC, hashed API keys, trusted proxy identity with a shared proxy secret, and secure production configuration checks.
- Added CloudEvents-compatible history, SSE streaming, webhooks, TTL reconciliation, structured logging, metrics, readiness, and graceful shutdown.
- Added a complete responsive product dashboard with Overview, Machines, Images, Activity, Settings, machine detail actions, dark/light appearance, and session-token support.
- Added Helm RBAC, hardened container settings, OpenAPI, CLI, architecture/deployment docs, race tests, and provider integration tests.

## Verification performed in this workspace

- `go test ./...`
- `go test -race ./...`
- `go vet ./...`
- `go build ./cmd/krytond ./cmd/krytonctl`
- `node --check cmd/krytond/web/app.js`
- end-to-end demo-provider create/list/stop smoke test
- API-key authentication smoke test
- fake Kubernetes API integration test covering KubeVirt create, stop patch, status mapping, and snapshots

The release source is Apache-2.0 licensed and contains no Windows media or activation material.
