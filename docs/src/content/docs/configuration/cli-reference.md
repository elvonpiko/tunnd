---
title: CLI Reference
description: 'Interactive first-time setup. Asks for your server address and auth token, verifies the server is reachable, then saves config to `~/.config/tunnd/config.json`.'
---
---

## `tunnd` — client

### `tunnd setup`

Interactive first-time setup. Asks for your server address and auth token, verifies the server is reachable, then saves config to `~/.config/tunnd/config.json`.

```bash
tunnd setup
```

Run again at any time to reconfigure.

---

### `tunnd http <port>`

Tunnel a local HTTP service to your public domain. Works with any framework — Vite, Next.js, webpack, Bun, Deno, Express, FastAPI, Rails, plain `python -m http.server`. Run it and it works.

```bash
tunnd http 3000
tunnd http 5173
tunnd http 8080 --subdomain myapp
```

Tunnd transparently handles:

- Plain HTTP/1.1, WebSockets (HMR works), Server-Sent Events, chunked streaming, large uploads
- HTTP and HTTPS dev servers (auto-detected)
- IPv4 and IPv6 loopback (Windows-bound `::1` listeners just work)
- Host header rewriting for frameworks that pin `allowedHosts`
- `X-Forwarded-For` / `X-Forwarded-Proto` / `X-Forwarded-Host` for upstream apps that read them

WebSocket upgrades are handled automatically on every HTTP tunnel — no flag, no protocol declaration. If your app is primarily realtime, [`tunnd ws`](#tunnd-ws-port) prints a `wss://` URL instead so it's ready to paste into a client.

| Flag | Default | Description |
|------|---------|-------------|
| `--subdomain`, `-s` | random | Pin a specific subdomain |
| `--inspector-port` | 4040 | Local inspector UI port (`0` to disable) |

#### Power-user flags

You usually don't need these. Set them when you want to override tunnd's defaults.

| Flag | Default | Description |
|------|---------|-------------|
| `--host-header` | `rewrite` | `rewrite` (default) replaces the public Host with `localhost:<port>`. `preserve` forwards the public Host unchanged — useful when your upstream uses Host-based routing or signed URLs. Pass any literal hostname to set a fixed value (e.g. `app.local`). |
| `--upstream-scheme` | auto | Force `http` or `https` instead of letting tunnd auto-detect. Auto-detect runs a quick TLS probe on first connect. |
| `--upstream-tls-skip-verify` | `false` | Skip TLS verification on the upstream. Auto-detect already does this for self-signed dev certs; set this when forcing `--upstream-scheme=https` against a self-signed cert. |

---

### `tunnd ws <port>`

Tunnel a WebSocket service. This is the **same transport as `tunnd http`** — it registers an HTTP tunnel on the wire and accepts the same flags — but it prints a `wss://` URL in the banner so the public address is copy-paste ready for a WebSocket client. HTTP requests on the same port keep working, so apps that mix REST and WebSockets are fully supported.

```bash
tunnd ws 3000
tunnd ws 8080 --subdomain realtime
```

```
  ▲  Tunnd

  Forwarding    wss://icy-creek.tunnd.yourdomain.com → localhost:3000
  HTTP/HTTPS    https://icy-creek.tunnd.yourdomain.com (same port)
  Inspector     http://localhost:4040
```

Use `tunnd http` when your app is mostly REST with occasional upgrades; use `tunnd ws` when the WebSocket URL is the thing you want to share. Functionally they are interchangeable.

---

### `tunnd tcp <port>`

Tunnel a raw TCP port. Each public connection gets its own bidirectional stream — useful for databases, SSH, custom protocols, anything that speaks TCP.

The server allocates a public port from its configured range (default `20000–20100`) and prints the resulting `tcp://host:port` URL.

```bash
tunnd tcp 5432
tunnd tcp 22
```

The inspector is always disabled for TCP tunnels (no HTTP request semantics).

---

### `tunnd status`

Show the current configuration.

```bash
tunnd status
```

---

### `tunnd update`

Check for the latest released version and install it. If you're already on the latest, the command is a no-op.

```bash
tunnd update
```

The CLI also prints a single-line "v0.x.y is available" hint at most once every 24 hours when starting a tunnel. Disable that check with `TUNND_NO_UPDATE_CHECK=1` if you don't want any background network traffic to GitHub's API.

---

### `tunnd version`

```bash
tunnd version
```

---

## `tunnd-server` — server

### `tunnd-server` (start)

Start the server. Reads config from file or environment.

```bash
tunnd-server --config /etc/tunnd/tunnd-server.yaml
```

---

### `tunnd-server token`

Manage auth tokens. These are also manageable from the admin dashboard.

```bash
tunnd-server token create my-laptop
tunnd-server token create ci-server --max-tunnels 5
tunnd-server token list
tunnd-server token revoke <id>
```

| Flag | Description |
|------|-------------|
| `--max-tunnels` | Maximum concurrent tunnels for this token (0 = unlimited) |

---

### `tunnd-server version`

```bash
tunnd-server version
```

---

## Exit codes

| Code | Meaning |
|------|---------|
| `0` | Success |
| `1` | Error (message printed to stderr) |

### Client exit code 1 causes

- Not set up yet — run `tunnd setup`
- Server unreachable — check address with `tunnd status`
- Token rejected (invalid or revoked)
- Subdomain in use or invalid

### Server exit code 1 causes

- `domain` not configured
- Port already in use
- Database cannot be opened
