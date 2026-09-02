<div align="center">

# Kryton

### Windows virtualization control plane.

One stable machine API. Kubernetes, KubeVirt, and dockur stay behind the provider boundary.

[![CI](https://github.com/zyvorai/kryton/actions/workflows/ci.yml/badge.svg)](https://github.com/zyvorai/kryton/actions/workflows/ci.yml)
[![License: Apache-2.0](https://img.shields.io/github/license/zyvorai/kryton)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.23+-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Go Reference](https://pkg.go.dev/badge/github.com/zyvorai/kryton.svg)](https://pkg.go.dev/github.com/zyvorai/kryton)

[Quick start](#quick-start) · **[How to use](#how-to-use-kryton)** · [User guide](docs/USER-GUIDE.md) · [Customer ready](docs/CUSTOMER.md) · [KubeVirt](#kubevirt-windows-vms) · [Remote deploy](#remote-deploy) · [Dockur lab](#dockur-lab-provider) · [Helm](#helm-kubevirt) · [API](#api) · [All docs](docs/README.md)

</div>

---

## Contents

- [How to use Kryton](#how-to-use-kryton)
- [Quick start](#quick-start)
- [Install](#install)
- [Project layout](#project-layout)
- [KubeVirt Windows VMs](#kubevirt-windows-vms)
- [Remote deploy](#remote-deploy)
- [Dockur lab provider](#dockur-lab-provider)
- [Helm (KubeVirt)](#helm-kubevirt)
- [Authentication](#authentication)
- [API](#api)
- [Configuration](#configuration)
- [Development](#development)
- [What Kryton is not](#what-kryton-is-not)
- [Docs](#docs)
- [License](#license)

---

**Kryton** is a provider-neutral Windows workload control plane. Portals, CI, and automation talk to one REST + CloudEvents contract — whether the backend is an in-memory demo, real Windows via [dockur/windows](https://github.com/dockur/windows), or production KubeVirt on Kubernetes.

```text
  Veyron / Zeus / Atlas / Haven / Axiom / CI / portals
                    │
             REST + CloudEvents
                    │
                 Kryton
                    │
           Provider interface
         /        |          \
      demo     dockur      KubeVirt
                 │              │
          dockur/windows    Kubernetes API
```

| | |
|---|---|
| **Stable UUIDs** | Independent of namespace / provider name |
| **Auth** | API keys · trusted reverse proxy · secure-by-default |
| **Day-2** | Start / stop / snapshot / TTL expiry · SSE + webhooks |
| **Diagnostics** | `krytonctl doctor` + `/api/v1/doctor` |
| **UI** | Operator dashboard with collapsible rail (light/dark) |
| **License** | Apache-2.0 — no Windows media or keys shipped |

### Providers

| Provider | Use case | Real Windows? |
|----------|----------|---------------|
| `demo` | Local eval, CI smoke tests | No (in-memory) |
| `dockur` | Lab hosts with Docker/Podman + KVM | Yes ([dockur/windows](https://github.com/dockur/windows)) |
| `kubevirt` | Production Kubernetes estates | Yes (operator-managed golden images) |

---

## How to use Kryton

Pick the path that matches your role. Full walkthroughs (UI, CLI, API, troubleshooting): **[docs/USER-GUIDE.md](docs/USER-GUIDE.md)**.

| You are… | Do this | Provider | Auth |
|----------|---------|----------|------|
| **Trying it locally** | `make demo` → open `:8080` | `demo` | off |
| **Running real Windows in a lab** | [Deploy remote](#remote-deploy) → [harden lab](#dockur-lab-provider) → create VM | `dockur` | apikey |
| **Production on Kubernetes** | [Golden image](docs/GOLDEN-IMAGES.md) → [setup-kubevirt](#kubevirt-windows-vms) → Helm | `kubevirt` | apikey + TLS |
| **Integrating a portal / CI** | [API](#api) + `KRYTON_TOKEN` | any | apikey |

### Typical lab workflow (dockur)

```bash
# 1. Deploy to Linux host
make deploy-remote H=<host> U=<user> ARGS='--quick --key'

# 2. Harden shared lab (apikey + auto-auth UI)
ssh <user>@<host> 'cd ~/.deployments/kryton && ./scripts/ensure-api-keys.sh && KRYTON_LAB_PUBLIC_HOST=<host-ip> ./scripts/harden-lab-services.sh'

# 3. Create real Windows
export KRYTON_URL=http://<host>:7088
export KRYTON_TOKEN=$(ssh <user>@<host> 'cat ~/.kryton/lab.token')
krytonctl doctor
krytonctl create --image windows-11-enterprise --cpu 4 --memory 8192 win11-01
krytonctl get <uuid>    # open consoleUrl, Copy RDP in UI
```

### Typical production workflow (KubeVirt)

```bash
# 1. Golden image (45–90 min first time)
./scripts/setup-kubevirt-production.sh --build-golden

# 2. Or from laptop when lab SSH works:
make run-kubevirt-production-remote H=<host> U=<user> BUILD=1

# 3. Verify + create
kubectl -n kryton-images get datasource windows-11-enterprise
krytonctl doctor
krytonctl create --image windows-11-enterprise prod-win-01
```

### UI (all providers)

| Page | What you do |
|------|-------------|
| **Overview** | Project health and activity |
| **Machines** | Create · start/stop · console · snapshots |
| **Images** | Catalog · golden image factory (kubevirt hosts) |
| **Activity** | Event timeline · SSE stream |
| **Settings** | Storage class · Atlas · auth |

Browser auth: paste API token once, or use **lab auto-auth** (`KRYTON_LAB_AUTO_AUTH=true` on shared labs).

### CLI quick reference

```bash
krytonctl list | create NAME | get ID | start|stop|delete ID
krytonctl snapshot ID | snapshots ID | restore ID SNAP
krytonctl doctor | images | capabilities | events
krytonctl generate-token | hash-token TOKEN
```

Create flags: `--image`, `--cpu`, `--memory`, `--disk`, `--ttl`, and all `--dockur-*` options — see [DOCKUR.md](docs/DOCKUR.md).

Environment: `KRYTON_URL`, `KRYTON_TOKEN`, `KRYTON_PROJECT`.

Product site: [zyvor.dev/kryton](https://zyvor.dev/kryton) · Docs: [zyvor.dev/docs/kryton](https://zyvor.dev/docs/kryton)

---

## Quick start

Requires **Go 1.23+**.

```bash
git clone https://github.com/zyvorai/kryton.git
cd kryton
make demo
```

Open [http://localhost:8080](http://localhost:8080).

```bash
# CLI against the local demo
go run ./cmd/krytonctl list
go run ./cmd/krytonctl create win-dev-01
go run ./cmd/krytonctl doctor
```

Local defaults: `demo` provider + authentication **disabled** — evaluation only.

---

## Install

### From source

```bash
make build
sudo install -m755 bin/krytond bin/krytonctl /usr/local/bin/
krytond   # listens on :8080
```

### Container

```bash
docker build -t kryton:dev .
docker run --rm -p 8080:8080 \
  -e KRYTON_PROVIDER=demo \
  -e KRYTON_AUTH_MODE=disabled \
  -e KRYTON_ALLOW_INSECURE=true \
  kryton:dev
```

### Make targets

| Target | What it does |
|--------|----------------|
| `make demo` | Run local demo (auth off) |
| `make build` | Build `bin/krytond` + `bin/krytonctl` |
| `make check` | fmt · test · vet · build |
| `make image` | Docker image `kryton:dev` |
| `make deploy-remote H=… U=…` | SSH deploy (see below) |
| `make setup-kubevirt IMAGE=…` | Bootstrap KubeVirt + API + Windows 11 VM |
| `make setup-kubevirt-production BUILD=1` | Golden + CDI + API + VM |
| `make run-kubevirt-production-remote H=… U=…` | Remote production pipeline |
| `make bootstrap-kubevirt IMAGE=… ID=…` | CDI DataSource only |
| `make build-golden VERSION=11e` | Golden qcow2 via dockur |

---

## Project layout

```text
cmd/krytond/          krytond binary: HTTP entrypoint + embedded operator UI (web/)
cmd/krytonctl/         krytonctl binary: CLI client for the same REST API

internal/api/          HTTP handlers, routing, middleware, OpenAPI serving
internal/provider/      Provider interface — the backend-agnostic machine contract
internal/demo/          In-memory provider (evaluation, CI smoke tests)
internal/dockur/        dockur/windows provider (Docker/Podman + KVM lab hosts)
internal/kubevirt/      KubeVirt provider (production Kubernetes VMs)
internal/kubeapi/       Thin Kubernetes REST + kubeconfig + WebSocket helpers
internal/model/         Shared machine/snapshot/capability/job types
internal/auth/          API-key and proxy authentication
internal/settings/      Persisted operator settings (storage class, Atlas, auth)
internal/storage/       StorageClass detection + Rook/Longhorn installer
internal/golden/        Golden-image (CDI DataSource) lifecycle manager
internal/catalog/       Image catalog exposed to the UI/CLI
internal/images/        Image inventory helpers
internal/jobs/          Long-running job tracking (golden builds, bootstraps)
internal/events/        CloudEvents history + SSE stream + webhook sink
internal/reconciler/    TTL-based machine expiry
internal/doctor/        Provider-aware health checks (krytonctl doctor)
internal/atlas/         Optional Zyvor Atlas storage-control-plane client
internal/config/        Environment-variable configuration loading
internal/metrics/       Prometheus-style metrics
internal/id/            Stable UUID helpers
internal/connection/    Shared connection-test plumbing for Settings

deploy/helm/kryton/     Production Helm chart — see deploy/helm/kryton/README.md for values
deploy/kubevirt/        Namespaces, clone RBAC, DataSource examples
deploy/rook-ceph/       Block pool, StorageClass, VolumeSnapshotClass
scripts/                Deploy, harden, golden-image, and KubeVirt bootstrap scripts
docs/                   Per-role guides — see docs/README.md
examples/               Sample API payloads (auth keys, images, machine)
```

Package-level doc comments live alongside the code — browse them on [pkg.go.dev](https://pkg.go.dev/github.com/zyvorai/kryton) or with `go doc ./internal/...`.

---

## KubeVirt Windows VMs

Create real Windows 11 guests on Kubernetes through the Kryton API — fully automated:

```bash
# Build golden image (WinForge-style) or use your own qcow2
VERSION=11e ./scripts/build-golden-image.sh
# ... Sysprep, then FINALIZE=1 ...

export KRYTON_WINDOWS_IMAGE=./out/windows-11e-golden.qcow2
./scripts/setup-kubevirt.sh
# API on :9088 — create/list/start/stop via REST or krytonctl
```

See **[docs/KUBEVIRT.md](docs/KUBEVIRT.md)** for Helm mode, bootstrap-only, and production auth.  
Golden image pipeline: **[docs/GOLDEN-IMAGES.md](docs/GOLDEN-IMAGES.md)**.

---

## Remote deploy

Same pattern as GuestKit: **SSH + rsync → build on the host → systemd demo unit**.

```bash
./scripts/deploy-remote.sh <user>@<host> --key
# or
make deploy-remote H=<host> U=<user> ARGS='--quick --key'
```

| Flag | Meaning |
|------|---------|
| `--quick` | Skip Go toolchain install when `go` is already present |
| `--build-local` | Ship Linux binaries built on your laptop |
| `--no-service` | Install binaries only |
| `--port <N>` | Listen port for the demo systemd unit (default `8080`, or `$KRYTON_PORT`) — use when the default port is already taken on the host |
| `--verify-only` | Hit `/readyz` |
| `--uninstall` | Remove unit + binaries + staging dir |

Full guide: **[docs/DEPLOY-REMOTE.md](docs/DEPLOY-REMOTE.md)**.

---

## Dockur lab provider

Run real Windows guests on a Linux host with Docker/Podman and KVM — inspired by [dockur/windows](https://github.com/dockur/windows) and [WinPodX](https://github.com/kernalix7/winpodx).

```bash
export KRYTON_PROVIDER=dockur
export KRYTON_DOCKUR_RUNTIME=docker
export KRYTON_DOCKUR_PUBLIC_HOST=<your-host-ip>
krytond

krytonctl doctor
krytonctl create --image windows-11-enterprise --cpu 4 --memory 8192 lab-win01
krytonctl get <id>   # open consoleUrl to watch install
```

See **[docs/DOCKUR.md](docs/DOCKUR.md)** for image mapping, ports, and requirements.

---

## Helm (KubeVirt)

Production path: Kubernetes with KubeVirt + CDI, administrator-managed Windows `DataSource` objects, and API-key auth.

```bash
export KRYTON_PROVIDER=kubevirt
export KRYTON_PROJECTS=finance,engineering
export KRYTON_DEFAULT_PROJECT=finance
export KRYTON_IMAGE_NAMESPACE=kryton-images
export KRYTON_AUTH_MODE=apikey
export KRYTON_API_KEYS_FILE=/etc/kryton/keys.json
```

```bash
TOKEN=$(krytonctl generate-token)
krytonctl hash-token "$TOKEN"   # store only the hash in keys.json

kubectl -n kryton create secret generic kryton-auth --from-file=keys.json
helm upgrade --install kryton ./deploy/helm/kryton -n kryton --create-namespace
```

Deep dive: **[docs/DEPLOYMENT.md](docs/DEPLOYMENT.md)** · **[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)** · **[docs/API.md](docs/API.md)**.

---

## Authentication

Roles: `viewer` · `operator` · `admin` (always intersected with project scope).

```bash
export KRYTON_TOKEN='<raw token>'
export KRYTON_PROJECT=finance
krytonctl list
```

For browser SSO, terminate identity at a reverse proxy and set `X-Kryton-User` / `X-Kryton-Role` / `X-Kryton-Projects` with `KRYTON_AUTH_MODE=proxy`.

---

## API

```text
GET    /api/v1                          # discovery (public)
GET    /openapi.yaml                    # OpenAPI 3.1 (public)
GET    /api/v1/projects
GET    /api/v1/capabilities
GET    /api/v1/doctor
GET    /api/v1/settings · PUT · POST …/test
POST   /api/v1/integrations/atlas/test
GET    /api/v1/storage · config · setup
GET    /api/v1/images
GET    /api/v1/jobs
GET    /api/v1/summary?project=finance
GET    /api/v1/machines?project=finance
POST   /api/v1/machines
GET    /api/v1/machines/{id}?project=finance
POST   /api/v1/machines/{id}/start|stop|snapshot?project=finance
GET    /api/v1/machines/{id}/snapshots?project=finance
POST   /api/v1/machines/{id}/snapshots/{sid}/restore?project=finance
DELETE /api/v1/machines/{id}/snapshots/{sid}?project=finance
DELETE /api/v1/machines/{id}?project=finance
GET    /api/v1/events
GET    /api/v1/events/stream
```

Machine responses include `consoleUrl`, `progressPercent`, and `message` when the provider supports install progress (dockur, demo).

OpenAPI: [`openapi.yaml`](openapi.yaml) (also served at `/openapi.yaml`). Full contract: [docs/API.md](docs/API.md).

---

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `KRYTON_PROVIDER` | `demo` | `demo` · `dockur` · `kubevirt` |
| `KRYTON_AUTH_MODE` | `disabled` | `disabled` · `apikey` · `proxy` |
| `KRYTON_ALLOW_INSECURE` | `false` | Lab/dev TLS skip + enables lab auto-auth |
| `KRYTON_LAB_AUTO_AUTH` | `false` | UI auto-loads bearer from `lab.token` (requires apikey + allow insecure) |
| `KRYTON_PROJECTS` | `default` | Comma-separated project list |
| `KRYTON_ADDR` | `:8080` | Listen address |
| `KRYTON_DOCKUR_RUNTIME` | `docker` | `docker` or `podman` |
| `KRYTON_DOCKUR_DATA_DIR` | *(temp)* | Compose state directory |
| `KRYTON_KUBECONFIG` | `~/.kube/config` | Kubernetes credentials (kubevirt provider) |
| `KRYTON_STORAGE_CLASS` | *(cluster default)* | PVC StorageClass for new KubeVirt disks (`rook-ceph-block` or `longhorn`; see [STORAGE.md](docs/STORAGE.md)) |
| `KRYTON_CORS_ORIGINS` | *(empty)* | Comma-separated browser origins allowed to call the API (`*` for lab). Needed when Axiom/Haven call Kryton from another origin |
| `KRYTON_ATLAS_URL` | *(empty)* | Atlas gateway base URL for Settings → Integrations (e.g. `http://127.0.0.1:5110`) |
| `KRYTON_ATLAS_TOKEN` | *(empty)* | Atlas bearer JWT (`product.service.kryton`); see [docs/ATLAS.md](docs/ATLAS.md) |
| `KRYTON_EVENTS_FILE` | *(memory only)* | Append-only JSONL audit log (survives restarts) |
| `KRYTON_EVENT_WEBHOOK_SECRET` | *(none)* | HMAC-SHA256 signature for webhook payloads |
| `KRYTON_DOCKUR_PUBLIC_HOST` | `127.0.0.1` | Hostname/IP for console URLs |
| `KRYTON_RATE_LIMIT_RPS` | `0` (disabled) | Per-caller `/api/*` requests/sec (token bucket keyed by API-key name, or remote address when auth is disabled) |
| `KRYTON_RATE_LIMIT_BURST` | `KRYTON_RATE_LIMIT_RPS` | Burst size for the same token bucket |

`GET /api/v1/machines` supports `?limit=` (default 50, max 500) and `?cursor=` (from a previous response's `nextCursor`) for pagination, same pattern as `GET /api/v1/events`.

Run `krytonctl doctor` after changing provider settings to validate the environment.

---

## Development

```bash
make check      # gofmt + go test ./... + go vet + build
make test        # go test ./...
make race         # go test -race ./...
make vet          # go vet ./...
make fmt          # gofmt -w cmd internal
```

- Provider-specific behavior stays behind `internal/provider.Provider` — never leak `dockur`/`kubevirt` details into `internal/api`.
- Never expose a raw provider identifier (namespace/name, compose project) as the primary machine ID — only the stable UUID.
- New source files need the Apache-2.0 header: `./scripts/add-license-headers.sh`.
- CI (`.github/workflows/ci.yml`) runs license-header check, `golangci-lint`, `govulncheck`, `gosec`, `go vet`, `go test`, builds both binaries, and checks `cmd/krytond/web/app.js` syntax on every push/PR to `main`; on merge it publishes a multi-arch (`linux/amd64`,`linux/arm64`) `ghcr.io/zyvorai/kryton` image and Trivy-scans it for critical/high CVEs. Dependabot keeps Go modules, the base image, and Actions up to date.
- Full guidelines: [CONTRIBUTING.md](CONTRIBUTING.md).

---

## What Kryton is not

- Not a Windows installer, activation service, or media distributor.
- Not a raw KubeVirt YAML factory for callers — that stays inside the provider.
- Microsoft media, activation, and entitlement remain the **operator's** responsibility.

---

## Docs

| Doc | Topic |
|-----|--------|
| **[USER-GUIDE.md](docs/USER-GUIDE.md)** | **How to use Kryton — all personas (start here)** |
| [docs/README.md](docs/README.md) | Documentation index |
| [DEPLOY-REMOTE.md](docs/DEPLOY-REMOTE.md) | SSH / rsync lab deploy |
| [DOCKUR.md](docs/DOCKUR.md) | Real Windows via dockur/windows provider |
| [CUSTOMER.md](docs/CUSTOMER.md) | Production vs lab readiness checklist |
| [DEPLOYMENT.md](docs/DEPLOYMENT.md) | Production KubeVirt |
| [STORAGE.md](docs/STORAGE.md) | Rook Ceph / Longhorn disks and snapshots |
| [ATLAS.md](docs/ATLAS.md) | Integrate Zyvor Atlas storage control plane |
| [GA.md](docs/GA.md) | Production go-live checklist |
| [ARCHITECTURE.md](docs/ARCHITECTURE.md) | Provider boundary & IDs |
| [API.md](docs/API.md) | HTTP contract + third-party integration |
| [SECURITY.md](SECURITY.md) | Reporting & posture |
| [CONTRIBUTING.md](CONTRIBUTING.md) | How to contribute |
| [CHANGELOG.md](CHANGELOG.md) | Version-by-version change index |
| [RELEASE_NOTES.md](RELEASE_NOTES.md) | Narrative release write-ups |
| [deploy/helm/kryton/README.md](deploy/helm/kryton/README.md) | Helm chart values & overlays |

---

## License

Apache License 2.0. See [`LICENSE`](LICENSE) and [`NOTICE`](NOTICE).

**Kryton** — Windows virtualization control plane. · [zyvorai](https://github.com/zyvorai)
