---
title: Docker Deployment
description: 'The Tunnd server is available as a multi-arch Docker image (amd64 + arm64).'
---
The Tunnd server is available as a multi-arch Docker image (amd64 + arm64).

```
ghcr.io/elvonpiko/tunnd-server:latest
```

---

## Quick start — Docker Compose

Three compose files are included. Choose the one that matches your setup.

### Let's Encrypt (simplest, automatic TLS)

Port 80 must be publicly reachable for the ACME challenge.

```bash
export TUNND_DOMAIN=tunnd.yourdomain.com
export TUNND_TLS_EMAIL=you@example.com
docker compose up -d
```

Uses `docker-compose.yml`. Tunnd handles TLS directly — no reverse proxy needed.

### Manual TLS certificate

Use when you already have a wildcard cert (e.g. from Certbot DNS-01).

```bash
export TUNND_DOMAIN=tunnd.yourdomain.com
export TLS_CERTS_DIR=/etc/letsencrypt/live/tunnd.yourdomain.com
docker compose -f docker-compose.manual-tls.yml up -d
```

### Behind Caddy (existing reverse proxy)

Use when you already run Caddy and want Tunnd on the internal Docker network.

```bash
export TUNND_DOMAIN=tunnd.yourdomain.com
docker compose -f docker-compose.caddy.yml up -d
```

See [Caddy Reverse Proxy](/deployment/reverse-proxy/caddy) for Caddyfile configuration.

---

## Environment variables

| Variable | Required | Description |
|----------|----------|-------------|
| `TUNND_DOMAIN` | Yes | Base domain (e.g. `tunnd.yourdomain.com`) |
| `TUNND_TLS_EMAIL` | Let's Encrypt only | Email for ACME registration |
| `TUNND_TLS_CERT_FILE` | Manual TLS only | Path to certificate PEM |
| `TUNND_TLS_KEY_FILE` | Manual TLS only | Path to private key PEM |
| `TUNND_HTTP_PORT` | No | Tunnel port (default: `443`) |
| `TUNND_ADMIN_PORT` | No | Admin port (default: `9091`) |
| `TUNND_DB_PATH` | No | SQLite path (default: `/data/tunnd.db`) |
| `TUNND_ACME_CACHE_DIR` | No | Let's Encrypt cache (default: `/data/.autocert-cache`) |
| `TUNND_LOG_LEVEL` | No | `debug`/`info`/`warn`/`error` (default: `info`) |
| `TUNND_LOG_FORMAT` | No | `pretty`/`json` (default: `pretty`) |

`TUNND_ADMIN_PASSWORD` is **not required** — set your password on first login via the dashboard.

---

## Persistent data

Mount `/data` as a named volume to preserve the database and certificate cache across restarts:

```yaml
volumes:
  - tunnd-data:/data
```

The container runs as UID 1000 (`tunnd` user). If you use a bind mount:
```bash
sudo chown -R 1000:1000 /opt/tunnd/data
```

---

## Run without Compose

```bash
docker run -d \
  --name tunnd \
  --restart unless-stopped \
  -p 80:80 -p 443:443 -p 9091:9091 \
  -v tunnd-data:/data \
  -e TUNND_DOMAIN=tunnd.yourdomain.com \
  -e TUNND_TLS_EMAIL=you@example.com \
  ghcr.io/elvonpiko/tunnd-server:latest
```

---

## Common operations

```bash
# Check status
docker compose ps

# Follow logs
docker compose logs -f tunnd

# Create a client token
docker compose exec tunnd tunnd-server token create my-laptop

# Upgrade
docker compose pull && docker compose up -d

# Stop (data preserved in volume)
docker compose down

# Stop + remove data (destructive)
docker compose down -v
```

---

## Health check

The image includes a health check that polls `http://localhost:9091/healthz` (an unauthenticated endpoint that returns `200 ok` whenever the server is alive). Status shows `healthy` once the server is fully started (~10s).

```bash
docker inspect --format='{{.State.Health.Status}}' tunnd
```

---

## Next steps

- [TLS Certificates](/deployment/tls-certificates) — certificate options in depth
- [Caddy Reverse Proxy](/deployment/reverse-proxy/caddy) — running behind Caddy
- [Server Config Reference](/configuration/server-config)
