---
title: Server Configuration Reference
description: 'The Tunnd server reads configuration from a YAML file. All fields can also be overridden with `TUNND_<FIELD>` environment variables.'
---
The Tunnd server reads configuration from a YAML file. All fields can also be overridden with `TUNND_<FIELD>` environment variables.

## Config file location

The server searches in this order:

| Priority | Path |
|----------|------|
| 1 | `--config <path>` flag |
| 2 | `./tunnd-server.yaml` |
| 3 | `~/.tunnd/tunnd-server.yaml` |
| 4 | `/etc/tunnd/tunnd-server.yaml` |

---

## Fields

### `domain` *(required)*

Base domain for tunnels. Tunnels are exposed as `<subdomain>.<domain>`.

Requires a wildcard DNS A record: `*.tunnd.yourdomain.com → <server-ip>`

```yaml
domain: "tunnd.yourdomain.com"
```

---

### `http_port`

Port for tunnel traffic and WebSocket client connections.

- **Behind Caddy (recommended):** set this to the internal port Caddy proxies to (e.g. `9095`)
- **Standalone:** use `443` (requires TLS config)

Default: `443`

```yaml
http_port: 9095
```

---

### `admin_port`

Port for the admin dashboard and REST API.

Default: `9091`

```yaml
admin_port: 9096
```

---

### `admin_bind`

Interface the admin dashboard binds to.

Default: `127.0.0.1` — the admin port is **loopback-only** and not reachable from the network. The dashboard remains reachable over HTTPS on the base domain (`https://your-domain.tld`) and via SSH tunnel.

Set `0.0.0.0` to listen on all interfaces — for example when Docker's published port mapping is your access control, or when you deliberately want LAN access. Fire-wall it accordingly.

```yaml
admin_bind: "0.0.0.0"   # opt back into all-interfaces binding
```

Environment variable: `TUNND_ADMIN_BIND`

::: warning[Changed in v0.2.1 — breaking]
Previously the admin port bound to all interfaces. If you access the dashboard via `http://<server-ip>:9091`, add `admin_bind: "0.0.0.0"` to keep that setup — or better, switch to the base-domain HTTPS route or an SSH tunnel.
:::

---

### `db_path`

SQLite database file path. The directory is created automatically.

Default: `./tunnd.db`

```yaml
db_path: "/data/tunnd.db"
```

---

### `admin_password`

Admin dashboard password. **Optional at config level** — if left empty, the server shows a one-time **bootstrap setup page** the first time you visit the dashboard, where you set the password interactively.

Passwords set via the dashboard (or changed later in **Settings → Change admin password**) are stored as bcrypt hashes in the database — plaintext is never persisted. Changing the password also signs out every other admin session.

Setting it here still works (setup scripts use it), but while it's set the dashboard change-password flow is disabled — the config value would win after a restart anyway. Prefer the bootstrap/dashboard flow for new installs.

```yaml
# Leave empty to use the first-run bootstrap flow
# admin_password: ""

# Or set explicitly (legacy / unattended installs)
# admin_password: "your-strong-password"
```

::: tip
The bootstrap approach is the default. You don't need this field in your config at all.
:::

---

### `reserved_subdomains`

Subdomain names clients cannot register. Defaults to `["www", "api", "admin", "mail", "ftp"]`.

```yaml
reserved_subdomains:
  - "www"
  - "api"
  - "admin"
  - "mail"
  - "ftp"
```

---

### `max_tunnels_per_token`

Maximum concurrent tunnels per auth token. `0` = unlimited.

Default: `0`

```yaml
max_tunnels_per_token: 5
```

---

### `tcp_min_port` / `tcp_max_port`

The inclusive port range from which the server allocates public ports for `tunnd tcp <port>` clients. Open this range in your firewall (and publish it from Docker if you run in a container).

Defaults: `20000` – `20100` (room for 100 simultaneous TCP tunnels).

```yaml
tcp_min_port: 20000
tcp_max_port: 20100
```

---

### `log_level`

`debug` | `info` | `warn` | `error`

Default: `info`

---

### `log_format`

`pretty` (human-readable) | `json` (for log aggregators)

Default: `pretty`

---

### TLS options (standalone mode only)

Only needed if you're **not** using a reverse proxy like Caddy.

#### `tls_email` — Let's Encrypt automatic

```yaml
tls_email: "you@example.com"
acme_cache_dir: "/data/.autocert-cache"
```

Port 80 must be publicly reachable.

#### `tls_cert_file` + `tls_key_file` — manual certificate

```yaml
tls_cert_file: "/etc/tunnd/certs/fullchain.pem"
tls_key_file:  "/etc/tunnd/certs/privkey.pem"
```

---

## Minimal example (behind Caddy)

```yaml
domain: "tunnd.yourdomain.com"
http_port: 9095
admin_port: 9096
db_path: "/data/tunnd.db"
log_level: "info"
log_format: "json"

# TCP tunneling — open these ports in your firewall too
tcp_min_port: 20000
tcp_max_port: 20100
```

No `admin_password` needed — set it on first visit to the dashboard.

---

## Next steps

- [Server Deployment](/getting-started/server-deployment)
- [CLI Reference](/configuration/cli-reference)
