# Kryton user guide

How to use Kryton from first eval through production KubeVirt. One API — three providers.

---

## Choose your path

| You are… | Provider | Auth | Start here |
|----------|----------|------|------------|
| Evaluating the UI/API locally | `demo` | disabled | [§1 Evaluator](#1-evaluator-local-demo) |
| Running real Windows in a lab | `dockur` | apikey on shared hosts | [§2 Lab operator](#2-lab-operator-dockur) |
| Operating production K8s estates | `kubevirt` | apikey + TLS | [§3 Production](#3-production-operator-kubevirt) |
| Wiring a portal or CI pipeline | any | apikey or proxy | [§4 Integrator](#4-integrator-api--automation) |

---

## 1. Evaluator (local demo)

**Goal:** See the dashboard and API without Kubernetes or Docker.

### Install and run

```bash
git clone https://github.com/zyvorai/kryton.git
cd kryton
make demo
```

Open **http://localhost:8080**. Auth is **disabled** — do not expose this port publicly.

### CLI

```bash
go run ./cmd/krytonctl list
go run ./cmd/krytonctl create win-dev-01
go run ./cmd/krytonctl get <uuid>
go run ./cmd/krytonctl doctor
go run ./cmd/krytonctl delete <uuid>
```

### UI walkthrough

1. **Overview** — project summary and recent activity  
2. **Machines** — create, start/stop, open console, snapshots (demo simulates progress)  
3. **Images** — catalog of Windows image IDs  
4. **Activity** — event stream (SSE also at `/api/v1/events/stream`)  
5. **Settings** — provider info, storage, Atlas integration (when configured)

### What you get

In-memory machines with stable UUIDs. No real Windows — suitable for API contract tests and UI exploration.

---

## 2. Lab operator (dockur)

**Goal:** Real Windows guests on a Linux host with Docker/Podman + KVM.

### Requirements

- Linux x86_64, `/dev/kvm`, Docker or Podman + Compose v2  
- ~32–64 GiB disk per VM, outbound HTTPS (dockur downloads ISOs)  
- User in `docker` group if using Docker

### Deploy Kryton to a lab host

```bash
make deploy-remote H=<host> U=<user> ARGS='--quick --key'
```

Then enable dockur (see [DEPLOY-REMOTE.md](DEPLOY-REMOTE.md)) or use hardened lab units:

```bash
ssh <user>@<host>
cd ~/.deployments/kryton
./scripts/ensure-api-keys.sh
KRYTON_LAB_PUBLIC_HOST=<public-ip> ./scripts/harden-lab-services.sh
```

This installs:

- `kryton-dockur.service` on **:7088** (typical)  
- `kryton-kubevirt.service` on **:9088** (typical)  
- apikey auth + **lab auto-auth** for the browser UI

### Authenticate

**Browser:** Open `http://<host>:7088/` — with auto-auth, no token paste needed.

**CLI / scripts:**

```bash
export KRYTON_URL=http://127.0.0.1:7088
export KRYTON_TOKEN=$(cat ~/.kryton/lab.token)
krytonctl doctor
```

### Create a Windows VM

**CLI:**

```bash
krytonctl create \
  --image windows-11-enterprise \
  --cpu 4 --memory 8192 --disk 80 \
  --dockur-username admin \
  --dockur-password 'Str0ng-Lab-Only!' \
  win11-lab-01
```

**UI:** Machines → **Create** → pick image → expand **Dockur options** → submit.

**API:**

```bash
curl -s -X POST "$KRYTON_URL/api/v1/machines" \
  -H "Authorization: Bearer $KRYTON_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "project": "default",
    "name": "lab-win01",
    "image": "windows-11-enterprise",
    "compute": {"cpu": 4, "memoryMiB": 8192},
    "disk": {"sizeGiB": 80},
    "dockur": {"username": "admin", "password": "ChangeMe!", "audio": true}
  }'
```

### While Windows installs

- Open **consoleUrl** in the dashboard (embedded noVNC) or browser  
- Watch `progressPercent` and `message` on the machine detail page  
- Use **Copy RDP** when RDP ports are published (`KRYTON_DOCKUR_RDP_BASE`, default 13389+)

### Day-2

```bash
krytonctl stop <uuid>
krytonctl start <uuid>
krytonctl delete <uuid>
```

Full dockur option matrix: [DOCKUR.md](DOCKUR.md).

### Image catalog (12 images)

| Image ID | Windows variant |
|----------|-----------------|
| `windows-11-enterprise` | 11 Enterprise |
| `windows-11-pro` | 11 Pro |
| `windows-11-ltsc` | 11 LTSC |
| `windows-10-enterprise` | 10 Enterprise |
| `windows-10-pro` | 10 Pro |
| `windows-10-ltsc` | 10 LTSC |
| `windows-tiny11` | Tiny11 |
| `windows-tiny11-core` | Tiny11 Core |
| `windows-server-2025` | Server 2025 |
| `windows-server-2022` | Server 2022 |
| `windows-server-2019` | Server 2019 |
| `windows-server-2016` | Server 2016 |

---

## 3. Production operator (KubeVirt)

**Goal:** Windows VMs on Kubernetes with golden images, CDI, snapshots, and apikey auth.

### Prerequisites

1. Kubernetes + **KubeVirt** + **CDI**  
2. Snapshot-capable CSI (**Rook Ceph** or **Longhorn**) — see [STORAGE.md](STORAGE.md)  
3. Sysprepped **golden qcow2** — see [GOLDEN-IMAGES.md](GOLDEN-IMAGES.md)  
4. `kubectl`, `virtctl`, `qemu-img` on the operator host

### One-shot setup (recommended)

```bash
# Build golden + bootstrap CDI + install API + smoke VM
./scripts/setup-kubevirt-production.sh --build-golden

# Or with existing qcow2:
KRYTON_WINDOWS_IMAGE=./out/windows-11e-golden.qcow2 \
  ./scripts/setup-kubevirt-production.sh

# Remote from laptop:
make run-kubevirt-production-remote H=<host> U=<user> BUILD=1
```

### Customer Helm profile

```bash
./scripts/setup-kubevirt-production.sh \
  --customer-helm \
  --image ./out/windows-11e-golden.qcow2
```

Or manually:

```bash
helm upgrade --install kryton ./deploy/helm/kryton \
  -n kryton --create-namespace \
  -f deploy/helm/kryton/values-customer.yaml
```

### Verify readiness

```bash
kubectl -n kryton-images get datasource windows-11-enterprise
krytonctl doctor   # zero "fail" checks
```

### Create a production VM

```bash
export KRYTON_URL=https://kryton.example.com
export KRYTON_TOKEN=<api-key>
krytonctl create --image windows-11-enterprise --cpu 4 --memory 8192 prod-win-01
```

Snapshots:

```bash
krytonctl snapshot <uuid>
krytonctl snapshots <uuid>
krytonctl restore <uuid> <snapshot-id>
```

Full details: [KUBEVIRT.md](KUBEVIRT.md) · [CUSTOMER.md](CUSTOMER.md) · [GA.md](GA.md).

---

## 4. Integrator (API + automation)

**Goal:** Call Kryton from a portal, pipeline, or another Zyvor product.

### Discovery

```bash
curl -s "$KRYTON_URL/api/v1" | jq .
curl -s "$KRYTON_URL/openapi.yaml" -o openapi.yaml
```

### Authentication

| Mode | When | How |
|------|------|-----|
| `disabled` | Local demo only | No header |
| `apikey` | Lab + production | `Authorization: Bearer <token>` |
| `proxy` | SSO behind nginx/Envoy | `X-Kryton-User`, `X-Kryton-Role`, `X-Kryton-Projects` |

Generate keys:

```bash
TOKEN=$(krytonctl generate-token)
krytonctl hash-token "$TOKEN"   # store hash in keys.json
```

Roles: `viewer` · `operator` · `admin` (scoped to projects).

### Core workflow

```text
GET  /api/v1/capabilities     # what this provider supports
GET  /api/v1/doctor           # preflight before create
POST /api/v1/machines         # create
GET  /api/v1/machines/{id}    # poll state, consoleUrl, IPs
POST /api/v1/machines/{id}/start|stop|snapshot
GET  /api/v1/events/stream    # SSE for automation
```

Machine IDs are **stable UUIDs** — store them in your database, not provider VM names.

### CloudEvents

History and webhooks use CloudEvents-compatible payloads. Configure `KRYTON_EVENT_WEBHOOK_SECRET` for HMAC signatures.

Full contract: [API.md](API.md).

---

## CLI reference

Environment: `KRYTON_URL` (default `http://localhost:8080`), `KRYTON_TOKEN`, `KRYTON_PROJECT` (default `default`).

| Command | Description |
|---------|-------------|
| `krytonctl list` | List machines in project |
| `krytonctl create [flags] NAME` | Create machine |
| `krytonctl get <id>` | Machine detail |
| `krytonctl start\|stop\|delete <id>` | Lifecycle |
| `krytonctl snapshot <id>` | Take snapshot |
| `krytonctl snapshots <id>` | List snapshots |
| `krytonctl restore <id> <snap-id>` | Restore snapshot |
| `krytonctl delete-snapshot <id> <snap-id>` | Delete snapshot |
| `krytonctl doctor` | Environment diagnostics |
| `krytonctl images` | Image catalog |
| `krytonctl capabilities` | Provider capabilities |
| `krytonctl events` | Recent events |
| `krytonctl storage` | Storage classes (kubevirt) |
| `krytonctl set-storage <class>` | Set default StorageClass |
| `krytonctl generate-token` | New random API token |
| `krytonctl hash-token <token>` | Hash for keys.json |

### Create flags (common)

| Flag | Default | Description |
|------|---------|-------------|
| `--image` | `windows-server-2025` | Catalog image ID |
| `--cpu` | 4 | vCPUs |
| `--memory` | 8192 | RAM MiB |
| `--disk` | 80 | Boot disk GiB |
| `--ttl` | 0 | Auto-delete after N minutes |
| `--dockur-*` | | Full dockur options (lab provider) |

---

## UI reference

| Page | Actions |
|------|---------|
| **Overview** | Project stats, health, quick links |
| **Machines** | Create, filter, start/stop, delete |
| **Machine detail** | Console (noVNC), Copy RDP, Dockur options summary, snapshots, events |
| **Images** | Catalog, golden image factory (when enabled) |
| **Activity** | Event timeline |
| **Settings** | Auth mode, storage class, Atlas URL, CORS |

**Light/dark** appearance toggles in the rail. Session token stored in browser for apikey mode.

---

## Troubleshooting

| Symptom | Check |
|---------|-------|
| Create fails immediately | `krytonctl doctor` — KVM, docker, kubeconfig |
| UI shows unauthorized | Set `KRYTON_TOKEN` or enable lab auto-auth |
| Console blank / iframe error | v1.1.0+ embeds noVNC; check CSP on reverse proxy |
| KubeVirt VM stuck provisioning | `kubectl get vm,vmi,dv` in target namespace; CDI DataSource ready? |
| Snapshots fail | StorageClass must support VolumeSnapshot — see [STORAGE.md](STORAGE.md) |
| Lab host hung | `./scripts/lab-recover.sh` then restart services |

---

## Suite context

| Product | Use with Kryton when… |
|---------|----------------------|
| [Veyron](https://zyvor.dev/veyron) | Fleet templates + GitOps CRDs at scale |
| [Zeus OS](https://zyvor.dev/zeus-os) | Visual day-2 operator workspace |
| [Axiom](https://zyvor.dev/axiom) | Private cloud console calling Kryton API |
| [HyperCluster](https://zyvor.dev/hypercluster) | Bootstrap the target cluster first |

Marketing: [zyvor.dev/kryton](https://zyvor.dev/kryton) · [Blog index](https://zyvor.dev/blog/introducing-kryton)

---

## What Kryton does not do

- Ship Windows ISOs, keys, or activation  
- Replace Veyron template libraries or Zeus OS consoles  
- Expose raw KubeVirt YAML to callers — that stays inside the provider

Microsoft licensing remains **your responsibility**.
