# Customer readiness

What “done” means for external deployments.

## Production (KubeVirt on Kubernetes)

Follow [GA.md](GA.md) end-to-end:

1. KubeVirt + CDI + snapshot-capable CSI (Rook Ceph or Longhorn).
2. Sysprepped golden qcow2 → CDI `DataSource` per catalog ID ([GOLDEN-IMAGES.md](GOLDEN-IMAGES.md)).
3. Helm with `values-customer.yaml` (or `values-rook-ceph.yaml` + `auth.mode: apikey`).
4. `krytonctl doctor` — no `fail` findings.
5. Create → snapshot → restore → delete lifecycle smoke test.

```bash
helm upgrade --install kryton ./deploy/helm/kryton -n kryton --create-namespace \
  -f deploy/helm/kryton/values-customer.yaml
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
