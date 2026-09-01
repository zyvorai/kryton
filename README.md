<div align="center">

# Kryton

### Windows virtualization, beautifully simple.

One stable machine API. Kubernetes and KubeVirt stay behind the provider boundary.

[![CI](https://github.com/zyvorai/kryton/actions/workflows/ci.yml/badge.svg)](https://github.com/zyvorai/kryton/actions/workflows/ci.yml)
[![License: Apache-2.0](https://img.shields.io/github/license/zyvorai/kryton)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.23+-00ADD8?logo=go&logoColor=white)](https://go.dev/)

[Quick start](#quick-start) · [Install](#install) · [Remote deploy](#remote-deploy) · [Helm](#helm-kubevirt) · [API](#api) · [Docs](docs/)

</div>

---

**Kryton** is a provider-neutral Windows workload control plane. Portals, CI, and automation talk to one REST + CloudEvents contract. The KubeVirt provider turns that into VirtualMachines, DataVolumes, and snapshots — without callers ever touching `kubectl` or `virtctl`.

```text
  Veyron / Zeus / Transiva / CI / portals
                    │
             REST + CloudEvents
                    │
                 Kryton
                    │
           Provider interface
              /          \
           demo        KubeVirt ──► Kubernetes API
```

| | |
|---|---|
| **Stable UUIDs** | Independent of namespace / provider name |
| **Auth** | API keys · trusted reverse proxy · secure-by-default |
| **Day-2** | Start / stop / snapshot / TTL expiry · SSE + webhooks |
| **UI** | Apple-inspired dark/light dashboard |
| **License** | Apache-2.0 — no Windows media or keys shipped |

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

---

## Remote deploy

Same pattern as GuestKit: **SSH + rsync → build on the host → systemd demo unit**.

```bash
./scripts/deploy-remote.sh sus@175.110.122.71 --key
# or
make deploy-remote H=175.110.122.71 U=sus ARGS='--quick --key'
```

| Flag | Meaning |
|------|---------|
| `--quick` | Skip Go toolchain install when `go` is already present |
| `--build-local` | Ship Linux binaries built on your laptop |
| `--no-service` | Install binaries only |
| `--verify-only` | Hit `/readyz` |
| `--uninstall` | Remove unit + binaries + staging dir |

Full guide: **[docs/DEPLOY-REMOTE.md](docs/DEPLOY-REMOTE.md)**.

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
GET    /api/v1/projects
GET    /api/v1/images
GET    /api/v1/summary?project=finance
GET    /api/v1/machines?project=finance
POST   /api/v1/machines
GET    /api/v1/machines/{id}?project=finance
POST   /api/v1/machines/{id}/start|stop|snapshot?project=finance
DELETE /api/v1/machines/{id}?project=finance
GET    /api/v1/events
GET    /api/v1/events/stream
```

OpenAPI: [`openapi.yaml`](openapi.yaml).

---

## What Kryton is not

- Not a Windows installer, activation service, or media distributor.
- Not a raw KubeVirt YAML factory for callers — that stays inside the provider.
- Microsoft media, activation, and entitlement remain the **operator’s** responsibility.

---

## Docs

| Doc | Topic |
|-----|--------|
| [DEPLOY-REMOTE.md](docs/DEPLOY-REMOTE.md) | SSH / rsync lab deploy |
| [DEPLOYMENT.md](docs/DEPLOYMENT.md) | Production KubeVirt |
| [ARCHITECTURE.md](docs/ARCHITECTURE.md) | Provider boundary & IDs |
| [API.md](docs/API.md) | HTTP contract |
| [SECURITY.md](SECURITY.md) | Reporting & posture |
| [CONTRIBUTING.md](CONTRIBUTING.md) | How to contribute |

---

## License

Apache License 2.0. See [`LICENSE`](LICENSE) and [`NOTICE`](NOTICE).

**Kryton** — Windows virtualization, beautifully simple. · [zyvorai](https://github.com/zyvorai)
