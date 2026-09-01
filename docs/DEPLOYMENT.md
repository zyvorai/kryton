# Production deployment

For a lab host over SSH (demo provider + systemd), see **[DEPLOY-REMOTE.md](DEPLOY-REMOTE.md)**.

## Prerequisites

- Kubernetes with KubeVirt installed.
- CDI available for image-backed DataVolume templates.
- `DataSource` objects for each configured Kryton image ID in the configured image namespace.
- One Kubernetes namespace per Kryton project, or namespaces using the configured prefix.
- RBAC allowing the Kryton service account to manage KubeVirt VMs and snapshots in those namespaces.

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

## Authentication

For machine-to-machine access, use `KRYTON_AUTH_MODE=apikey` and mount a JSON key file from a Kubernetes Secret. Store SHA-256 token digests rather than raw tokens.

For browser SSO, place Kryton behind an authenticated reverse proxy and use `KRYTON_AUTH_MODE=proxy`. The proxy must strip inbound `X-Kryton-*` headers, set trusted identity headers itself, and send the shared `X-Kryton-Proxy-Secret` value loaded by Kryton from `KRYTON_PROXY_SECRET_FILE`. Do not expose the Kryton service directly when proxy auth is enabled.

## TLS

Kryton can terminate TLS directly with `KRYTON_TLS_CERT_FILE` and `KRYTON_TLS_KEY_FILE`. If `KRYTON_CLIENT_CA_FILE` is configured, the server additionally requires and verifies client certificates.

## Network attachments

When `network.networkId` is empty, the KubeVirt provider creates a pod network with masquerade binding. When it is set, the value is passed as a Multus network attachment name and Kryton uses bridge binding.

## TTL

`ttlMinutes` is persisted as a provider annotation. The TTL reconciler periodically lists machines and deletes expired workloads. This remains effective across Kryton restarts.
