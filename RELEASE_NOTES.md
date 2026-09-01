# Kryton release notes

## Unreleased

### Added

- **Dockur lab provider** — provision real Windows guests via [dockur/windows](https://github.com/dockur/windows) (Docker/Podman + KVM). Image catalog maps Kryton image IDs to dockur `VERSION` codes.
- **Doctor diagnostics** — `krytonctl doctor` and `GET /api/v1/doctor` validate auth, projects, catalog, provider health, container runtime, compose, KVM, and data-dir writability.
- **Install progress** — machine responses include `consoleUrl`, `progressPercent`, and `message` for dockur and demo providers.
- **Install events** — `io.kryton.machine.install.started` emitted when dockur unattended setup begins.
- **Remote deploy** — `scripts/deploy-remote.sh` (GuestKit-style SSH + rsync + systemd) and `make deploy-remote`.
- **UI polish** — Apple-inspired typography, collapsible sidebar, refined light/dark palette.

### Docs

- New guides: [DOCKUR.md](docs/DOCKUR.md), expanded [DEPLOY-REMOTE.md](docs/DEPLOY-REMOTE.md), [API.md](docs/API.md), [ARCHITECTURE.md](docs/ARCHITECTURE.md).

---

## 1.0.0

Initial public release of Kryton as a production-oriented standalone control plane.

### Major changes

- Replaced `kubectl` / `virtctl` subprocess execution with direct Kubernetes REST API integration.
- Introduced stable provider-neutral machine UUIDs and explicit provider references.
- Added KubeVirt state translation from `VirtualMachine.status.printableStatus` plus VMI IP discovery.
- Added project RBAC, hashed API keys, trusted proxy identity with a shared proxy secret, and secure production configuration checks.
- Added CloudEvents-compatible history, SSE streaming, webhooks, TTL reconciliation, structured logging, metrics, readiness, and graceful shutdown.
- Added a complete responsive product dashboard with Overview, Machines, Images, Activity, Settings, machine detail actions, dark/light appearance, and session-token support.
- Added Helm RBAC, hardened container settings, OpenAPI, CLI, architecture/deployment docs, race tests, and provider integration tests.

The release source is Apache-2.0 licensed and contains no Windows media or activation material.
