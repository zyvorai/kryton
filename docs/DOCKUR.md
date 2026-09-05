# Dockur lab provider

Kryton can provision **real Windows guests** on a Linux host using [dockur/windows](https://github.com/dockur/windows) — the same engine behind [WinPodX](https://github.com/kernalix7/winpodx) — without a full KubeVirt cluster.

**Step-by-step lab workflow:** [USER-GUIDE.md § Lab operator](USER-GUIDE.md#2-lab-operator-dockur)

Use this provider for labs, demos, and developer workstations. For production estates, use the KubeVirt provider with Helm.

---

## Requirements

| Requirement | Notes |
|-------------|-------|
| Linux x86_64 | With hardware virtualization |
| `/dev/kvm` | KVM device accessible to your user |
| Docker or Podman | With Compose v2 (`docker compose` / `podman-compose`) |
| Disk space | ~32–64 GiB+ per Windows image |
| Network | Outbound HTTPS for dockur to fetch ISOs |

Run `krytonctl doctor` to validate all of the above before creating machines.

### Secure lab (recommended on shared hosts)

```bash
./scripts/ensure-api-keys.sh
cat ~/.kryton/lab.token          # show the bearer key
./scripts/harden-lab-services.sh
```

With `KRYTON_LAB_AUTO_AUTH=true` (set by `harden-lab-services.sh`), the UI **auto-connects** — no manual token paste. The bearer is read from `lab.token` via `GET /api/v1/lab/bootstrap` (lab-only; requires `KRYTON_ALLOW_INSECURE=true`).

Without auto-auth, open the UI login chapters and paste the token from `lab.token`.

For CLI/scripts: `export KRYTON_TOKEN=$(cat ~/.kryton/lab.token)`

Full guide (create, rotate, Helm, troubleshooting): [AUTH.md](AUTH.md) · [CUSTOMER.md](CUSTOMER.md).

---

## Enable

```bash
export KRYTON_PROVIDER=dockur
export KRYTON_AUTH_MODE=disabled          # lab only; use apikey on shared hosts
export KRYTON_DOCKUR_RUNTIME=docker       # or podman
export KRYTON_DOCKUR_PUBLIC_HOST=<host> # hostname/IP clients use for console + RDP
export KRYTON_DOCKUR_DATA_DIR=$HOME/.kryton/dockur

krytond
krytonctl doctor
```

### Environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `KRYTON_DOCKUR_RUNTIME` | `docker` | Container runtime (`docker` or `podman`) |
| `KRYTON_DOCKUR_DATA_DIR` | *(temp)* | Persistent compose + disk state |
| `KRYTON_DOCKUR_PUBLIC_HOST` | `127.0.0.1` | Hostname in `consoleUrl` and RDP hints |
| `KRYTON_DOCKUR_HTTP_BASE` | `18006` | First HTTP console port (incremented per VM) |
| `KRYTON_DOCKUR_RDP_BASE` | `13389` | First RDP host port (incremented per VM) |

---

## Image → VERSION map

Kryton image IDs map to dockur `VERSION` codes:

| Kryton image ID | dockur `VERSION` |
|-----------------|------------------|
| `windows-11-enterprise` | `11e` |
| `windows-11-pro` | `11` |
| `windows-11-ltsc` | `11l` |
| `windows-10-enterprise` | `10e` |
| `windows-10-pro` | `10` |
| `windows-10-ltsc` | `10l` |
| `windows-tiny11` | `tiny11` |
| `windows-tiny11-core` | `core11` |
| `windows-server-2025` | `2025` |
| `windows-server-2022` | `2022` |
| `windows-server-2019` | `2019` |
| `windows-server-2016` | `2016` |

Catalog entries carry `dockurVersion` so custom image files can override the built-in map.

---

## Create a machine

```bash
krytonctl create --image windows-11-enterprise --cpu 4 --memory 8192 \
  --dockur-username labuser --dockur-password 'ChangeMe!' \
  --dockur-language English --dockur-region en-US --dockur-audio \
  --dockur-extra-disks 32 lab-win01
krytonctl get <id>
```

While installing, the machine reports:

- `consoleUrl` — dockur web viewer (watch unattended setup)
- `progressPercent` — install progress when available
- `message` — current phase description
- `rdpHost` / `rdpPort` / `rdpUsername` — RDP endpoint (default user `Docker`)

Open the console URL in a browser. RDP is published on the host port starting at `KRYTON_DOCKUR_RDP_BASE`.

Default guest credentials (override with `dockur` on create): **Docker** / **admin**.

### Dockur options (`spec.dockur`)

| Field | dockur env / volume | Notes |
|-------|---------------------|-------|
| `username` / `password` | `USERNAME` / `PASSWORD` | Password is write-only (redacted on GET) |
| `hostname` | `HOST` | Defaults to machine name |
| `language` / `region` / `keyboard` | `LANGUAGE` / `REGION` / `KEYBOARD` | Locale |
| `productKey` | `KEY` | Activation key |
| `domain` / `domainOu` | `DOMAIN` / `DOMAIN_OU` | AD join during install |
| `sharedDir` | host path → `/shared` | Desktop **Shared** / drive `Z:` |
| `oemDir` | host path → `/oem` | Runs `install.bat` post-setup |
| `command` | `COMMAND` | Single post-install command |
| `customIso` | URL as `VERSION`, or file → `/custom.iso` | Bring-your-own media |
| `edition` | `EDITION` | e.g. `core` for Server Core |
| `audio` | `AUDIO=Y` | Web viewer audio |
| `secureBoot` | `BOOT_MODE=windows_secure` + `TPM=Y` | Win11-style secure guest |
| `extraDisksGiB` | `DISK2_SIZE`… + `/storage2`… | Up to 3 extra disks |
| `autologin` | `AUTOLOGIN=N` when `false` | Default dockur autologin stays on |

### API example

```bash
curl -s -X POST http://localhost:8080/api/v1/machines \
  -H 'Content-Type: application/json' \
  -d '{
    "project": "default",
    "name": "lab-win01",
    "image": "windows-11-enterprise",
    "compute": {"cpu": 4, "memoryMiB": 8192},
    "disk": {"sizeGiB": 80},
    "dockur": {
      "username": "labuser",
      "password": "ChangeMe!",
      "language": "English",
      "sharedDir": "/home/lab/shared/win01",
      "audio": true
    }
  }' | jq .
```

---

## Lifecycle

Start, stop, and delete work through the standard Kryton API:

```bash
krytonctl stop <id>
krytonctl start <id>
krytonctl delete <id>
```

Compose projects live under `KRYTON_DOCKUR_DATA_DIR`. Deleting a machine removes its compose stack.

---

## Events

Dockur provisioning emits `io.kryton.machine.install.started` when unattended setup begins, in addition to the standard create/start/stop/delete events.

---

## Doctor checks

`krytonctl doctor` (or `GET /api/v1/doctor`) validates:

- Auth mode appropriateness for a non-demo provider
- Project configuration
- Image catalog
- Provider health
- Container runtime binary (`docker` / `podman`)
- Compose availability
- `/dev/kvm` access
- Data directory writability

---

## Security notes

- The dockur provider is **lab-oriented**. On shared hosts, set `KRYTON_AUTH_MODE=apikey`.
- Console and RDP ports are published on the host — restrict with firewall rules.
- Kryton does not ship Microsoft ISOs; dockur downloads media when you opt into this provider.

---

## What we deliberately did not copy

- FreeRDP RemoteApp / Linux `.desktop` integration (WinPodX product surface)
- Reverse-open of Linux apps inside Windows
- Shipping Microsoft ISOs from Kryton itself
