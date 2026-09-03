# Golden Windows images

Kryton ships **no Windows media**. Operators build sysprepped golden images and register them as CDI `DataSource` objects in `kryton-images`. The pipeline below is adapted from [WinForge](https://github.com/zyvorai/winforge) — dockur install → Sysprep → qcow2 capture → CDI bootstrap.

---

## Pipeline overview

```text
  dockur/windows          Sysprep + capture          CDI bootstrap
  (build-golden-image)  →  (FINALIZE=1)           →  (bootstrap-kubevirt-images)
        │                        │                            │
   install guest            golden.qcow2              DataSource + PVC
```

Three ways to get the image into the cluster:

| Mode | When to use |
|------|-------------|
| **Upload** (default) | You have a local qcow2 on the operator host |
| **HTTP import** | Image is published to object storage (S3, MinIO, etc.) |
| **Manual manifest** | GitOps / hand-crafted CDI YAML |

---

## 1. Build a golden image (automated)

Uses [dockur/windows](https://github.com/dockur/windows) for hands-free ISO download and install, plus Kryton's `deploy/dockur/oem/install.bat` for Sysprep generalization.

### Fully automated (CLI)

```bash
VERSION=11e ./scripts/build-golden-image.sh --auto
```

This will:

1. Start `dockurr/windows` with the OEM hook mounted
2. Download and install Windows unattended (watch at `http://<host>:8066/`)
3. Run Sysprep via `install.bat` when setup completes
4. Capture `out/windows-11e-golden.qcow2` automatically

### From the Kryton UI

Open **Images → Golden image factory** and click **Build golden image**. The dashboard shows a 5-step progress bar and embeds the dockur web viewer while Windows installs.

Requires docker + `/dev/kvm` on the `krytond` host (`capabilities.goldenImages: true`).

### Manual (interactive)

```bash
VERSION=11e ./scripts/build-golden-image.sh
# watch http://localhost:8066/ then:
BUILD_ID=... WORKDIR=... VERSION=11e FINALIZE=1 ./scripts/build-golden-image.sh
```

### API

```bash
curl -X POST http://127.0.0.1:9088/api/v1/golden \
  -H 'Content-Type: application/json' \
  -d '{"imageId":"windows-11-enterprise","auto":true}'

curl http://127.0.0.1:9088/api/v1/golden
```

### Boot-readiness gate (guestkit)

If [`guestkit`](https://github.com/zyvorai/guestkit) is on the build host's `PATH`,
`build-golden-image.sh` runs `guestkit gate` against the captured qcow2 right after
`qemu-img convert`/`check`, before the build is marked `ready`. This **flags, it
doesn't block**: a low score or a missing binary still lets the build reach
`ready` — it just records `certified`/`validationScore` on the `GoldenBuild` (and
on the catalog `Image` once bootstrapped) so operators see boot-readiness before
publishing to KubeVirt, instead of finding out at cutover.

```bash
# Install once per build host (Linux amd64):
curl -fsSL -o guestkit.tar.gz \
  https://github.com/zyvorai/guestkit/releases/latest/download/guestkit-linux-amd64.tar.gz
tar xzf guestkit.tar.gz && sudo install -m755 guestkit /usr/local/bin/
```

| Env var | Default | Meaning |
|---------|---------|---------|
| `GUESTKIT_BIN` | `guestkit` | Binary name/path to invoke |
| `GUESTKIT_MIN_SCORE` | `70` | `--fail-below` passed to `guestkit gate` |
| `SKIP_GUESTKIT` | `0` | Set `1` to skip the gate entirely on hosts without guestkit |

`guestkit gate` needs root to mount the disk via NBD/loop, so the FINALIZE step
runs it under `sudo` — make sure the build-host user has passwordless `sudo` (or
run the whole script as root, consistent with `scripts/e2e-kryton-golden-host.sh`'s
existing host-level access assumption). `krytonctl doctor` reports whether
`guestkit` is installed (informational, not a failure).

### Gotchas (lab-proven)

- **`FINALIZE` / `IMAGE_ID` must survive re-exec.** Parent env `FINALIZE=1` and an explicit
  `IMAGE_ID` must not be clobbered by script defaults (fixed in `build-golden-image.sh`).
  Otherwise “finalize” restarts install instead of capturing, or writes the wrong catalog ID.
- **One-minute `ready` is not a golden.** Real install + Sysprep is hours. A build that flips
  to `ready` immediately often captured a mid-Setup disk — VNC shows “Install Windows /
  restarted unexpectedly”, guest never answers ARP/RDP on `10.0.2.2`. With `guestkit` installed
  on the build host, this is now caught automatically: the gate's EFI/BCD checks (BOOT-012/013/014)
  flag `certified=false` on exactly this class of disk — see [Boot-readiness gate](#boot-readiness-gate-guestkit) above.
- **Progress stuck at ~58%** often means dockur is still installing (watch `:8066`); do not
  finalize early.
- **Podman** is supported for builds on hosts without Docker.
- **CDI PVC size / volumeMode:** virtual disks can be ~80 Gi — bootstrap with **≥96 Gi**.
  Match Filesystem vs Block between DataSource and clone targets or KubeVirt returns
  `IncompatibleVolumeModes`.
- **Catalog aliases:** do not point multiple image IDs at one qcow2 unless the content truly
  matches (e.g. `windows-11-pro` ≠ Tiny11 Core).

---

## 1b. Build details (dockur VERSION codes)

### Version → Kryton catalog ID

| `VERSION` (dockur) | Kryton image ID |
|--------------------|-----------------|
| `2025` | `windows-server-2025` |
| `2022` | `windows-server-2022` |
| `11e` | `windows-11-enterprise` |
| `11` | `windows-11-pro` |
| `10` | `windows-10-pro` |
| `2019` | `windows-server-2019` |

Override with `KRYTON_IMAGE_ID=my-custom-image`.

---

## 2. Bootstrap into Kubernetes (CDI)

### API (from a ready golden build)

On a kubevirt krytond with `kubectl`/`virtctl` and kubeconfig:

```bash
curl -X POST http://127.0.0.1:9088/api/v1/golden/<build-id>/bootstrap
```

The Images page **Publish to KubeVirt** button runs the same path: upload the captured qcow2 and create `DataSource` `<imageNamespace>/<imageId>`.

### Upload from local file

```bash
KRYTON_WINDOWS_IMAGE=./out/windows-2025-golden.qcow2 \
  KRYTON_IMAGE_ID=windows-server-2025 \
  ./scripts/bootstrap-kubevirt-images.sh
```

Or via Make:

```bash
make bootstrap-kubevirt IMAGE=./out/windows-2025-golden.qcow2 ID=windows-server-2025
```

### HTTP import (WinForge-style publish)

Upload the qcow2 to HTTPS-accessible storage, then:

```bash
KRYTON_IMAGE_URL=https://artifacts.example.com/windows-server-2025-golden.qcow2 \
  KRYTON_IMAGE_ID=windows-server-2025 \
  ./scripts/bootstrap-kubevirt-images.sh --http
```

See `deploy/kubevirt/datasource-http.yaml.example` for a hand-crafted manifest.

---

## 3. Full automated setup

Chain build → bootstrap → Kryton API → smoke-test VM:

```bash
# If you already have a golden qcow2:
make setup-kubevirt IMAGE=./out/windows-11-enterprise-golden.qcow2

# Build first, then setup (interactive — waits for Sysprep):
make build-golden VERSION=11e
# ... complete Sysprep in guest ...
make build-golden VERSION=11e FINALIZE=1
make setup-kubevirt IMAGE=./out/windows-11e-golden.qcow2
```

`setup-kubevirt.sh` also accepts `--http --url https://...` when the image is already published.

---

## Verify

```bash
kubectl -n kryton-images get datasource,pvc
krytonctl doctor   # should show all catalog images as ready
```

Then create a VM from the UI or API using the matching `image` ID.

---

## WinForge relationship

| WinForge | Kryton |
|----------|--------|
| `scripts/build-golden.sh` | `scripts/build-golden-image.sh` |
| `deploy/k8s/windows-image-datasource.yaml` | `deploy/kubevirt/datasource-http.yaml.example` |
| `winforged` API | **Not integrated** — use Kryton `kubevirt` provider |
| `windows-images` namespace | `kryton-images` (`KRYTON_IMAGE_NAMESPACE`) |

WinForge remains useful as a standalone image factory; Kryton consumes its output via CDI.
