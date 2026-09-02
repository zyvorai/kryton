# Kryton release notes

Narrative release write-ups. For the flat, Keep-a-Changelog-style version index, see [CHANGELOG.md](CHANGELOG.md).

## 1.2.0

Reliability and hardening release — no breaking changes.

### Added

- **API rate limiting** — cap requests per caller with `KRYTON_RATE_LIMIT_RPS`/`KRYTON_RATE_LIMIT_BURST` (Helm: `rateLimit.rps`/`.burst`). Off by default; a token bucket keyed by API-key name (or remote address when auth is disabled).
- **Pagination** on `GET /api/v1/machines` (`?limit=`/`?cursor=`/`nextCursor`), matching the existing events endpoint's pattern.
- **Helm**: `serviceMonitor` and `podDisruptionBudget` templates (both opt-in), plus a documented single-replica caveat — see [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).
- **CI hardening**: license-header enforcement, `golangci-lint`, `govulncheck`, `gosec`, a Trivy scan on the published image, multi-arch (`amd64`/`arm64`) builds, and Dependabot for Go/Docker/Actions.
- Extensive new unit test coverage across previously-untested packages (`reconciler`, `config`, `kubeapi`, `jobs`, `catalog`, `images`, plus more in `api`/`doctor`/`storage`).
- Full godoc coverage — every package and exported symbol now has real documentation.
- `--port <N>` flag on `scripts/deploy-remote.sh`.

### Fixed

- **Helm chart pods failing to start** under the chart's own default hardened security settings — a real bug found via live-cluster testing, not just `helm template`. Fixed with an explicit numeric `runAsUser`/`runAsGroup: 65532` and a writable `emptyDir` for krytond's local state under `readOnlyRootFilesystem: true`. If you deployed the chart before this release with default values, your pods were likely crash-looping or refusing to start — upgrade to pick up the fix.
- `go.mod` dependency-directness drift.
- A batch of latent bugs the new lint gate caught, including one real correctness bug in a KubeVirt feature-gate check that was silently only checking the first item in a list.
- Helm chart's default `image.tag` (`1.0.0`) didn't match any image CI actually publishes — `helm install` with defaults would fail to pull. Now defaults to `latest`.

### Docs

- `docs/API.md` and `docs/DEPLOYMENT.md` updated for the above; `CHANGELOG.md` and the Helm chart's own `README.md` added.

## 1.1.0

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
- **Dockur lab provider** — provision real Windows guests via [dockur/windows](https://github.com/dockur/windows).
- **Doctor diagnostics** — `krytonctl doctor` and `GET /api/v1/doctor`.
- **Install progress** — `consoleUrl`, `progressPercent`, `message` on machines.
- **Remote deploy** — `scripts/deploy-remote.sh` and `make deploy-remote`.

The release source is Apache-2.0 licensed and contains no Windows media or activation material.
