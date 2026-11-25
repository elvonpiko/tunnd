# Changelog

All notable changes to tunnd are documented in this file.

The format is loosely based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
Goreleaser also auto-generates a per-release changelog from commit messages
on tag; this file captures hand-written notes for the changes that warrant
extra context (breaking changes, migration notes, wire-protocol compatibility).

## [0.3.1] - 2026-11-25

First published release after v0.2.1. It includes the v0.3.0 security batch below, plus these follow-up fixes. `install.sh` and `setup.sh` install this release.

### Fixed

- Installers abort when the release checksum does not match, or when no checksum tool is installed.
- SQLite now actually runs in WAL mode with foreign keys and a busy timeout. The previous DSN used a driver syntax this module ignores.
- Streamed responses are flushed to the browser as they arrive.
- A stalled response closes that stream instead of blocking the control connection, which was dropping every tunnel on the client after the pong deadline.
- Reconnecting a token that is already at its tunnel limit replaces the stale session instead of returning `tunnel_limit_reached`.
- The client's host-header policy is stored before the session can serve traffic.
- The local inspector records the real method, path, and status.
- HTTPS upstreams half-close the write side after the request, including TLS connections.
- Response reads use a 60-minute idle timeout that resets while bytes flow.
- After a successful registration, retryable rejections still stop after 10 attempts.
- Public tunnel URLs follow the listener scheme and port. Set `public_scheme` / `public_port` when a reverse proxy terminates TLS.
- Let's Encrypt only issues certificates for the base domain and a single label under it.
- `POST /setup` requires the setup token printed at startup. A page served on a tunnel subdomain cannot call the admin API.
- `X-Forwarded-For` and `X-Forwarded-Proto` from a public client are replaced with the connection tunnd accepted. Private peers (a local reverse proxy) still keep the proxy's headers.
- Request logs older than 14 days are deleted.

### Changed

- `setup.sh` no longer writes a random `admin_password` into the config. The first visit to the dashboard sets a bcrypt password. Set `ADMIN_PASSWORD` only when you want the legacy config password.
- Docker Compose publishes the admin port on `127.0.0.1` of the host.
- Landing page, docs, admin dashboard, and the local inspector share one sand-on-black arch mark.

## [0.3.0] - 2026-11-25

Tagged at the security batch. No separate binary release; those changes shipped to users in v0.3.1.

Security hardening and reconnect reliability, focused on the server's failure modes: what happens when a client reconnects mid-session, when a device is revoked, when someone hammers the login page, and when a stream outlives a request timeout.

### Added

- **Session takeover on reconnect.** Re-registering a subdomain with the **same token** now replaces the stale session instead of failing with `subdomain_in_use`. The stale connection is closed with new application close code **4429** and a human-readable reason; clients treat it as fatal (never auto-reconnect) to avoid takeover wars. Different tokens still get `subdomain_in_use`.
- **Token revocation kills active sessions.** `tunnd-server token revoke` and the dashboard's revoke button now disconnect that token's running tunnels immediately (close code 4429, reason "auth token revoked"). CLI revocations reach a running server within one minute via periodic revalidation, since the CLI is a separate process. Previously a revoked token's tunnels kept running until the client chose to disconnect.
- **Rate limiting at both entry points.** Admin login: 5 failed attempts / IP / 15 minutes (`429` + `Retry-After`, successful logins reset). Control-plane handshake: 10 failures / IP / minute, refused before the WebSocket upgrade. Buckets key on the TCP peer address, never client-supplied `X-Forwarded-For`.
- **Change-password flow.** Dashboard → Settings → Change admin password (`POST /api/password`). Stores a bcrypt hash (cost 10) and signs out every other admin session. The first-run bootstrap stores bcrypt too; legacy plaintext DB passwords are transparently upgraded to bcrypt on first successful login.
- **`admin_bind` config** (env `TUNND_ADMIN_BIND`) to control the admin listener's interface.

### Fixed

- **Long streams no longer die at 30 seconds.** The public listener previously set `WriteTimeout: 30s`, so any streamed response — SSE, long-polls, big downloads — was severed after 30 seconds on a fresh production-style listener (the dev harness ran a different listener and masked this). The public listener now uses `ReadHeaderTimeout: 30s` + `IdleTimeout: 120s` only, matching the stream-first design; liveness is enforced on the control plane (pong deadline), not on data streams. A production-configured e2e test (`TestE2E_ProductionListener_LongSSE_SurvivesOldWriteTimeout`) fails if this regresses.
- **Reconnect storms are survivable.** The client retries transient registration rejections (`subdomain_in_use`, `tunnel_limit_reached`) with exponential backoff for up to ~3 minutes, remembers its assigned subdomain so the tunnel URL stays stable across reconnects, and prints actionable hints when giving up. Previously a `subdomain_in_use` rejection during a server-side timeout window (up to ~90s) aborted the client.
- **Random subdomains are unguessable.** Generated subdomains now carry a 6-character crypto-random base32 suffix (`happy-river-4tq7zm`, 30 bits, about 4 × 10¹¹ names together with the word pair) instead of a bare word pair that could be enumerated from the word list.
- **setup.sh no longer opens port 9091 in ufw** and no longer prints a nonexistent "Username: admin" line — the dashboard is reached at `https://<domain>` (or via SSH tunnel), not over plain HTTP on a public admin port.

### Changed (breaking)

- **The admin port now binds `127.0.0.1` by default** instead of all interfaces. The dashboard remains reachable over HTTPS on the base domain and via SSH tunnel. If you relied on `http://<server-ip>:9091`, set `admin_bind: "0.0.0.0"` (or `TUNND_ADMIN_BIND=0.0.0.0`) — the two Docker compose files that publish 9091 set this explicitly, since Docker's port mapping is their access control.

### Wire compatibility

- New application close code **4429** (server → client): "this session was replaced or its token was revoked — do not reconnect." Clients older than this change treat it as an ordinary disconnect; same-token re-registration still works for them (takeover, not `subdomain_in_use`), which actually *fixes* their reconnect-after-blip behavior. The only degraded case is two old clients running the same token and subdomain simultaneously, where they now flap via takeover instead of one failing loudly — the updated client exits on 4429 instead.

## [0.2.1] - Released

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
