# Changelog

All notable changes to Kryton are documented here, in [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) format. Kryton follows [Semantic Versioning](https://semver.org/).

For narrative release write-ups (what changed and why, aimed at operators), see [RELEASE_NOTES.md](RELEASE_NOTES.md). This file is the flat, version-by-version index; RELEASE_NOTES.md is the prose companion — keep both in sync when cutting a release.

## [Unreleased]

### Added

- Package-level godoc comments across every `internal/`/`cmd/` package, plus full per-method documentation on the `provider.Provider` interface.
- Comprehensive [docs/USER-GUIDE.md](docs/USER-GUIDE.md) covering all four personas (evaluator, lab operator, production operator, integrator).
- KubeVirt production setup scripts (`scripts/setup-kubevirt-production.sh`, `scripts/run-kubevirt-production-remote.sh`) automating golden-image build + CDI bootstrap end to end.
- Lab token auto-auth for the browser UI (`KRYTON_LAB_AUTO_AUTH`), so shared lab hosts don't require pasting a bearer token per session.
- Apache-2.0 license headers applied across all Go, shell, web, and OpenAPI source files (`scripts/add-license-headers.sh`).
- README table of contents, full project-layout tree, and a Development section (make targets, provider-boundary rules, CI summary).
- Helm chart `README.md` documenting `values.yaml` keys, the four values overlays, the auth-secret contract, and RBAC scope.

## [1.1.0] - 2026-09-01

### Added

- **Dockur options (full)** — `spec.dockur` on create: credentials, locale, AD join, shared/OEM folders, post-install command, custom ISO, edition, audio, secure boot + TPM, extra disks, autologin.
- **`rdpUsername`** on machine responses; password redacted on GET.
- **Expanded catalog** — Windows 10/11 LTSC, Tiny11, Server 2016, Enterprise variants (12 dockur images).
- **UI** — Dockur create panel, detail **Dockur options** summary, Copy RDP, embedded noVNC console (no CDN dependency).
- **CLI** — `krytonctl create … --dockur-*` flags for all dockur fields.
- **Lab hardening** — `scripts/ensure-api-keys.sh`, `scripts/harden-lab-services.sh` (apikey auth on shared lab hosts).
- **Customer profile** — `deploy/helm/kryton/values-customer.yaml`, [docs/CUSTOMER.md](docs/CUSTOMER.md).

### Fixed

- KubeVirt console iframe blocked by CSP / `X-Frame-Options`.
- VNC proxy 500 (missing `Hijacker` on response writer).
- Storage picker hang (`lsblk` timeout); Rook Ceph preferred in UI sort order.
- Actionable API errors with hints on create failures.

### Docs

- [DOCKUR.md](docs/DOCKUR.md) — full option matrix, CLI examples, auth guidance.
- OpenAPI `DockurOptions` schema on `CreateMachine`.

## [1.0.0] - Initial release

Initial public release of Kryton as a production-oriented standalone control plane.

### Added

- Stable provider-neutral machine UUIDs and explicit provider references, replacing `kubectl` / `virtctl` subprocess execution with direct Kubernetes REST API integration.
- KubeVirt state translation from `VirtualMachine.status.printableStatus` plus VMI IP discovery.
- Project RBAC, hashed API keys, trusted-proxy identity with a shared proxy secret, and secure production configuration checks.
- CloudEvents-compatible history, SSE streaming, webhooks, TTL reconciliation, structured logging, metrics, readiness, and graceful shutdown.
- Complete responsive product dashboard: Overview, Machines, Images, Activity, Settings, machine detail actions, dark/light appearance, session-token support.
- Helm RBAC, hardened container settings, OpenAPI, CLI, architecture/deployment docs, race tests, and provider integration tests.
- **Dockur lab provider** — provision real Windows guests via [dockur/windows](https://github.com/dockur/windows).
- **Doctor diagnostics** — `krytonctl doctor` and `GET /api/v1/doctor`.
- **Install progress** — `consoleUrl`, `progressPercent`, `message` on machines.
- **Remote deploy** — `scripts/deploy-remote.sh` and `make deploy-remote`.

The release source is Apache-2.0 licensed and contains no Windows media or activation material.

[Unreleased]: https://github.com/zyvorai/kryton/compare/v1.1.0...HEAD
[1.1.0]: https://github.com/zyvorai/kryton/releases/tag/v1.1.0
