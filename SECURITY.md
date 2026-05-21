# Security Policy

## Reporting a vulnerability

If you have found a security issue in Tunnd, please **do not open a public
issue or PR**. Instead, report it privately via [GitHub Security
Advisories](https://github.com/elvonpiko/tunnd/security/advisories/new).

We aim to acknowledge new reports within 72 hours and to provide an initial
assessment within one week. Once a fix is ready, we'll coordinate with you on
the disclosure timeline before it ships.

## Supported versions

Only the latest minor release receives security patches. Earlier releases are
not patched — please upgrade.

| Version       | Supported          |
| ------------- | ------------------ |
| latest minor  | ✅                 |
| anything else | ❌                 |

## Hardening notes for operators

- Run `tunnd-server` as an unprivileged system user. The provided systemd unit
  uses `User=tunnd`, `NoNewPrivileges`, `ProtectSystem=strict`, `ProtectHome`,
  `PrivateTmp`, `PrivateDevices`, and only the `CAP_NET_BIND_SERVICE`
  capability.
- Restrict the admin port (`9091` by default) to trusted IPs at the firewall.
  The dashboard does authenticate every request, but unnecessary exposure is
  unnecessary risk.
- Pick a strong admin password (12+ characters) on first run.
- Rotate auth tokens periodically. Revoking is one click in the dashboard.
- TLS is on by default — do not disable it for production deployments.
- Keep the binary up to date. Subscribe to release notifications on GitHub.

## Threat model

Tunnd's intended threat model:

- **In scope:** unauthenticated attackers on the public internet trying to
  bypass tunnel authentication, hijack tunnels, or escalate from the tunnel
  surface to the host.
- **Out of scope:** attackers with shell access on the VPS, malicious tunnel
  *clients* with valid tokens (token holders are trusted to forward whatever
  they like), and side-channel attacks against the underlying host or runtime.

If your use case lies outside this model (multi-tenant SaaS, untrusted token
holders, etc.) please reach out before deploying.
