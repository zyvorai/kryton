# Kryton

**Windows virtualization control plane.**

Kryton is a small, open-source (Apache-2.0) control-plane API for Windows
workloads — one stable REST+CloudEvents contract in front of interchangeable
backends (demo, dockur, KubeVirt). It is not a Windows installer or
activation service, not a raw KubeVirt YAML factory, and not a full desktop
virtualization/VDI product in its own right. One stable machine API;
Kubernetes, KubeVirt, and dockur stay behind the provider boundary.

For the full project overview — quick start, install, project layout, and
the "how to use Kryton" walkthrough — see the
[README on GitHub](https://github.com/zyvorai/kryton).

## Start here

- **[FAQ](FAQ.md)** — licensing, support, and production-readiness questions
- **[User guide](USER-GUIDE.md)** — pick your path: evaluator, lab operator, production operator, or integrator
- **[Documentation index](https://github.com/zyvorai/kryton/blob/main/docs/README.md)** — the full "I want to… → read" map, plus scripts reference
- **[Authentication](AUTH.md)** — get and use the API key
- **[Architecture](ARCHITECTURE.md)** — the provider boundary, stable identity, and single-replica model
- **[KubeVirt Windows VMs](KUBEVIRT.md)** — the production path
- **[GA / go-live checklist](GA.md)** — what "production-ready" means today, stated honestly

Troubleshooting is covered inline in the relevant guide rather than a
separate document — see the "Troubleshooting" sections of
[User guide](USER-GUIDE.md#troubleshooting),
[Authentication](AUTH.md#troubleshooting), and
[KubeVirt](KUBEVIRT.md#troubleshooting).
