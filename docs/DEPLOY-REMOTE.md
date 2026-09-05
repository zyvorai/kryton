# Remote deployment

Deploy Kryton to a Linux host over SSH — same workflow style as GuestKit: **rsync → build → install → verify**.

Ideal for lab control planes, integration testing, and staging before Helm/KubeVirt production.

---

## Quick start

```bash
# SSH key (recommended)
./scripts/deploy-remote.sh <user>@<host> --key

# Or via Makefile
make deploy-remote H=<host> U=<user> ARGS='--key'

# Fast iterate (skip Go install when already present)
./scripts/deploy-remote.sh <user>@<host> --quick --key
```

After a successful deploy the demo control plane listens on port **8080** (override with `KRYTON_PORT`):

```text
http://<host>:8080/
```

Default unit uses **`KRYTON_AUTH_MODE=disabled`**. For a shared host, turn on API keys before exposing the port:

```bash
ssh <user>@<host>
cd ~/.deployments/kryton
./scripts/ensure-api-keys.sh
cat ~/.kryton/lab.token    # paste into the UI Sign in chapter, or export KRYTON_TOKEN
```

How to create, rotate, and use keys (browser + CLI + Helm): **[AUTH.md](AUTH.md)**.

If the port is firewalled, tunnel:

```bash
ssh -L 8080:127.0.0.1:8080 <user>@<host>
open http://localhost:8080
```

Verify:

```bash
curl -s http://<host>:8080/readyz
ssh <user>@<host> krytonctl doctor
```

---

## Profiles

| Profile | Flags | What it does |
|---------|-------|----------------|
| Full | *(default)* | rsync → install Go if needed → `go build` → `/usr/local/bin` → systemd `kryton.service` (demo provider) |
| Quick | `--quick` | rsync → build on remote (skip Go install when `go` exists) |
| Quick + local binary | `--quick --build-local` | build on Linux laptop, rsync `bin/krytond` + `bin/krytonctl` only |
| Preflight | `--preflight-only` | SSH, disk, sudo checks |
| Verify | `--verify-only` | hit `/readyz` |
| Binaries only | `--no-service` | install CLIs/daemon without enabling systemd |
| Uninstall | `--uninstall` | stop unit, remove binaries and staging dir |

---

## What gets installed

| Path | Purpose |
|------|---------|
| `/usr/local/bin/krytond` | Control plane |
| `/usr/local/bin/krytonctl` | CLI |
| `/etc/systemd/system/kryton.service` | Demo unit (`KRYTON_PROVIDER=demo`, auth disabled) |
| `~/.deployments/kryton` | Remote source checkout (override with `DEPLOY_DIR`) |

The systemd unit is intentionally a **lab demo** (auth disabled). For a shared lab, enable apikey auth — see [AUTH.md](AUTH.md). For production KubeVirt, use Helm (`deploy/helm/kryton`) with API-key auth — see [DEPLOYMENT.md](DEPLOYMENT.md).

### Switching to dockur after deploy

Edit the systemd unit or run manually:

```bash
export KRYTON_PROVIDER=dockur
export KRYTON_DOCKUR_RUNTIME=docker
export KRYTON_DOCKUR_PUBLIC_HOST=<host-ip>
krytond
```

See [DOCKUR.md](DOCKUR.md).

---

## Requirements

**Local:** `ssh`, `rsync`, optional `sshpass` for password auth (deprecated).

**Remote:** Linux x86_64 or arm64. Non-root users need passwordless `sudo` for install into `/usr/local/bin` and systemd.

---

## Production (Helm) after SSH staging

Once you have a cluster with KubeVirt + CDI:

```bash
kubectl -n kryton create secret generic kryton-auth --from-file=keys.json
helm upgrade --install kryton ./deploy/helm/kryton -n kryton --create-namespace
```

---

## Logs

Set `KRYTON_DEPLOY_LOG` to capture a timestamped log under `~/.kryton/`:

```bash
KRYTON_DEPLOY_LOG=~/deploy.log ./scripts/deploy-remote.sh <user>@<host> --key
```
