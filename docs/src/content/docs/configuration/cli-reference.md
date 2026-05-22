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

Tunnel an HTTP service on localhost. Transparently supports:

- Plain HTTP/1.1 requests
- WebSocket (`ws://` / `wss://`) upgrades
- Server-Sent Events (`text/event-stream`)
- Chunked transfer encoding and streaming responses

```bash
tunnd http 3000
tunnd http 8080 --subdomain myapp
tunnd http 3000 --inspector-port 4041
tunnd http 3000 --inspector-port 0   # disable inspector
```

| Flag | Default | Description |
|------|---------|-------------|
| `--subdomain`, `-s` | random | Pin a specific subdomain |
| `--inspector-port` | 4040 | Local inspector UI port (`0` to disable) |

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
