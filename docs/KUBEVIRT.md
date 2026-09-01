# KubeVirt Windows VMs

Provision **Windows 11** (and other catalog images) on Kubernetes via KubeVirt. Kryton exposes the same REST API — callers never touch `VirtualMachine` YAML.

---

## Prerequisites

- Kubernetes cluster with **KubeVirt** and **CDI** installed
- `kubectl`, `virtctl`, and `qemu-img` on the operator host
- A **sysprepped Windows qcow2** golden image (operator-provided; Kryton does not ship media)

Build one with the WinForge-style pipeline — see **[GOLDEN-IMAGES.md](GOLDEN-IMAGES.md)**.

---

## Golden images (WinForge pipeline)

```bash
# 1. Build (dockur → Sysprep → capture)
VERSION=11e ./scripts/build-golden-image.sh
# ... Sysprep in guest ...
VERSION=11e FINALIZE=1 ./scripts/build-golden-image.sh

# 2. Bootstrap CDI DataSource
KRYTON_WINDOWS_IMAGE=./out/windows-11e-golden.qcow2 ./scripts/bootstrap-kubevirt-images.sh

# Or HTTP import if published to object storage:
KRYTON_IMAGE_URL=https://artifacts.example/win11.qcow2 ./scripts/bootstrap-kubevirt-images.sh --http
```

Full details: [GOLDEN-IMAGES.md](GOLDEN-IMAGES.md).

---

## Automated setup (recommended)

One command bootstraps the golden image, installs the Kryton API, and creates a Windows 11 VM:

```bash
export KRYTON_WINDOWS_IMAGE=/path/to/windows11.qcow2
./scripts/setup-kubevirt.sh
```

Or via Make:

```bash
make setup-kubevirt IMAGE=/path/to/windows11.qcow2
```

### What it does

1. Creates `kryton-images` namespace and **auto-provisions project namespaces**
2. Uploads your qcow2 via CDI and creates a `DataSource` (`windows-11-enterprise`)
3. Installs `krytond` as a **host systemd service** (`kryton-kubevirt.service`) using your kubeconfig
4. Calls `POST /api/v1/machines` and polls until the VM is running

### In-cluster API (Helm + NodePort)

```bash
make setup-kubevirt IMAGE=/path/to/windows11.qcow2 ARGS='--helm'
```

Uses `deploy/helm/kryton/values-lab.yaml` (auth disabled, NodePort **30088**).

---

## Bootstrap images only

If you only need CDI DataSources:

```bash
# Local upload
KRYTON_WINDOWS_IMAGE=/path/to/windows11.qcow2 ./scripts/bootstrap-kubevirt-images.sh

# HTTP import (published golden image)
KRYTON_IMAGE_URL=https://artifacts.example/win11.qcow2 ./scripts/bootstrap-kubevirt-images.sh --http
```

---

## Manual API usage

After setup, the API listens on `:9088` by default (host mode, auto-increments if busy) or NodePort `30088` (Helm lab).

```bash
export KRYTON_URL=http://127.0.0.1:9088

krytonctl doctor
krytonctl create --image windows-11-enterprise --cpu 4 --memory 8192 win11-01
krytonctl get <id>
krytonctl list
```

```bash
curl -s -X POST "${KRYTON_URL}/api/v1/machines" \
  -H 'Content-Type: application/json' \
  -d '{
    "project": "default",
    "name": "win11-01",
    "image": "windows-11-enterprise",
    "compute": {"cpu": 4, "memoryMiB": 8192},
    "disk": {"sizeGiB": 96}
  }' | jq .
```

---

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `KRYTON_PROVIDER` | `kubevirt` | Required for this path |
| `KRYTON_KUBECONFIG` | `~/.kube/config` | Kubernetes credentials (host mode) |
| `KRYTON_IMAGE_NAMESPACE` | `kryton-images` | CDI DataSource namespace |
| `KRYTON_PROJECTS` | `default` | Kryton projects → K8s namespaces |
| `KRYTON_AUTH_MODE` | `apikey` (prod) / `disabled` (lab) | API authentication |

| `KRYTON_EVENTS_FILE` | *(memory only)* | Durable JSONL event log path |
| `KRYTON_EVENT_WEBHOOK_SECRET` | *(none)* | Webhook HMAC signing secret |

`krytonctl doctor` verifies KubeVirt connectivity, project namespaces, and that every catalog image has a matching CDI `DataSource`.

### Console

KubeVirt machines expose `consoleUrl` when the guest instance is running. Open it in the dashboard or browser to launch an embedded noVNC session proxied through Kryton (`/api/v1/machines/{id}/console`).

---

## Production

For production estates use Helm with API-key auth — see [DEPLOYMENT.md](DEPLOYMENT.md).

```bash
helm upgrade --install kryton ./deploy/helm/kryton -n kryton --create-namespace \
  --set auth.mode=apikey \
  --set auth.existingSecret=kryton-auth
```

---

## Troubleshooting

| Symptom | Fix |
|---------|-----|
| `Missing DataSources` in doctor | Run `bootstrap-kubevirt-images.sh` |
| VM stuck `Provisioning` | `kubectl -n default get dv,pvc,vm` — check CDI import |
| `kubernetes config` error on start | Set `KRYTON_KUBECONFIG` or `KRYTON_KUBERNETES_ENDPOINT` |
| Overlay qcow2 upload fails | Script auto-flattens; ensure `qemu-img` is installed |
