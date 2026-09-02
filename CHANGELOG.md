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
- Per-symbol godoc comments across `internal/`/`cmd/` exported types and functions.
- **API rate limiting** — `KRYTON_RATE_LIMIT_RPS`/`KRYTON_RATE_LIMIT_BURST` (Helm: `rateLimit.rps`/`.burst`), a per-caller token bucket keyed by API-key name (or remote address when auth is disabled); disabled by default. Returns `429`/`RATE_LIMITED`.
- **Pagination** on `GET /api/v1/machines` — `?limit=` (default 50, max 500) and `?cursor=`, returning `nextCursor` in the response envelope, matching the existing `GET /api/v1/events` pattern.
- **Helm**: `serviceMonitor` (Prometheus Operator scrape config for `/metrics`) and `podDisruptionBudget` templates, both disabled by default.
- CI: license-header check, `golangci-lint`, `govulncheck`, `gosec`, and a Trivy image scan on the multi-arch (`linux/amd64`,`linux/arm64`) image build; `.github/dependabot.yml` for Go modules, the Docker base image, and Actions.
- GitHub PR template and bug/feature issue templates under `.github/`.
- `--port <N>` flag on `scripts/deploy-remote.sh` (previously only settable via `KRYTON_PORT`).
- Expanded unit test coverage: `internal/reconciler` (0→95.5%), `internal/config` (0→87.9%), `internal/kubeapi` (0→33.6%), `internal/jobs` (0→71.6%), `internal/catalog` (0→100%), `internal/images` (0→70.7%), plus additions to `internal/api`, `internal/doctor`, and `internal/storage`.

### Fixed

- Helm chart pods failing to start under the chart's own default hardened security context: `podSecurityContext`/`containerSecurityContext` now set an explicit numeric `runAsUser`/`runAsGroup: 65532` (some kubelet/containerd versions can't verify `runAsNonRoot` against the distroless image's named `nonroot` user without one), and `KRYTON_STORAGE_CONFIG_FILE`/`KRYTON_SETTINGS_CONFIG_FILE` now point at an `emptyDir`-backed `/var/lib/kryton` so `readOnlyRootFilesystem: true` doesn't block krytond's local state writes.
- `go.mod` `gorilla/websocket` `// indirect` drift (it's imported directly, and `golang.org/x/time` for rate limiting is now a direct dependency too).
- A handful of pre-existing lint findings surfaced by the new `golangci-lint`/`gosec` CI gate: an always-first-item-only loop in `internal/doctor`'s KubeVirt feature-gate check, an unsafe `fmt.Errorf(reason)` with user-controlled content in `internal/storage`, several unchecked `Close()`/`Fprintf` errors, and one dead-code method in `internal/api`.

### Docs

- `docs/API.md`: pagination and rate-limiting sections.
- `docs/DEPLOYMENT.md`: observability & availability section (ServiceMonitor, PDB, rate limiting, single-replica caveat).
- `docs/ARCHITECTURE.md` / `docs/GA.md`: documented that `krytond` cannot safely run more than one replica today (TTL reconciler has no leader election; event bus is in-process).
- `CHANGELOG.md` (this file) and `deploy/helm/kryton/README.md` added.

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
