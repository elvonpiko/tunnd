---
title: Caddy Reverse Proxy
description: 'Run Tunnd behind Caddy when you already have Caddy managing TLS and routing on your VPS. Caddy handles TLS termination — Tunnd runs on internal ports without '
---
Run Tunnd behind Caddy when you already have Caddy managing TLS and routing on your VPS. Caddy handles TLS termination — Tunnd runs on internal ports without touching certificates.

This is the setup used in the [example VPS deployment](/getting-started/server-deployment) with `docker-compose.caddy.yml`.

---

## How it works

```
Internet (HTTPS :443)
    ↓
Caddy (TLS termination, your wildcard cert)
    ↓
tunnd-server:8080  ← tunnel traffic, WebSocket control plane, and the admin dashboard
```

The public listener serves all three. The base domain is the dashboard; subdomains are tunnels; `/_tunnd/control` is the client connection. The admin port stays on loopback and is not what Caddy should proxy. Tunnd is not exposed to the internet directly.

---

## Caddyfile

Replace `tunnd.example.com` with your actual domain and cert paths:

```caddy
# Base domain (dashboard + control plane) and tunnel subdomains.
# Point both at tunnd's plain HTTP port. Set TUNND_PUBLIC_SCHEME=https
# and TUNND_PUBLIC_PORT=443 so the client prints https URLs.
*.tunnd.example.com, tunnd.example.com {
    tls /etc/caddy/certs/tunnd/fullchain.pem /etc/caddy/certs/tunnd/privkey.pem

    reverse_proxy tunnd-server:8080 {
        header_up X-Real-IP {remote_host}
        header_up X-Forwarded-For {remote_host}
        header_up X-Forwarded-Proto {scheme}
    }
}
```

::: tip WebSocket
Caddy handles `Connection: Upgrade` and `Upgrade: websocket` automatically — no extra configuration needed.
:::

---

## Docker Compose

`docker-compose.caddy.yml` deploys Tunnd + Caddy together on the `proxy` network:

```bash
export TUNND_DOMAIN=tunnd.example.com
docker compose -f docker-compose.caddy.yml up -d
```

If Caddy is already running as a separate container on a `proxy` network:

1. Start only the `tunnd-server` service:
   ```bash
   docker compose -f docker-compose.caddy.yml up -d tunnd-server
   ```

2. Make sure `tunnd-server` joins the same network as Caddy:
   ```yaml
   # in docker-compose.caddy.yml, the tunnd-server service already uses
   # an external network named "proxy" — same as your Caddy container
   networks:
     - proxy
   networks:
     proxy:
       external: true
   ```

3. Update your Caddyfile to add the Tunnd routes and reload:
   ```bash
   docker exec caddy caddy reload --config /etc/caddy/Caddyfile
   ```

---

## Server config for behind-Caddy mode

```yaml
# /data/tunnd-server.yaml
domain: "tunnd.example.com"
http_port: 8080              # internal — Caddy handles public :443
public_scheme: "https"       # URLs the client prints
public_port: 443
db_path: "/data/tunnd.db"
log_level: "info"
log_format: "json"
# No tls_email, no tls_cert_file — Caddy handles TLS.
# Leave admin_password unset and set it on the first visit to the dashboard.
```

---

## Manual TLS certs with Caddy

Caddy can use your own certificates instead of Let's Encrypt. Mount them into the Caddy container and reference them in the Caddyfile:

```yaml
# in docker-compose.caddy.yml
caddy:
  volumes:
    - ./certs:/etc/caddy/certs:ro
```

```caddy
tunnd.example.com {
    tls /etc/caddy/certs/tunnd/fullchain.pem /etc/caddy/certs/tunnd/privkey.pem
    ...
}
```

This is exactly how the example VPS is configured with `pixnode.cloud` certs.

---

## Verify

```bash
# Server reachable
curl https://tunnd.example.com/api/stats

# WebSocket upgrade works
curl -I -H "Upgrade: websocket" -H "Connection: Upgrade" \
     https://tunnd.example.com/_tunnd/control

# Wildcard routing works
curl https://myapp.tunnd.example.com
# Expected: 502 (no client connected) or response from your tunnel
```
