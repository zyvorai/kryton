# Production deployment

For a lab host over SSH (demo or dockur provider + systemd), see **[DEPLOY-REMOTE.md](DEPLOY-REMOTE.md)** and **[DOCKUR.md](DOCKUR.md)**.

This guide covers the **KubeVirt** production path with Helm.

---

## Prerequisites

- Kubernetes with KubeVirt installed.
- CDI available for image-backed DataVolume templates.
- `DataSource` objects for each configured Kryton image ID in the configured image namespace.
- One Kubernetes namespace per Kryton project, or namespaces using the configured prefix.
- RBAC allowing the Kryton service account to manage KubeVirt VMs and snapshots in those namespaces.

Validate your estate before go-live:

```bash
krytonctl doctor   # after pointing KRYTON_PROVIDER=kubevirt at the cluster
```

---

## Image contract

Kryton does not distribute Microsoft installation media or licenses. An image catalog entry is a logical reference. The KubeVirt provider expects a CDI `DataSource` with the same ID.

Example:

```yaml
apiVersion: cdi.kubevirt.io/v1beta1
kind: DataSource
metadata:
  name: windows-server-2025
  namespace: kryton-images
spec:
  source:
    pvc:
      name: windows-server-2025-golden
      namespace: kryton-images
```

---

## Helm install

```bash
export KRYTON_PROVIDER=kubevirt
export KRYTON_PROJECTS=finance,engineering
export KRYTON_DEFAULT_PROJECT=finance
export KRYTON_IMAGE_NAMESPACE=kryton-images
export KRYTON_AUTH_MODE=apikey
export KRYTON_API_KEYS_FILE=/etc/kryton/keys.json

TOKEN=$(krytonctl generate-token)
krytonctl hash-token "$TOKEN"   # store only the hash in keys.json

kubectl -n kryton create secret generic kryton-auth --from-file=keys.json
helm upgrade --install kryton ./deploy/helm/kryton -n kryton --create-namespace
```

Image: `ghcr.io/zyvorai/kryton` (see `deploy/helm/kryton/values.yaml`).

---

## Authentication

For machine-to-machine access, use `KRYTON_AUTH_MODE=apikey` and mount a JSON key file from a Kubernetes Secret. Store SHA-256 token digests rather than raw tokens.

For browser SSO, place Kryton behind an authenticated reverse proxy and use `KRYTON_AUTH_MODE=proxy`. The proxy must strip inbound `X-Kryton-*` headers, set trusted identity headers itself, and send the shared `X-Kryton-Proxy-Secret` value loaded by Kryton from `KRYTON_PROXY_SECRET_FILE`. Do not expose the Kryton service directly when proxy auth is enabled.

---

## TLS

Kryton can terminate TLS directly with `KRYTON_TLS_CERT_FILE` and `KRYTON_TLS_KEY_FILE`. If `KRYTON_CLIENT_CA_FILE` is configured, the server additionally requires and verifies client certificates.

Alternatively, terminate TLS at an ingress controller and forward plain HTTP to Kryton inside the cluster.

---

## Network attachments

When `network.networkId` is empty, the KubeVirt provider creates a pod network with masquerade binding. When it is set, the value is passed as a Multus network attachment name and Kryton uses bridge binding.

---

## TTL

`ttlMinutes` is persisted as a provider annotation. The TTL reconciler periodically lists machines and deletes expired workloads. This remains effective across Kryton restarts.

---

## Provider comparison

| | demo | dockur | kubevirt |
|---|------|--------|----------|
| **Target** | Local eval | Lab Linux host | Production K8s |
| **Persistence** | In-memory | Compose on disk | Kubernetes etcd |
| **Auth default** | disabled | disabled (use apikey) | apikey required |
| **Real Windows** | No | Yes | Yes (golden images) |
