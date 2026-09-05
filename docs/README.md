# Kryton documentation

Start here, then drill into the guide that matches your role.

| I want to… | Read |
|------------|------|
| **Understand what Kryton is and pick a path** | [USER-GUIDE.md](USER-GUIDE.md) · [ARCHITECTURE.md](ARCHITECTURE.md) |
| **Get / rotate the API key and sign in** | [AUTH.md](AUTH.md) |
| **Try it locally in 5 minutes (demo)** | [USER-GUIDE.md § Evaluator](USER-GUIDE.md#1-evaluator-local-demo) · [README](../README.md#quick-start) |
| **Run real Windows in a lab (dockur)** | [USER-GUIDE.md § Lab operator](USER-GUIDE.md#2-lab-operator-dockur) · [DOCKUR.md](DOCKUR.md) |
| **Deploy to a remote Linux host over SSH** | [DEPLOY-REMOTE.md](DEPLOY-REMOTE.md) |
| **Production Windows on Kubernetes (KubeVirt)** | [USER-GUIDE.md § Production](USER-GUIDE.md#3-production-operator-kubevirt) · [KUBEVIRT.md](KUBEVIRT.md) · [CUSTOMER.md](CUSTOMER.md) |
| **Build golden images + CDI bootstrap** | [GOLDEN-IMAGES.md](GOLDEN-IMAGES.md) |
| **Integrate portals / CI via HTTP** | [USER-GUIDE.md § Integrator](USER-GUIDE.md#4-integrator-api--automation) · [API.md](API.md) |
| **Storage (Rook Ceph / Longhorn / snapshots)** | [STORAGE.md](STORAGE.md) |
| **Helm install + customer values** | [DEPLOYMENT.md](DEPLOYMENT.md) |
| **Atlas storage integration** | [ATLAS.md](ATLAS.md) |
| **Go-live checklist** | [GA.md](GA.md) · [CUSTOMER.md](CUSTOMER.md) |
| **Report a security issue** | [SECURITY.md](../SECURITY.md) |
| **Contribute code** | [CONTRIBUTING.md](../CONTRIBUTING.md) |
| **See what changed between versions** | [CHANGELOG.md](../CHANGELOG.md) · [RELEASE_NOTES.md](../RELEASE_NOTES.md) |
| **Configure the Helm chart** | [deploy/helm/kryton/README.md](../deploy/helm/kryton/README.md) |

## Product site

- [zyvor.dev/kryton](https://zyvor.dev/kryton) — product page  
- [zyvor.dev/docs/kryton](https://zyvor.dev/docs/kryton) — suite docs  
- [Blog: Introducing Kryton](https://zyvor.dev/blog/introducing-kryton)

## Scripts

| Script | Purpose |
|--------|---------|
| `scripts/deploy-remote.sh` | SSH/rsync install (demo unit) |
| `scripts/ensure-api-keys.sh` | Create `~/.kryton/lab.token` + hashed `keys.json` |
| `scripts/harden-lab-services.sh` | apikey + systemd for dockur/kubevirt lab |
| `scripts/lab-recover.sh` | Free hung ports before long jobs |
| `scripts/build-golden-image.sh` | dockur → Sysprep → golden qcow2 |
| `scripts/bootstrap-kubevirt-images.sh` | CDI DataSource bootstrap |
| `scripts/setup-kubevirt.sh` | Bootstrap + API + smoke VM |
| `scripts/setup-kubevirt-production.sh` | Full production chain |
| `scripts/add-license-headers.sh` | Apply Apache headers (maintainers) |
