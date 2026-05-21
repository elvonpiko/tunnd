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

### `db_path`

SQLite database file path. The directory is created automatically.

Default: `./tunnd.db`

```yaml
db_path: "/data/tunnd.db"
```

---

### `admin_password`

Admin dashboard password. **Optional at config level** — if left empty, the server shows a one-time **bootstrap setup page** the first time you visit the dashboard, where you set the password interactively.

Once set via the dashboard, the password is stored in the database. You can also set it here to skip the bootstrap step entirely.

```yaml
# Leave empty to use the first-run bootstrap flow
# admin_password: ""

# Or set explicitly to skip bootstrap
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
