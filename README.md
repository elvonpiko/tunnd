<div align="center">

<img src="docs/src/assets/logo.svg" alt="Tunnd" width="56" height="56">

# tunnd

**Self-hosted ngrok alternative — HTTP, WebSockets, and raw TCP tunnels in one Go binary.**

[![CI](https://github.com/elvonpiko/tunnd/actions/workflows/ci.yml/badge.svg)](https://github.com/elvonpiko/tunnd/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/elvonpiko/tunnd)](https://goreportcard.com/report/github.com/elvonpiko/tunnd)
[![License: MIT](https://img.shields.io/badge/License-MIT-violet.svg)](LICENSE)
[![Latest Release](https://img.shields.io/github/v/release/elvonpiko/tunnd)](https://github.com/elvonpiko/tunnd/releases/latest)

[Website](https://elvonpiko.github.io/tunnd/) · [Docs](https://elvonpiko.github.io/tunnd/getting-started/quick-start/) · [Quick start](#quick-start) · [Self-hosting](#self-hosting) · [Why?](#why-tunnd)

<br>

```
$ tunnd http 3000

  ▲  Tunnd

  Forwarding    https://brave-river.tunnd.yourdomain.com → localhost:3000
  Inspector     http://localhost:4040

  Ctrl+C to close tunnel
```

</div>

---

## Why tunnd?

Tunnd is the self-hosted tunnel server I wished existed. It does HTTP, WebSockets, and raw TCP — the things developers actually tunnel — on a VPS you already own. No subscriptions, no third party, one binary, one command.

I built it because existing options each had a deal-breaker: hosted services route my traffic through their network and meter it, Cloudflare Tunnel doesn't do raw TCP, and the closest self-hosted alternatives wanted YAML rituals before they'd open a single port. So I wrote what I wanted: a single Go binary that gives me a public HTTPS URL or TCP port pointed at localhost, served from infrastructure I already pay for.

|                                | **Tunnd**       | ngrok free    | Cloudflare Tunnel | frp        |
| ------------------------------ | --------------- | ------------- | ----------------- | ---------- |
| Self-hosted                    | ✅              | ❌            | partial           | ✅         |
| Auto HTTPS                     | ✅              | ✅            | ✅                | manual     |
| Custom subdomain               | ✅              | paid          | ✅                | manual     |
| Unlimited tunnels              | ✅              | 1 only        | ✅                | ✅         |
| WebSocket / SSE                | ✅              | ✅            | ✅                | ✅         |
| Raw TCP                        | ✅              | paid          | ❌                | ✅         |
| Live request inspector         | ✅ built-in     | ✅            | ❌                | ❌         |
| Admin dashboard                | ✅              | limited       | CF dashboard      | ❌         |
| Monthly cost                   | **VPS only**    | $10–20/mo     | free tier         | free       |
| Your traffic stays yours       | ✅              | ❌            | ❌                | ✅         |

### What Tunnd doesn't do (yet)

Honesty matters. If you need any of these today, ngrok or Cloudflare Tunnel is probably the better fit:

- **OAuth / SSO in front of tunnels.** ngrok's `--oauth google` flag has no equivalent in Tunnd. You can layer auth in your reverse proxy or your app.
- **Request replay from the inspector.** Tunnd shows every request that flowed through a tunnel; ngrok lets you re-fire any of them with one click.
- **Bring-your-own arbitrary domains.** Tunnd serves wildcard subdomains under one base domain (`*.tunnd.yourdomain.com`). ngrok lets you point any domain you own at any tunnel.
- **Edge anycast.** ngrok runs on a global anycast network. Tunnd lives wherever your VPS is — pick a region close to you.
- **Team accounts, audit logs, SSO, status page.** Tunnd is a single-operator tool. If you need org-level features, you need a hosted product.

These are on the roadmap as community demand justifies them. PRs and feature requests welcome.

---

## Quick start

### 1. Self-host the server

SSH into your VPS, then run:

```bash
curl -fsSL https://raw.githubusercontent.com/elvonpiko/tunnd/main/setup.sh | sudo bash
```

The script asks for your domain and email, installs the binary, requests a Let's Encrypt certificate, and starts a systemd service. Visit `https://yourdomain.com` to set your admin password and create your first auth token.

> Prefer Docker, manual TLS, or running behind Caddy? See the [deployment guide](https://elvonpiko.github.io/tunnd/getting-started/server-deployment/).

### 2. Install the client

**macOS / Linux:**

```bash
curl -fsSL https://raw.githubusercontent.com/elvonpiko/tunnd/main/install.sh | bash
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/elvonpiko/tunnd/main/install.ps1 | iex
```

### 3. Open a tunnel

```bash
tunnd setup        # paste the server URL + token from your dashboard
tunnd http 3000    # public HTTPS URL pointing at localhost:3000
```

```
  ▲  Tunnd

  Forwarding    https://brave-river.tunnd.yourdomain.com → localhost:3000
  Inspector     http://localhost:4040

  Ctrl+C to close tunnel
```

That's it. Open the inspector at `http://localhost:4040` to see live request logs while the tunnel is open.

---

## What you can tunnel

```bash
tunnd http 3000                       # any HTTP service — public HTTPS URL
tunnd http 3000 --subdomain myapp     # pin a subdomain
tunnd tcp 5432                        # raw TCP — Postgres, Redis, SSH, anything
```

WebSocket upgrades pass through transparently — there's no flag to set, no protocol to declare. Streaming HTTP responses (chunked transfer, Server-Sent Events) keep their flushes end to end. Raw TCP gets a public port allocated automatically from the server's configured range (default `20000–20100`).

---

## Self-hosting

### Prerequisites

- A VPS (any provider — 1 vCPU, 512 MB RAM is plenty)
- A domain with DNS access
- Ports 80, 443, and 9091 open
- A range of TCP ports open if you plan to use `tunnd tcp` (default: `20000–20100`)

### DNS setup

Add a wildcard A record at your DNS provider:

```
Type:  A
Name:  *.tunnd
Value: <your-server-ip>
TTL:   3600
```

This makes both `tunnd.yourdomain.com` and any `*.tunnd.yourdomain.com` resolve to your server.

### One-command setup (Ubuntu / Debian)

```bash
# Recommended — interactive (asks for domain + email):
curl -fsSL https://raw.githubusercontent.com/elvonpiko/tunnd/main/setup.sh | sudo bash

# Non-interactive (CI / automation):
curl -fsSL https://raw.githubusercontent.com/elvonpiko/tunnd/main/setup.sh | sudo \
  DOMAIN=tunnd.yourdomain.com EMAIL=you@example.com bash

# With a manual cert:
curl -fsSL https://raw.githubusercontent.com/elvonpiko/tunnd/main/setup.sh | sudo \
  DOMAIN=tunnd.yourdomain.com \
  TLS_CERT=/etc/ssl/certs/wildcard.pem \
  TLS_KEY=/etc/ssl/private/wildcard.key bash
```

The script installs the server binary, creates a system user, writes a config file, registers a systemd service, opens firewall ports, and prints the admin password and a starter auth token.

### Docker Compose

```bash
curl -fsSL https://raw.githubusercontent.com/elvonpiko/tunnd/main/docker-compose.yml -o docker-compose.yml

export TUNND_DOMAIN=tunnd.yourdomain.com
export TUNND_TLS_EMAIL=you@example.com

docker compose up -d
```

Then visit `https://tunnd.yourdomain.com` to set your admin password and create a token. The `docker-compose.caddy.yml` and `docker-compose.manual-tls.yml` variants cover reverse-proxy and bring-your-own-cert deployments — see the [Docker docs](https://elvonpiko.github.io/tunnd/deployment/docker/).

### TLS options

Tunnd supports three modes:

- **Let's Encrypt (automatic)** — set `tls_email` and `acme_cache_dir` in config. Per-subdomain certs issued on first request. Port 80 must be publicly reachable.
- **Manual / wildcard certificate** — set `tls_cert_file` and `tls_key_file`. Get a wildcard cert with Certbot DNS-01.
- **No TLS (local dev)** — set `http_port` to anything other than 443.

See [TLS Certificates](https://elvonpiko.github.io/tunnd/deployment/tls-certificates/) for details.

---

## Configuration

Server config lives at `/etc/tunnd/tunnd-server.yaml`. All values can be overridden with `TUNND_<KEY>` environment variables.

```yaml
domain: "tunnd.yourdomain.com"
http_port: 443
admin_port: 9091

# TLS — pick one
tls_email: "you@example.com"
acme_cache_dir: "/var/lib/tunnd/.autocert-cache"
# tls_cert_file: "/etc/letsencrypt/live/tunnd.yourdomain.com/fullchain.pem"
# tls_key_file:  "/etc/letsencrypt/live/tunnd.yourdomain.com/privkey.pem"

db_path: "/var/lib/tunnd/tunnd.db"
admin_password: ""        # leave empty to set via the dashboard's first-run setup
max_tunnels_per_token: 0  # 0 = unlimited

# Public TCP port range allocated to `tunnd tcp <port>` clients
tcp_min_port: 20000
tcp_max_port: 20100

log_level: "info"         # debug | info | warn | error
log_format: "json"        # json | pretty
```

See the full [server config reference](https://elvonpiko.github.io/tunnd/configuration/server-config/) and [CLI reference](https://elvonpiko.github.io/tunnd/configuration/cli-reference/).

---

## CLI

### Client

```
tunnd setup                  configure server URL and token (interactive)
tunnd http <port> [flags]    tunnel an HTTP service
  --subdomain string         pin a subdomain (random if not set)
  --inspector-port int       local inspector UI port (default 4040)
tunnd tcp <port>             tunnel a raw TCP port
tunnd status                 print current configuration
tunnd update                 install the latest released client
tunnd version                print version
```

### Server

```
tunnd-server [--config FILE]
tunnd-server token create [LABEL] [--max-tunnels N]
tunnd-server token list
tunnd-server token revoke <ID>
tunnd-server version
```

---

## Admin API

Login establishes a session cookie (12-hour TTL). The dashboard is served from the admin port (`9091` by default) — visit `http://<server-ip>:9091` or, behind a reverse proxy, your admin domain.

```
GET    /api/stats                       server stats
GET    /api/tunnels/active              live tunnel sessions
GET    /api/tunnels?limit=50&offset=0   tunnel history
GET    /api/tunnels/{id}/requests       request log for a tunnel
GET    /api/tokens                      list tokens
POST   /api/tokens                      create a token
DELETE /api/tokens/{id}                 revoke a token
```

Full reference in the [Admin API docs](https://elvonpiko.github.io/tunnd/api/admin-api/).

---

## Architecture

```
HTTP:
  Browser ──HTTPS──▶ tunnd-server ──WebSocket──▶ tunnd client ──TCP──▶ localhost:3000
                     (your VPS)                  (your laptop)

TCP:
  Client  ──TCP─────▶ tunnd-server ──WebSocket──▶ tunnd client ──TCP──▶ localhost:5432
                      (port allocated from
                       tcp_min_port..tcp_max_port)
```

The server accepts public HTTPS traffic on a wildcard domain, looks up the tunnel session by subdomain, and bridges the request bytes to the connected client over a single WebSocket. The client dials the local service and pipes bytes back. The wire protocol uses two frame kinds — JSON envelopes for control messages and tagged binary frames for stream payloads — which avoids ~33% base64 inflation and per-chunk JSON parsing on the data path.

For deeper detail on the request lifecycle and the protocol, see [Architecture](https://elvonpiko.github.io/tunnd/architecture/overview/) and [Data Flow](https://elvonpiko.github.io/tunnd/architecture/data-flow/).

---

## Development

```bash
git clone https://github.com/elvonpiko/tunnd
cd tunnd
go mod tidy
make build            # → bin/tunnd-server + bin/tunnd
make test
make lint
```

### Local dev (no TLS)

```bash
TUNND_DOMAIN=localhost TUNND_HTTP_PORT=8081 TUNND_ADMIN_PORT=9091 \
  ./bin/tunnd-server

./bin/tunnd-server token create dev    # or set the password via the dashboard

./bin/tunnd setup                      # → wss://localhost:8081, paste token
./bin/tunnd http 3000
```

### Project layout

```
cmd/
  server/           tunnd-server entrypoint + token CLI
  client/           tunnd client (setup wizard, http/tcp commands, inspector UI)
internal/
  auth/             token issuance + validation (192-bit random)
  config/           viper-based loader (file < env < flags)
  control/          WebSocket control-plane handler
  admin/            REST API + embedded dashboard, login + bootstrap
  store/            SQLite persistence (tokens, tunnels, request logs, settings)
  tunnel/           in-memory registry + HTTP reverse proxy + TCP listener
pkg/
  proto/            wire protocol (JSON envelopes + binary data frames)
docs/
  public/index.html landing page (served from / on GitHub Pages)
  src/content/docs/ Astro Starlight documentation
```

---

## Security

- All tunnel traffic is TLS-encrypted (HTTPS / WSS)
- Client auth uses 192-bit cryptographically random tokens (`tnnd_<48 hex>`)
- Admin sessions use HttpOnly Secure SameSite=Strict cookies
- Token values are shown only at creation; list APIs return only a hint
- SQLite uses WAL mode with foreign-key enforcement
- The systemd unit runs as an unprivileged `tunnd` user with `NoNewPrivileges`, `ProtectSystem=strict`, `ProtectHome=yes`, `PrivateTmp`, `PrivateDevices`, and only the `CAP_NET_BIND_SERVICE` capability (for binding ports 80/443)

**Reporting vulnerabilities:** open a [GitHub Security Advisory](https://github.com/elvonpiko/tunnd/security/advisories/new) rather than a public issue.

---

## Contributing

Contributions are welcome — bug reports, feature suggestions, documentation fixes, and PRs all help. See [CONTRIBUTING.md](CONTRIBUTING.md) for the workflow and conventions. By participating, you agree to our [Code of Conduct](CODE_OF_CONDUCT.md).

If you're not sure where to start, the [`good first issue`](https://github.com/elvonpiko/tunnd/labels/good%20first%20issue) label has tasks suitable for first-time contributors.

---

## License

MIT — see [LICENSE](LICENSE).
