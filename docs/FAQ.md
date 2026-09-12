---
hero:
  eyebrow: FAQ
  title: FAQ
---

Questions people evaluating Kryton actually ask, before they've decided to
adopt it. Already decided? [`docs/USER-GUIDE.md`](USER-GUIDE.md) is a
better starting point.

## Licensing & cost

**Is it really free?** Yes. Apache-2.0 — use, modify, and run it for
personal, lab, and commercial production use at no charge, subject to
preserving notices (see [`NOTICE`](https://github.com/zyvorai/kryton/blob/main/NOTICE)). No Windows media or license
keys are shipped — that stays the operator's responsibility. See the
README's [License](https://github.com/zyvorai/kryton#license) section.

**What does "Enterprise" mean here?** Production support, SLAs, and
Zyvor's other commercial products are licensed separately. Contact
sales@zyvor.dev. Nothing in this repository requires it.

## Support

**What if I find a bug?** Open a GitHub issue.

**What if I find a security vulnerability?** See [`SECURITY.md`](https://github.com/zyvorai/kryton/blob/main/SECURITY.md)
— private reporting via GitHub Security Advisories. Note its own text:
"Kryton does not yet maintain long-term-support branches. Security fixes
land on `main` and the latest tagged release; older tags are not
backported."

## Production readiness

**Is this production-ready?** It depends which provider. Per
[`docs/GA.md`](GA.md): the **KubeVirt provider is the GA path**. The
**`dockur` lab provider "remains a lab installer, not GA."** Also
explicitly out of GA scope today: running more than one `krytond` replica —
[`docs/ARCHITECTURE.md`](ARCHITECTURE.md) documents a single-writer TTL
reconciler and in-process event/SSE bus, so multi-replica isn't safe yet.
Read `docs/GA.md`'s full checklist before a production go-live decision.

**What Kryton explicitly doesn't do**: it's not a Windows installer,
activation service, or media distributor, and not a raw KubeVirt YAML
factory for callers — see the README's "What Kryton is not" section.

## Architecture

**How does the provider abstraction work?** One Go `provider.Provider`
interface; callers only ever see REST+CloudEvents, never backend-specific
detail. Three providers translate the same contract: `demo` (in-memory),
`dockur` (real Windows via dockur/windows on Docker/Podman+KVM, lab-grade),
and `kubevirt` (production Kubernetes VMs via operator-managed golden
images). See [`docs/ARCHITECTURE.md`](ARCHITECTURE.md).

**Does it integrate with other Zyvor products?** Yes — see
[`docs/ATLAS.md`](ATLAS.md) for the Zyvor Atlas storage control plane
integration.

## Hardware & platform

**What do I need to run it?** Go 1.23+; for the `dockur` provider,
Docker/Podman with KVM; for the `kubevirt` provider, a Kubernetes cluster
with KubeVirt and CDI installed (see [`docs/DEPLOYMENT.md`](DEPLOYMENT.md)
and [`docs/STORAGE.md`](STORAGE.md) for storage-class requirements).

**Windows licensing?** Not Kryton's concern by design — "Microsoft media,
activation, and entitlement remain the operator's responsibility" (README).
