# Customer readiness

What “done” means for external deployments.

**New users:** start with [USER-GUIDE.md](USER-GUIDE.md) for step-by-step lab and production workflows.

## Production (KubeVirt on Kubernetes)

Follow [GA.md](GA.md) end-to-end:

1. KubeVirt + CDI + snapshot-capable CSI (Rook Ceph or Longhorn).
2. Sysprepped golden qcow2 → CDI `DataSource` per catalog ID ([GOLDEN-IMAGES.md](GOLDEN-IMAGES.md)).
3. Helm with `values-customer.yaml` (or `values-rook-ceph.yaml` + `auth.mode: apikey`).
4. `krytonctl doctor` — no `fail` findings.
5. Create → snapshot → restore → delete lifecycle smoke test.

```bash
# On the lab/K8s host (needs docker+kvm, kubectl, virtctl, ~100GiB free):

# Option A — already have a sysprepped qcow2
KRYTON_WINDOWS_IMAGE=/path/to/windows11.qcow2 ./scripts/setup-kubevirt.sh

# Option B — build golden via dockur + bootstrap + VM (45–90 min for install)
./scripts/setup-kubevirt-production.sh --build-golden

# Option C — from your laptop (rsync + remote nohup)
make run-kubevirt-production-remote H=175.110.122.71 U=sus BUILD=1

# Option D — Helm customer profile (after golden exists on cluster)
./scripts/setup-kubevirt-production.sh --customer-helm --skip-create --image ./out/windows-11e-golden.qcow2
```

## Lab / eval (dockur)

Real Windows via [dockur/windows](https://github.com/dockur/windows) — **not GA**, suitable for labs and PoCs.

```bash
# On the Linux host after krytond is installed:
./scripts/ensure-api-keys.sh
./scripts/harden-lab-services.sh   # systemd + apikey on :7088 and :9088
```

See [DOCKUR.md](DOCKUR.md) for create options (UI, API, `krytonctl --dockur-*`).

**Never** run `KRYTON_AUTH_MODE=disabled` on internet-facing hosts.

## Release checklist (maintainers)

| Step | Command |
|------|---------|
| Tests | `make check` |
| Version | Chart `1.x.y` + tag `v1.x.y` |
| Notes | Update `RELEASE_NOTES.md` |
| Push | `git push origin main --tags` |
| Lab smoke | `KRYTON_TOKEN=… node scripts/dockur-ux-test.mjs` |

## Support matrix

| Profile | Provider | Auth | Use case |
|---------|----------|------|----------|
| Customer | `kubevirt` | `apikey` + TLS | Production private cloud |
| Lab secure | `dockur` / `kubevirt` | `apikey` | Shared lab hosts |
| Dev only | `demo` | `disabled` | Local UI hacking |
