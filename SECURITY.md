# Security Policy

StackMedic interacts with developer environments and potentially sensitive diagnostics, so security is a project requirement—not an optional feature.

## Supported versions

StackMedic is currently **pre-alpha** and does not yet have a supported stable release. The maintainers will publish a supported-version policy when the first release is available.

## Reporting a vulnerability

**Do not disclose suspected vulnerabilities, exploit payloads, credentials, or private environment data in public GitHub issues or discussions.**

1. Use GitHub's **Report a vulnerability** feature in the repository's **Security and quality → Advisories** area, if private vulnerability reporting is enabled.
2. If private reporting is not enabled, contact the maintainers through a non-public channel listed on the repository or maintainer profile. If none is available, open a **non-sensitive** issue requesting a private security contact method; do not include vulnerability details.
3. Include the affected revision or version, the impact, minimal reproduction steps, and any evidence that is safe to share.

Maintainers should acknowledge reports and coordinate a fix and disclosure through a private channel. Response times and disclosure deadlines will be established after the project has an active maintenance team.

## Security design principles

- Read-only diagnostics by default; never silently change or restart workloads.
- Least-privilege access to Docker, Kubernetes, CI, and other integrations.
- Treat container logs, repository contents, and tool outputs as **untrusted input**.
- Redact secrets and sensitive values from exported reports by default.
- Never send diagnostics to external AI services without explicit user authorization.
- Require deliberate approval and post-action verification before introducing automated remediation.

For secure contributor practices, see [CONTRIBUTING.md](CONTRIBUTING.md).
