# Changelog

All notable changes to tunnd are documented in this file.

The format is loosely based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
Goreleaser also auto-generates a per-release changelog from commit messages
on tag; this file captures hand-written notes for the changes that warrant
extra context (breaking changes, migration notes, wire-protocol compatibility).

## [Unreleased]

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
