# Changelog

All notable changes to tunnd are documented in this file.

The format is loosely based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
Goreleaser also auto-generates a per-release changelog from commit messages
on tag; this file captures hand-written notes for the changes that warrant
extra context (breaking changes, migration notes, wire-protocol compatibility).

## [Unreleased]

## [0.2.1] - Pending

Onboarding and reliability fixes that make the one-command VPS setup work exactly as documented, plus a convenience command for WebSocket apps.

### Fixed

- **One-command setup prints a usable token.** `setup.sh` now extracts only the token value (it previously captured two lines) and creates the token as the `tunnd` user, so the database and its WAL/SHM files aren't left owned by root.
- **Admin dashboard reachable over HTTPS.** The dashboard is now served on the base domain (`https://tunnd.yourdomain.com`) in the standalone deploy, matching the docs — not only on the plain-HTTP admin port. The admin port still works for reverse-proxy / LAN access.
- **Session cookie is marked `Secure`** on HTTPS requests (direct TLS or `X-Forwarded-Proto: https`), so it is never sent in cleartext on secured deployments.
- **Caddy compose works as written.** `docker-compose.caddy.yml` now runs tunnd in plain-HTTP mode behind Caddy (it previously failed validation by defaulting to port 443 with no TLS).

### Added

- **`max_tunnels_per_token` is now enforced.** A per-token `max_tunnels` overrides the server-wide default; `0` means unlimited. Clients get a clear `tunnel_limit_reached` message.
- **Client honors `TUNND_SERVER_ADDR` / `TUNND_TOKEN`** (plus inspector port and log level) environment variables, so the client runs without `tunnd setup` — useful for CI, containers, and the export hints printed by `setup.sh`.
- **`tunnd ws <port>` command.** A convenience alias of `tunnd http` for WebSocket apps: same transport and flags, but it prints a copy-paste-ready `wss://` URL. HTTP on the same port keeps working, so mixed REST + WebSocket apps are fully supported. No wire-protocol change.

### Wire compatibility

No protocol changes. `tunnd ws` registers as an HTTP tunnel on the wire, so all client/server version combinations continue to interoperate.

## [0.2.0] - Released

### Tunnels just work for any local dev server

`tunnd http <port>` now reliably exposes any local dev server — Vite, Next.js, webpack, Bun, Deno, Express, FastAPI, Rails, you name it — out of the box. No framework configuration, no `allowedHosts` edits, no flags required.

Under the hood:

- **Reaches the upstream on every platform.** Dialing now uses Go's dual-stack resolver against `localhost:<port>`, so IPv6-only listeners (the default for many tools on Windows) connect on the first try.
- **Lets dev servers see their expected Host.** The public Host (`<sub>.your-domain`) is rewritten to `localhost:<port>` before forwarding, so frameworks that pin `allowedHosts` accept the request without configuration.
- **Auto-detects HTTPS upstreams.** `tunnd http 3000` works whether your dev server speaks HTTP or HTTPS (`vite --https`, `next dev --experimental-https`). No flag needed.
- **Streaming responses survive.** Server-Sent Events, long-polls, and large downloads no longer truncate at the 120-second mark.
- **Real client info reaches your app.** `X-Forwarded-For`, `X-Forwarded-Proto`, and `X-Forwarded-Host` are populated on every request.
- **Clearer error when the dev server isn't running.** "no service listening on port X — is your dev server running?" replaces the cryptic OS-level dial error.

### Power-user flags

For unusual setups (multi-tenant routing, strict TLS verification, etc.) — see [the CLI reference](https://elvonpiko.github.io/tunnd/configuration/cli-reference/):

- `--host-header` to control how the Host header is forwarded
- `--upstream-scheme` to force HTTP or HTTPS instead of auto-detect
- `--upstream-tls-skip-verify` to skip cert verification on the upstream

### Wire compatibility

Existing clients and servers continue to interoperate. `RegisterPayload` gained two additive `omitempty` JSON fields; old/new combinations behave correctly.

## [0.1.2] - Released

See the [v0.1.2 release notes](https://github.com/elvonpiko/tunnd/releases/tag/v0.1.2) for the auto-generated commit-grouped changelog.

## [0.1.1] - Released

See the [v0.1.1 release notes](https://github.com/elvonpiko/tunnd/releases/tag/v0.1.1).

## [0.1.0] - Released

Initial public release. See the [v0.1.0 release notes](https://github.com/elvonpiko/tunnd/releases/tag/v0.1.0).
