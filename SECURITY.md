# Security policy

Please do not open public issues for suspected vulnerabilities. Report security issues privately to the project maintainers through the security contact configured for the repository.

Kryton intentionally does not provide a generic remote command endpoint, direct public RDP exposure, or Windows licensing bypass functionality.

Production operators should use authentication, TLS at the application or ingress layer, least-privilege Kubernetes RBAC, NetworkPolicy, and administrator-managed Windows image sources.
