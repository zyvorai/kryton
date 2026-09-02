# Security policy

## Reporting a vulnerability

**Do not open a public issue for a suspected vulnerability.** Report it privately using [GitHub's private vulnerability reporting](https://github.com/zyvorai/kryton/security/advisories/new) (Security tab → **Report a vulnerability**) on this repository. This opens a private GitHub Security Advisory visible only to you and the maintainers, so the report, discussion, and fix can happen before any public disclosure.

If you don't have a GitHub account or the link above isn't reachable, open a regular issue asking a maintainer to contact you through another channel — without any vulnerability details in the issue itself.

We don't currently commit to a fixed response SLA; expect an initial acknowledgment within a few business days. Once a fix is ready, we'll coordinate a disclosure timeline with you before any public advisory or release notes go out.

## Supported versions

Kryton does not yet maintain long-term-support branches. Security fixes land on `main` and the latest tagged release; older tags are not backported. Run the latest release, or track `main`, to get fixes.

## Design notes

Kryton intentionally does not provide a generic remote command endpoint, direct public RDP exposure, or Windows licensing bypass functionality.

Production operators should use authentication, TLS at the application or ingress layer, least-privilege Kubernetes RBAC, NetworkPolicy, and administrator-managed Windows image sources.
