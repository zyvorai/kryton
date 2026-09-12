---
hero:
  eyebrow: WINDOWS VIRTUALIZATION CONTROL PLANE
  title: Kryton
  lead: >-
    One stable REST + CloudEvents machine API in front of interchangeable
    backends — demo, dockur, and KubeVirt stay behind the provider boundary.
  swatches:
    - {label: "REST + CloudEvents"}
    - {label: "Go 1.23+"}
    - {label: "Apache-2.0"}
  highlights:
    - {value: "3", label: "Interchangeable providers behind one machine API — demo, dockur, KubeVirt", footnote: "1"}
    - {value: "1", label: "krytond replica supported today — single-writer TTL reconciler and in-process event bus", footnote: "2"}
    - {value: "3", label: "RBAC roles — viewer, operator, admin — always scoped to project", footnote: "3"}
    - {value: "Apache-2.0", label: "Open-source license — no Windows media or keys shipped"}
  hub_bands:
    - {icon: "❓", title: "FAQ", description: "Questions people evaluating Kryton actually ask, before they've decided to adopt it.", href: "FAQ.md"}
    - {icon: "📘", title: "User guide", description: "Pick the path that matches your role — evaluator, lab operator, production operator, or integrator.", href: "USER-GUIDE.md"}
    - {icon: "🪟", title: "KubeVirt Windows VMs", description: "Provision Windows 11 on Kubernetes via KubeVirt — callers never touch VirtualMachine YAML.", href: "KUBEVIRT.md"}
    - {icon: "🧩", title: "Architecture", description: "One stable machine API; providers translate it into demo state, dockur compose stacks, or KubeVirt VirtualMachines.", href: "ARCHITECTURE.md"}
    - {icon: "✅", title: "GA path", description: "The KubeVirt provider is the GA path; dockur remains a lab installer, not GA.", href: "GA.md"}
footnotes:
  - {marker: "1", text: "Three providers translate one contract: demo (in-memory), dockur (real Windows via dockur/windows), and kubevirt (production Kubernetes VMs via operator-managed golden images).", href: "ARCHITECTURE.md#take-a-closer-look", href_label: "See Architecture — Take a closer look."}
  - {marker: "2", text: "internal/reconciler/ttl.go has no leader election and internal/events' history/SSE bus is in-process, so only one krytond replica is safe today; the Helm chart pins replicaCount: 1.", href: "ARCHITECTURE.md#single-replica-today", href_label: "See Architecture — Single-replica today."}
  - {marker: "3", text: "Roles viewer/operator/admin are always intersected with project membership.", href: "ARCHITECTURE.md#projects", href_label: "See Architecture — Projects."}
---

Kryton is a small, open-source (Apache-2.0) control-plane API for Windows
workloads — one stable REST+CloudEvents contract in front of interchangeable
backends (demo, dockur, KubeVirt). It is not a Windows installer or
activation service, not a raw KubeVirt YAML factory, and not a full desktop
virtualization/VDI product in its own right.

For the full project overview — quick start, install, project layout, and
the "how to use Kryton" walkthrough — see the
[README on GitHub](https://github.com/zyvorai/kryton).

## What's included today

<div class="icon-badge-list" markdown="1">

- 🆔 Stable UUIDs, independent of namespace or provider name
- 🔐 API keys or trusted reverse-proxy auth, secure by default
- ♻️ Day-2 ops: start / stop / snapshot / TTL expiry, SSE + webhooks
- 🩺 `krytonctl doctor` + `/api/v1/doctor` diagnostics
- 🖥️ Operator dashboard with collapsible rail (light/dark)
- 📜 Apache-2.0 — no Windows media or keys shipped

</div>

- **[Documentation index](https://github.com/zyvorai/kryton/blob/main/docs/README.md)** — the full "I want to… → read" map, plus scripts reference
- **[Authentication](AUTH.md)** — get and use the API key

Troubleshooting is covered inline in the relevant guide rather than a
separate document — see the "Troubleshooting" sections of
[User guide](USER-GUIDE.md#troubleshooting),
[Authentication](AUTH.md#troubleshooting), and
[KubeVirt](KUBEVIRT.md#troubleshooting).
