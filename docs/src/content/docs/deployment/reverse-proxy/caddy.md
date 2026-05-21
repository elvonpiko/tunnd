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
tunnd-server:9095  ← tunnel traffic + WebSocket control plane
tunnd-server:9096  ← admin dashboard
```

Tunnd is never exposed to the internet directly — only Caddy is.

---

## Caddyfile

Replace `tunnd.example.com` with your actual domain and cert paths:

```caddy
# Admin dashboard
tunnd.example.com {
    tls /etc/caddy/certs/tunnd/fullchain.pem /etc/caddy/certs/tunnd/privkey.pem

    # WebSocket control plane — clients connect here
    handle /_tunnd/control {
        reverse_proxy tunnd-server:9095
    }

    # Admin dashboard
    handle {
        reverse_proxy tunnd-server:9096
    }
}

# All tunnel subdomains
*.tunnd.example.com {
    tls /etc/caddy/certs/tunnd/fullchain.pem /etc/caddy/certs/tunnd/privkey.pem

    reverse_proxy tunnd-server:9095 {
        header_up X-Real-IP {remote_host}
        header_up X-Forwarded-For {remote_host}
        header_up X-Forwarded-Proto {scheme}
        header_up Host {host}
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
http_port: 9095     # internal — Caddy handles public :443
admin_port: 9096
db_path: "/data/tunnd.db"
log_level: "info"
log_format: "json"
# No tls_email, no tls_cert_file — Caddy handles TLS
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
