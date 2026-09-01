# Kryton architecture

Kryton is deliberately split at the provider boundary.

```text
Consumers
  Veyron / Zeus / Transiva / CI / portals / third parties
                         |
                 REST + CloudEvents
                         |
                     Kryton API
                         |
             provider.Provider contract
                         |
             +-----------+-----------+
             |                       |
          demo                    KubeVirt
                                     |
                             Kubernetes REST API
                                     |
                               QEMU / KVM VMs
```

## Stable identity

A Kryton machine receives a UUID independent from its provider name. The KubeVirt provider records the UUID and project in labels and preserves the original Kryton specification in a managed annotation. External clients therefore never need to address `namespace/name` directly.

## Source of truth

For the KubeVirt provider, Kubernetes is the source of truth. Kryton is stateless with respect to machine inventory and can be restarted without losing machine identity. The demo provider is intentionally in-memory.

## Projects

Kryton projects map to provider isolation domains. In the KubeVirt provider a project maps to a Kubernetes namespace, optionally prefixed by `KRYTON_NAMESPACE_PREFIX`.

## Events

Lifecycle events use the CloudEvents structured envelope. They are available through the REST history endpoint, an authenticated SSE stream, and an optional webhook sink.

## Security boundaries

Kryton does not expose raw QEMU, RDP, WinRM, or arbitrary PowerShell execution. Authentication supports API keys for service-to-service use or trusted identity headers from an authenticated reverse proxy. Production KubeVirt mode refuses to start with authentication disabled unless the operator explicitly opts into insecure operation.
